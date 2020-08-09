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

// todo 1 use 30s accumulated req and max static locate;
// 2 dynamic reschedule when req tps is small
// 3 auto aquire node when node is not necessary to avoid shepe, 1s ticker?
// 4. record qps and dump node usage every 3 min?
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
	r.warmup(9)
	go r.UpdateStats()
	go r.CalQps()
	//go func() {
	//	for i := 0; i < 10; i++ {
	//		time.Sleep(time.Duration(30 * time.Second))
	//		go r.remoteGetNode(staticAcctId, 0)
	//	}
	//}()
	go func() {
		for {
			select {
			case ctn := <-rtnCtnChan:
				r.returnContainer(ctn.(*model.ResponseInfo))
			}
		}
	}()
}

func (r *Router) warmup(num int) {
	// to avoid bootstrap swarm up and medium swarm both aquire when acctId change
	acctId := staticAcctId
	for i := 0; i < num; i++ {
		go func() {
			_, err := r.remoteGetNode(acctId)
			if err != nil {
				time.Sleep(30 * time.Second)
				_, err2 := r.remoteGetNode(acctId)
				if err2 != nil {
					logger.Errorf("after sleep 30s still can't acquire node")
				}
			}
		}()
	}
}

//todo use state machine to simulate fn to avoid aquire multiple ctns,
// and motivate expand and shink, pass finfo to core maybe
func (r *Router) AcquireContainer(ctx context.Context, req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	// Save the name for later ReturnContainer
	fn := req.FunctionName
	funChan <- fn
	r.requestMap.Set(req.RequestId, fn)
	r.fn2ctnSlice.SetIfAbsent(fn, &RwLockSlice{
		ctns: make([]*ExtendedContainerInfo, 0, 15),
	})
	r.fn2finfoMap.SetIfAbsent(fn, &model.FuncInfo{
		TimeoutInMs:       req.FunctionConfig.TimeoutInMs,
		MemoryInBytes:     req.FunctionConfig.MemoryInBytes,
		Handler:           req.FunctionConfig.Handler,
		MinDurationInMs:   2000000,
		TimeOverThreshold: false,
		CpuThreshold:      float64(req.FunctionConfig.MemoryInBytes)/gibyte*67 - 2,
		ReqThreshold:      parallelReqNum,
	})
	finfoObj, _ := r.fn2finfoMap.Get(fn)
	finfo := finfoObj.(*model.FuncInfo)
	return r.pickCntAccording2ExeMode(finfo.Exemode, req)
}

var values = []*ExtendedNodeInfo{}

func (r *Router) getNode(accountId string, memoryReq int64) (*ExtendedNodeInfo, error) {
	length := len(values)
	// todo best fit
	for i := 0; i < length; i++ {
		idx := random.Intn(length)
		node := values[idx]
		node.Lock()
		// todo exclude memintensive fn 限制超卖上限
		if node.AvailableMemoryInBytes > 2*float64(memoryReq) && node.availableMemInBytes > -1000000000 {
			node.availableMemInBytes -= memoryReq
			node.Unlock()
			return node, nil
		}
		if node.availableMemInBytes > memoryReq {
			node.availableMemInBytes -= memoryReq
			node.Unlock()
			return node, nil
		}
		node.Unlock()
	}
	logger.Infof("current nodes %s can't affoard %d", values, memoryReq)
	// only used for local
	if accountId != staticAcctId {
		logger.Errorf("acctId changed from %s to %s", staticAcctId, accountId)
		staticAcctId = accountId
		r.remoteGetNode(accountId)
		r.warmup(8)
	}
	if len(values) < 15 {
		logger.Warningf("begin expand node when no node satisfy")
		go r.remoteGetNode(staticAcctId)
	}
	if len(values) > 0 {
		return r.fallbackUseLocalNode()
	}
	return nil, errors.Errorf("NO NODE!")
}

func (r *Router) getNodeWithMemAndCpuCheck(accountId string, memoryReq int64, cpuThreshod float64) (*ExtendedNodeInfo, error) {
	sort.Slice(values, func(i, j int) bool {
		return values[i].availableCpu > values[j].availableCpu
	})
	for _, node := range values {
		if node.availableCpu < cpuThreshod {
			break
		}
		// 不超卖
		node.Lock()
		if node.availableMemInBytes > memoryReq {
			node.availableMemInBytes -= memoryReq
			node.availableCpu -= cpuThreshod
			node.Unlock()
			return node, nil
		}
		node.Unlock()
	}
	logger.Infof("getNodeWithMemAndCpuCheck current nodes %s can't affoard %d", values, memoryReq)

	if len(values) < 18 {
		logger.Warningf("getNodeWithMemAndCpuCheck begin expand node when no node satisfy")
		node, err := r.remoteGetNode(staticAcctId)
		if err == nil {
			return node, nil
		}
	}
	sort.Slice(values, func(i, j int) bool {
		return values[i].availableCpu > values[j].availableCpu
	})
	return values[0], nil
}

func (r *Router) remoteGetNode(accountId string) (*ExtendedNodeInfo, error) {
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
	node, err := NewNode(nodeDesc.Id, nodeDesc.Address, nodeDesc.NodeServicePort, nodeDesc.MemoryInBytes)
	logger.Infof("ReserveNode accntId %s %s Latency %d",
		accountId, node, (time.Now().UnixNano()-now)/1e6)
	if err != nil {
		go r.remoteReleaseNode(nodeDesc.Id)
		time.Sleep(time.Duration(30 * time.Second))
		logger.Errorf("ReserveNode fail", err)
		return nil, err
	}
	values = append(values, node)
	r.nodeMap.Set(nodeDesc.Id, node)
	return node, nil
}

func (r *Router) fallbackUseLocalNode() (*ExtendedNodeInfo, error) {
	// choose more
	sort.Slice(values, func(i, j int) bool {
		if values[i].CpuUsagePct > nodeCpuHighThreshold {
			return false
		}
		if values[i].MemoryUsageInBytes/values[i].TotalMemoryInBytes > nodeMemHighThreshold {
			return false
		}
		aMem := values[i].AvailableMemoryInBytes
		aCpu := values[i].CpuUsagePct
		aVal := aMem*0.8 + (200-aCpu)*0.2
		bMem := values[j].AvailableMemoryInBytes
		bCpu := values[j].CpuUsagePct
		bVal := bMem*0.8 + (200-bCpu)*0.2
		return aVal > bVal
	})
	logger.Infof("fallbackUseLocalNode %v", values[0])
	return values[0], nil
}

func (r *Router) handleContainerErr(node *ExtendedNodeInfo, functionMem int64) {
	node.Lock()
	node.availableMemInBytes += functionMem
	node.Unlock()
}

func (r *Router) ReturnContainer(res *model.ResponseInfo) {
	rtnCtnChan <- res
}

// todo use stat info from node to predict func type[first priority]
func (r *Router) returnContainer(res *model.ResponseInfo) error {
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
		return errors.Errorf("no func info for the fn %s", fn)
	}
	finfo := finfoObj.(*model.FuncInfo)

	curentDuration := res.DurationInMs
	curentMaxMem := res.MaxMemoryUsageInBytes

	if (curentDuration > finfo.MaxDurationInMs) {
		finfo.MaxDurationInMs = curentDuration
	}

	if (curentMaxMem > finfo.MaxMemoryUsageInBytes) {
		finfo.MaxMemoryUsageInBytes = curentMaxMem
		finfo.ActualUsedMemInBytes = curentMaxMem + slack
	}

	if curentDuration < finfo.MinDurationInMs {
		finfo.MinDurationInMs = curentDuration
	}

	finfo.SumDurationInMs += curentDuration
	finfo.Cnt += 1

	// update fun exe mode
	memUsage := float64(finfo.MaxMemoryUsageInBytes) / float64(finfo.MemoryInBytes)
	if finfo.Exemode != model.MemIntensive && memUsage > ctnMemHighThreshold {
		logger.Infof("update %s mode from %d to %d, since %f",
			fn, finfo.Exemode, model.MemIntensive, memUsage)
		finfo.Exemode = model.MemIntensive
	}

	ctnInfo, ok := r.ctn2info.Get(res.ContainerId)
	if !ok {
		return errors.Errorf("no container found with id %s", res.ContainerId)
	}
	container := ctnInfo.(*ExtendedContainerInfo)
	container.SumDurationInMs += float64(curentDuration)
	container.Cnt += 1
	delete(container.requests, res.ID)
	rw, _ := r.fn2ctnSlice.Get(fn)
	ctnSlice := rw.(*RwLockSlice)
	logger.Infof("ReturnContainer %d, %v, %v, ctns size :%d",
		curentDuration, finfo, container, len(ctnSlice.ctns))
	r.requestMap.Remove(res.ID)
	//todo release node&ctn when ctn is idle long for pericaolly program
	// currently, don't release
	//r.releaseCtn(fn, res.ContainerId)
	return nil
}
func (r *Router) rmCtnFromFnMap(fn string, ctnId string) {
	// rm from fn2ctnSlice
	rw, _ := r.fn2ctnSlice.Get(fn)
	ctnSlice := rw.(*RwLockSlice)
	ctns := ctnSlice.ctns

	outerIdx := -1
	for idx, val := range ctns {
		if ctnId == val.id {
			outerIdx = idx
		}
	}
	if outerIdx == -1 {
		return
	}
	ctnSlice.Lock()
	ctnSlice.ctns = append(ctns[:outerIdx], ctns[outerIdx+1:]...)
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
	logger.Infof("node info %s for ctn %s", node, ctnId)
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
		if values[i].CpuUsagePct > nodeCpuHighThreshold {
			return false
		}
		if values[i].MemoryUsageInBytes/values[i].TotalMemoryInBytes > nodeMemHighThreshold {
			return false
		}
		if (values[i].AvailableMemoryInBytes > 0 && values[j].AvailableMemoryInBytes > 0) {
			// choose 松裕的，防止雪崩，压垮小node
			if (values[i].AvailableMemoryInBytes > values[j].AvailableMemoryInBytes) {
				return true
			}
		}
		if (values[i].availableMemInBytes > values[j].availableMemInBytes) {
			return true
		}
		return true
	})
}
