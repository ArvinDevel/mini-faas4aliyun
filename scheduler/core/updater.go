package core

import (
	"aliyun/serverless/mini-faas/scheduler/model"
	pb "aliyun/serverless/mini-faas/scheduler/proto"
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
	"context"
	"github.com/satori/go.uuid"
	"sort"
	"time"
)

func (r *Router) UpdateStats() {
	// get stats async predicolly
	ticker := time.NewTicker(fetchStatsDuration)
	defer ticker.Stop()
	for {
		<-ticker.C
		for _, key := range r.nodeMap.Keys() {
			nmObj, _ := r.nodeMap.Get(key)
			node := nmObj.(*ExtendedNodeInfo)
			r.UpdateSignleNode(node)
		}
	}
}

func (r *Router) UpdateSignleNode(node *ExtendedNodeInfo) {
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	statsResp, error := node.GetStats(ctx, getStatsReq)
	if (error != nil) {
		logger.Errorf("GetStats from %s %s fail due to %v",
			node.nodeID, node.address, error)
		return
	}
	nodeStat := statsResp.NodeStats
	ctnStatList := statsResp.ContainerStatsList
	if (node.TotalMemoryInBytes == 0) {
		node.Lock()
		node.TotalMemoryInBytes = float64(nodeStat.TotalMemoryInBytes)
		node.Unlock()
	}
	// todo check whether need add lock
	node.AvailableMemoryInBytes = float64(nodeStat.AvailableMemoryInBytes)
	node.CpuUsagePct = nodeStat.CpuUsagePct
	node.MemoryUsageInBytes = float64(nodeStat.MemoryUsageInBytes)
	node.ctnCnt = len(ctnStatList)

	if node.isCpuOrMemUsageHigh() {
		logger.Warningf("node %v warn", node)
	}

	ctns := []*ExtendedContainerInfo{}
	for _, ctnStat := range ctnStatList {
		ctnInfo, ok := r.ctn2info.Get(ctnStat.ContainerId)
		if !ok {
			//errors.Errorf("no container found with id %s", ctnStat.ContainerId)
			continue
		}
		container := ctnInfo.(*ExtendedContainerInfo)
		container.MemoryUsageInBytes = float64(ctnStat.MemoryUsageInBytes)
		container.CpuUsagePct = ctnStat.CpuUsagePct
		ctns = append(ctns, container)
	}
	//sort.Slice(ctns, func(i, j int) bool {
	//	return ctns[i].MemoryUsageInBytes > ctns[j].MemoryUsageInBytes
	//})
	//if len(ctns) > 1 {
	//	if ctns[0].MemoryUsageInBytes/ctns[1].MemoryUsageInBytes > 1.5 {
	//		logger.Warningf("%v use more mem on %s", ctns[0], node)
	//	}
	//}

	sort.Slice(ctns, func(i, j int) bool {
		return ctns[i].CpuUsagePct > ctns[j].CpuUsagePct
	})
	if len(ctns) > 1 {
		if ctns[0].CpuUsagePct/ctns[1].CpuUsagePct > 1.5 && ctns[0].outlierCnt < outlierThreshold {
			logger.Warningf("%v use more cpu on %s", ctns[0], node)
			// todo reschedule this
		}
	}
}

func (r *Router) CalQps() {
	// get stats async predicolly
	ticker := time.NewTicker(calQpsDuration)
	defer ticker.Stop()
	defer close(funChan)
	fn2cnt := make(map[string]int)

	for {
		<-ticker.C
		cl := len(funChan)
		for i := 0; i < cl; i++ {
			fn := <-funChan
			if _, ok := fn2cnt[fn]; ok {
				fn2cnt[fn] += 1
			} else {
				fn2cnt[fn] = 1
			}
		}
		logger.Infof("fn qps %v", fn2cnt)
		r.updateFinfo(fn2cnt)
		for k := range fn2cnt {
			delete(fn2cnt, k)
		}
	}
}

// todo reschedule 不均衡的各个节点
func (r *Router) ReSchedule() {
}

func (r *Router) updateFinfo(fn2cnt map[string]int) {
	for fn, cnt := range fn2cnt {
		r.checkFn(fn)
		if cnt > reqQpsThreshold {
			finfoObj, ok := r.fn2finfoMap.Get(fn)
			if !ok {
				logger.Errorf("no func info for the fn %s when updateFinfo", fn)
				continue
			}
			finfo := finfoObj.(*model.FuncInfo)
			finfo.DenseCnt += 1
			if finfo.CallMode != model.Dense && finfo.DenseCnt > reqOverThresholdNum {
				logger.Infof("change fn %s mode from %v to %v",
					fn, finfo.CallMode, model.Dense)
				finfo.CallMode = model.Dense
				go r.boostCtnAction(fn)
				go r.addNewNodeAndCtnsAction(fn)
			}
		}
	}
}

func (r *Router) addNewNodeAndCtnsAction(fn string) {
	logger.Infof("exe addNewNodeAndCtnsAction for %s ", fn)
	var node *ExtendedNodeInfo
	for
	{
		if n, err := r.remoteGetNode(staticAcctId); err == nil {
			node = n
			break
		}
		time.Sleep(time.Second * 30)
	}
	finfoObj, ok := r.fn2finfoMap.Get(fn)
	if !ok {
		logger.Errorf("no func info for the fn %s when addNewNodeAndCtnsAction", fn)
		return
	}
	finfo := finfoObj.(*model.FuncInfo)

	ctnSize := node.availableMemInBytes / finfo.MemoryInBytes

	for i := int64(0); i < ctnSize; i++ {
		req := r.constructAcquireCtnReq(fn)
		if req == nil {
			logger.Errorf("constructAcquireCtnReq when addNewNodeAndCtnsAction for %s", fn)
			continue
		}
		r.CreateNewCntFromNode(req, 0.7)
	}
}

func (r *Router) boostCtnAction(fn string) {
	rwLockSlice, _ := r.fn2ctnSlice.Get(fn)
	lockSlice := rwLockSlice.(*RwLockSlice)
	usedNodeIds := []string{}
	ctns := lockSlice.ctns
	for _, container := range ctns {
		usedNodeIds = append(usedNodeIds, container.nodeId)
	}

	for key := range r.nodeMap.Keys() {
		noCtn := true
		for usedNodeIds := range usedNodeIds {
			if usedNodeIds == key {
				noCtn = false
				break
			}
		}
		if noCtn {
			req := r.constructAcquireCtnReq(fn)
			if req == nil {
				continue
			}
			logger.Infof("begin add one ctn for fn %s", fn)
			r.CreateNewCntFromNode(req, 1.0)
		}
	}
}

func (r *Router) constructAcquireCtnReq(fn string) *pb.AcquireContainerRequest {
	finfoObj, ok := r.fn2finfoMap.Get(fn)
	if !ok {
		logger.Errorf("no func info for the fn %s when constructAcquireCtnReq", fn)
		return nil
	}
	finfo := finfoObj.(*model.FuncInfo)
	funcConfig := &pb.FunctionConfig{
		Handler:       finfo.Handler,
		TimeoutInMs:   finfo.TimeoutInMs,
		MemoryInBytes: finfo.MemoryInBytes,
	}
	reqId := uuid.NewV4().String()
	return &pb.AcquireContainerRequest{
		AccountId:      staticAcctId,
		FunctionName:   fn,
		FunctionConfig: funcConfig,
		RequestId:      reqId,
	}
}

func (r *Router) checkFn(fn string) {
	finfoObj, ok := r.fn2finfoMap.Get(fn)
	if !ok {
		logger.Errorf("no func info for the fn %s when checkFn", fn)
		return
	}
	finfo := finfoObj.(*model.FuncInfo)
	if finfo.Cnt == 0 {
		return
	}
	ratio := float64(finfo.SumDurationInMs/finfo.Cnt) / float64(finfo.MinDurationInMs)
	if ratio > 1.2 && !finfo.TimeOverThreshold {
		logger.Warningf("fn %v time over 20%, change state 2 over", fn)
		finfo.TimeOverThreshold = true
		// todo if fn is peroricall, then reschedule it
	}
	if ratio <= 1.2 && finfo.TimeOverThreshold {
		logger.Warningf("fn %v time over recover , change state ", fn)
		finfo.TimeOverThreshold = false
	}
}
func (r *Router) ReleaseCtnResource() {
	// release unused ctn async predicolly:only keep one replica
	// NOT APPLYED for mem intensive:
	ticker := time.NewTicker(releaseResourcesDuration)
	defer ticker.Stop()
	for {
		<-ticker.C
		for _, key := range r.ctn2info.Keys() {
			ctnInfo, _ := r.ctn2info.Get(key)
			container := ctnInfo.(*ExtendedContainerInfo)
			fn := container.fn
			// consider fn container replica, todo consider fun frequency and duration
			if r.ctnReplicaNum4Fn(fn) < 2 {
				continue
			}
			container.Lock()
			if len(container.requests) == 0 {
				// after set this, we can safetly release it
				container.usable = false
			}
			container.Unlock()
			if !container.usable {
				logger.Infof("begin release %s", container)
				r.releaseCtn(container.fn, container.id)
			}
		}
	}
}

func (r *Router) ctnReplicaNum4Fn(fn string) int {
	ctns, _ := r.fn2ctnSlice.Get(fn)
	ctnSlice := ctns.(*RwLockSlice)
	return len(ctnSlice.ctns)
}
