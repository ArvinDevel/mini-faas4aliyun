package core

import (
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
	"context"
	"sort"
	"time"

	nspb "aliyun/serverless/mini-faas/nodeservice/proto"
	rmPb "aliyun/serverless/mini-faas/resourcemanager/proto"
	cp "aliyun/serverless/mini-faas/scheduler/config"
	"aliyun/serverless/mini-faas/scheduler/model"
	pb "aliyun/serverless/mini-faas/scheduler/proto"
	"github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
)

func NewRouter(config *cp.Config, rmClient rmPb.ResourceManagerClient) *Router {
	return &Router{
		nodeMap:     cmap.New(),
		fn2finfoMap: cmap.New(),
		fn2ctnSlice: cmap.New(),
		requestMap:  cmap.New(),
		ctn2info:    cmap.New(),
		cnt2node:    cmap.New(),
		rmClient:    rmClient,
	}
}

func (r *Router) Start() {
	// todo swarm up and use ONE NODE first(currently multiple query boost 10 nodes)
	go r.UpdateStats()
	go r.ReleaseCtnResource()
}

func (r *Router) AcquireContainer(ctx context.Context, req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	// Save the name for later ReturnContainer
	fn := req.FunctionName
	r.requestMap.Set(req.RequestId, fn)
	r.fn2ctnSlice.SetIfAbsent(fn, &RwLockSlice{})
	r.fn2finfoMap.SetIfAbsent(fn, &model.FuncInfo{
		TimeoutInMs:   req.FunctionConfig.TimeoutInMs,
		MemoryInBytes: req.FunctionConfig.MemoryInBytes,
	})
	funcExeMode := r.getFuncExeMode(req)
	return r.pickCntAccording2ExeMode(funcExeMode, req)
}

func (r *Router) getNode(accountId string, memoryReq int64) (*ExtendedNodeInfo, error) {
	values := []*ExtendedNodeInfo{}
	for _, key := range r.nodeMap.Keys() {
		nmObj, _ := r.nodeMap.Get(key)
		node := nmObj.(*ExtendedNodeInfo)
		values = append(values, node)
	}
	sortNodeByUsage(values)
	// todo best fit
	for _, node := range values {
		node.Lock()
		if node.availableMemInBytes > memoryReq {
			node.availableMemInBytes -= memoryReq
			node.Unlock()
			return node, nil
		}
		node.Unlock()
	}
	logger.Infof("current nodes %s can't affoard %d", values, memoryReq)
	node, err := r.remoteGetNode(accountId, memoryReq)
	if (err != nil) {
		return r.fallbackUseLocalNode(values)
	}
	return node, nil
}

func (r *Router) remoteGetNode(accountId string, memoryReq int64) (*ExtendedNodeInfo, error) {
	// todo use adaquate timeout
	ctxR, cancelR := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelR()
	now := time.Now().UnixNano()
	replyRn, err := r.rmClient.ReserveNode(ctxR, &rmPb.ReserveNodeRequest{
		AccountId: accountId,
	})
	if err != nil {
		logger.WithFields(logger.Fields{
			"Operation": "ReserveNode",
			"Latency":   (time.Now().UnixNano() - now) / 1e6,
			"Error":     true,
		}).Errorf("Failed to reserve node due to %v", err)
		return nil, errors.WithStack(err)
	}

	nodeDesc := replyRn.Node
	node, err := NewNode(nodeDesc.Id, nodeDesc.Address, nodeDesc.NodeServicePort, nodeDesc.MemoryInBytes-memoryReq)
	logger.Infof("ReserveNode accntId %s %s Latency %d",
		accountId, node, (time.Now().UnixNano()-now)/1e6)
	if err != nil {
		go r.remoteReleaseNode(nodeDesc.Id)
		return nil, err
	}
	r.nodeMap.Set(nodeDesc.Id, node)
	return node, nil
}

func (r *Router) fallbackUseLocalNode(localNodes []*ExtendedNodeInfo) (*ExtendedNodeInfo, error) {
	if len(localNodes) == 0 {
		return nil, errors.Errorf("No local node can be reused")
	}

	// choose more
	sort.Slice(localNodes, func(i, j int) bool {
		if localNodes[i].CpuUsagePct > nodeCpuHighThreshold {
			return false
		}
		if localNodes[i].failedCnt > nodeFailedCntThreshold {
			return false
		}
		if localNodes[i].MemoryUsageInBytes/localNodes[i].TotalMemoryInBytes > nodeMemHighThreshold {
			return false
		}
		aMem := localNodes[i].AvailableMemoryInBytes
		aCpu := localNodes[i].CpuUsagePct
		aVal := aMem*0.8 + (200-aCpu)*0.2
		bMem := localNodes[j].AvailableMemoryInBytes
		bCpu := localNodes[j].CpuUsagePct
		bVal := bMem*0.8 + (200-bCpu)*0.2
		return aVal > bVal
	})
	return localNodes[0], nil
}

func (r *Router) handleContainerErr(node *ExtendedNodeInfo, functionMem int64) {
	node.Lock()
	node.failedCnt += 1
	node.availableMemInBytes += functionMem
	node.Unlock()
}

// todo use stat info from node to predict func type[first priority]
func (r *Router) ReturnContainer(ctx context.Context, res *model.ResponseInfo) error {
	rmObj, ok := r.requestMap.Get(res.ID)
	if !ok {
		return errors.Errorf("no request found with id %s", res.ID)
	}
	fn := rmObj.(string)
	if (res.ErrorCode != "" || res.ErrorMessage != "") {
		logger.Errorf("ctn error for %s, ctnId %s, errorCd %s, errMsg %s",
			fn, res.ContainerId, res.ErrorCode, res.ErrorMessage)
		r.releaseCtn(fn, res.ContainerId)
		return nil
	}
	finfoObj, ok := r.fn2finfoMap.Get(fn)
	if !ok {
		return errors.Errorf("no func info for the fn %s", finfoObj)
	}
	finfo := finfoObj.(*model.FuncInfo)
	lastDuration := finfo.DurationInMs
	lastMaxMem := finfo.MaxMemoryUsageInBytes

	curentDuration := res.DurationInMs
	curentMaxMem := res.MaxMemoryUsageInBytes

	if (curentDuration > lastDuration) {
		//logger.Infof("ReturnContainer ctn for %s, update info 2: time %d",
		//	fn, curentDuration)
		finfo.DurationInMs = curentDuration
	}

	if (curentMaxMem > lastMaxMem) {
		//logger.Infof("ReturnContainer ctn for %s, update info 2: mem %d, req mem %d",
		//	fn, curentMaxMem, finfo.MemoryInBytes)
		finfo.MaxMemoryUsageInBytes = curentMaxMem
		finfo.ActualUsedMemInBytes = curentMaxMem + slack
	}

	ctnInfo, ok := r.ctn2info.Get(res.ContainerId)
	if !ok {
		return errors.Errorf("no container found with id %s", res.ContainerId)
	}
	container := ctnInfo.(*ExtendedContainerInfo)
	container.Lock()
	delete(container.requests, res.ID)
	container.Unlock()
	logger.Infof("fn %s %d %d, container: %f %f %f",
		fn, finfo.MaxMemoryUsageInBytes, finfo.DurationInMs,
		container.CpuUsagePct, container.MemoryUsageInBytes, container.TotalMemoryInBytes)
	r.requestMap.Remove(res.ID)
	//todo release node&ctn when ctn is idle long for pericaolly program
	// currently, don't release
	//r.releaseCtn(fn, res.ContainerId)
	return nil
}
func (r *Router) rmCtnFromFnMap(fn string, ctnId string) {
	// rm from fn2ctnSlice
	ctns, _ := r.fn2ctnSlice.Get(fn)
	ctnSlice := ctns.(*RwLockSlice)
	ctn_ids := ctnSlice.ctns

	outerIdx := -1
	for idx, val := range ctn_ids {
		if ctnId == val {
			outerIdx = idx
		}
	}
	if outerIdx == -1 {
		return
	}
	ctnSlice.Lock()
	ctn_ids = ctns.(*RwLockSlice).ctns
	ctnSlice.ctns = append(ctn_ids[:outerIdx], ctn_ids[outerIdx+1:]...)
	ctnSlice.Unlock()
}
func (r *Router) releaseCtn(fn string, ctnId string) {
	r.ctn2info.Remove(ctnId)
	go r.remoteReleaseCtn(ctnId)

	r.rmCtnFromFnMap(fn, ctnId)
}

func (r *Router) remoteReleaseCtn(ctnId string) {
	nodeWrapper, ok := r.cnt2node.Get(ctnId)
	if (!ok) {
		logger.Errorf("No cnt2node for %s", ctnId)
		return
	}
	// rm cnt2node
	r.cnt2node.Remove(ctnId)
	node := nodeWrapper.(*ExtendedNodeInfo)
	ctxR, cancelR := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelR()
	req := &nspb.RemoveContainerRequest{
		RequestId:   mockStr,
		ContainerId: ctnId,
	}
	reply, error := node.RemoveContainer(ctxR, req)
	if (error != nil) {
		logger.Errorf("RemoveContainer fail for %s, %s", ctnId, reply)
	}
}
func (r *Router) remoteReleaseNode(nid string) {
	ctxR, cancelR := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancelR()
	_, err := r.rmClient.ReleaseNode(ctxR, &rmPb.ReleaseNodeRequest{
		Id:        nid,
		RequestId: mockStr,
	})
	if err != nil {
		logger.WithFields(logger.Fields{
			"Operation": "remoteReleaseNode",
			"Error":     true,
		}).Errorf("Failed to remoteReleaseNode node due to %v", err)
	}
}

func sortNodeByUsage(values []*ExtendedNodeInfo) {
	sort.Slice(values, func(i, j int) bool {
		if (values[i].availableMemInBytes < values[j].availableMemInBytes) {
			return true
		}
		return values[i].failedCnt < values[j].failedCnt
	})
}
