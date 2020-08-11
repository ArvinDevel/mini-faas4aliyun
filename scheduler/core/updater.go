package core

import (
	"aliyun/serverless/mini-faas/scheduler/model"
	pb "aliyun/serverless/mini-faas/scheduler/proto"
	"context"
	"github.com/satori/go.uuid"
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
	statsResp, e := node.GetStats(ctx, getStatsReq)
	if (e != nil) {
		return
	}
	nodeStat := statsResp.NodeStats
	ctnStatList := statsResp.ContainerStatsList
	if (node.TotalMemoryInBytes == 0) {
		node.Lock()
		node.TotalMemoryInBytes = float64(nodeStat.TotalMemoryInBytes)
		node.Unlock()
	}
	node.AvailableMemoryInBytes = float64(nodeStat.AvailableMemoryInBytes)
	node.CpuUsagePct = nodeStat.CpuUsagePct
	node.MemoryUsageInBytes = float64(nodeStat.MemoryUsageInBytes)
	node.ctnCnt = len(ctnStatList)

	ctns := []*ExtendedContainerInfo{}
	for _, ctnStat := range ctnStatList {
		ctnInfo, ok := r.ctn2info.Get(ctnStat.ContainerId)
		if !ok {
			continue
		}
		container := ctnInfo.(*ExtendedContainerInfo)
		container.MemoryUsageInBytes = float64(ctnStat.MemoryUsageInBytes)
		container.CpuUsagePct = ctnStat.CpuUsagePct
		ctns = append(ctns, container)
	}
	for _, ctn := range ctns {
		finfoObj, _ := r.fn2finfoMap.Get(ctn.fn)
		finfo := finfoObj.(*model.FuncInfo)
		if ctn.CpuUsagePct > finfo.CpuThreshold {
			if finfo.Exemode != model.CpuIntensive {
				if finfo.Exemode == model.MemIntensive {
				} else {
					finfo.Exemode = model.CpuIntensive
					r.updateNodeInfo(ctn.fn, -finfo.CpuThreshold-2.0, finfo.MemoryInBytes-finfo.ActualUsedMemInBytes)
					r.checkAndTrigerExpand(ctn.fn, finfo.Qps, finfo.CpuThreshold)
				}
			}
		}
	}
}

func (r *Router) checkAndTrigerExpand(fn string, qps int, cpuThreshold float64) {
	rw, _ := r.fn2ctnSlice.Get(fn)
	ctnSlice := rw.(*RwLockSlice)
	currentCtnSize := len(ctnSlice.ctns)
	max := qps
	for _, ctn := range ctnSlice.ctns {
		avtiveReq := len(ctn.requests)
		if avtiveReq > max {
			max = avtiveReq
		}
	}
	if currentCtnSize < max {
		r.expand4MemIntensiveCtn(fn, max-currentCtnSize, cpuThreshold)
	}
}

func (r *Router) expand4MemIntensiveCtn(fn string, requireCnt int, cpuThreshod float64) {

	failedCnt := 0
	req := r.constructAcquireCtnReq(fn)
	for i := 0; i < requireCnt; i++ {
		node, _ := r.getNodeWithMemAndCpuCheck(staticAcctId, req.FunctionConfig.MemoryInBytes, cpuThreshod)
		if _, err := r.createNewCntOnNode(req, 1.0, node);
			err != nil {
			failedCnt++;
		}
	}
}

// when fn mode is update, update node resource
func (r *Router) updateNodeInfo(fn string, cpuDelta float64, memDelta int64) {
	for _, node := range values {
		if v, ok := node.fn2Cnt.Get(fn); ok {
			fnCnt := v.(int)
			node.Lock()
			node.availableCpu += float64(fnCnt) * cpuDelta
			node.availableMemInBytes += int64(fnCnt) * memDelta
			node.Unlock()
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
		r.updateFinfo(fn2cnt)
		for k := range fn2cnt {
			delete(fn2cnt, k)
		}
	}
}

func (r *Router) updateFinfo(fn2cnt map[string]int) {
	for fn, cnt := range fn2cnt {
		r.checkFn(fn)
		if cnt > reqQpsThreshold {
			finfoObj, _ := r.fn2finfoMap.Get(fn)
			finfo := finfoObj.(*model.FuncInfo)
			if finfo.Qps < cnt {
				finfo.Qps = cnt
			}
			finfo.DenseCnt += 1
			if finfo.CallMode != model.Dense && finfo.DenseCnt > reqOverThresholdNum {
				finfo.CallMode = model.Dense
				go r.boostCtnAction(fn)
				//if finfo.Exemode != model.CpuIntensive {
				//	go r.addNewNodeAndCtnsAction(fn)
				//}
			}
		}
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
			// todo fix bug:specify node
			r.CreateNewCntFromNode(req, 1.0)
		}
	}
}

func (r *Router) constructAcquireCtnReq(fn string) *pb.AcquireContainerRequest {
	finfoObj, _ := r.fn2finfoMap.Get(fn)
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
	finfoObj, _ := r.fn2finfoMap.Get(fn)
	finfo := finfoObj.(*model.FuncInfo)
	if finfo.Cnt == 0 {
		return
	}
	ratio := float64(finfo.SumDurationInMs/finfo.Cnt) / float64(finfo.MinDurationInMs)
	if ratio > 1.2 && finfo.Exemode != model.MemIntensive && finfo.Exemode != model.CpuIntensive {
		// todo check ctn parallel req, not all can solve by reduce parallel:transfer ctn from busy node
		r.checkOutlierCtn(fn, float64(finfo.SumDurationInMs/finfo.Cnt))
		//target := int(float64(parallelReqNum) / ratio)
		//if target < 1 {
		//	target = 1
		//}
		//if finfo.ReqThreshold != target {
		//	logger.Warningf("fn %v time over 20%, reduce reqThreshold to %d", fn, target)
		//	finfo.ReqThreshold = target
		//}
	}
}

func (r *Router) checkOutlierCtn(fn string, avgDuration float64) {
	rw, _ := r.fn2ctnSlice.Get(fn)
	ctnSlice := rw.(*RwLockSlice)
	ctns := ctnSlice.ctns
	avgUsages := []float64{}
	idx := 0
	max := avgDuration
	for id, ctn := range ctns {
		ctnAvgDuration := ctn.SumDurationInMs / ctn.Cnt
		if ctnAvgDuration > max {
			idx = id
			max = ctnAvgDuration
		}
		avgUsages = append(avgUsages, ctnAvgDuration)
	}
	if max/avgDuration > 2.0 {
		r.markCtnUnusedAndAcquireNewOne(ctns[idx], fn)
	}
}

func (r *Router) markCtnUnusedAndAcquireNewOne(toDeletedCtn *ExtendedContainerInfo, fn string) {
	toDeletedCtn.Lock()
	if toDeletedCtn.usable {
		toDeletedCtn.usable = false
		toDeletedCtn.Unlock()
	} else {
		toDeletedCtn.Unlock()
		return
	}
	go func() {
		for {
			if len(toDeletedCtn.requests) > 0 {
				time.Sleep(2 * time.Second)
			} else {
				r.releaseCtn(toDeletedCtn.fn, toDeletedCtn.id)
				break
			}
		}
	}()
	req := r.constructAcquireCtnReq(fn)
	// todo adjustment priority and use diff strategy for diff fn type
	r.CreateNewCntFromOnDemandNode(req, 1.0)
}
func (r *Router) ctnReplicaNum4Fn(fn string) int {
	ctns, _ := r.fn2ctnSlice.Get(fn)
	ctnSlice := ctns.(*RwLockSlice)
	return len(ctnSlice.ctns)
}
