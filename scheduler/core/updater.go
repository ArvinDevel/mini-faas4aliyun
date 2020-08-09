package core

import (
	"aliyun/serverless/mini-faas/scheduler/model"
	pb "aliyun/serverless/mini-faas/scheduler/proto"
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
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
		logger.Warningf(" %v mem high", node)
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
	for _, ctn := range ctns {
		finfoObj, _ := r.fn2finfoMap.Get(ctn.fn)
		finfo := finfoObj.(*model.FuncInfo)
		// todo 如果存在误报，需要增加次数来避免
		if ctn.CpuUsagePct > finfo.CpuThreshold {
			if finfo.Exemode != model.CpuIntensive {
				logger.Warningf("%v use more cpu on %v, change %v to cpu intensive",
					ctn, node, finfo)
				if finfo.Exemode == model.MemIntensive {
					logger.Warningf("fn %s is mem intensive, NOT change to cpu intensive",
						ctn.fn)
				} else {
					finfo.Exemode = model.CpuIntensive
					r.updateNodeInfo(ctn.fn, -finfo.CpuThreshold-2.0, finfo.MemoryInBytes-finfo.ActualUsedMemInBytes)
					r.checkAndTrigerExpand(ctn.fn, finfo.Qps, finfo.CpuThreshold)
				}
			}
			//else { // cause too much req
			//	r.checkAndTrigerExpand(ctn.fn, finfo.Qps, finfo.CpuThreshold)
			//}
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
			logger.Warningf("active req %d for %s large than ori max %d qps %d",
				avtiveReq, fn, max, qps)
			max = avtiveReq
		}
	}
	if currentCtnSize < max {
		r.expand4MemIntensiveCtn(fn, max-currentCtnSize, cpuThreshold)
	}
}

func (r *Router) expand4MemIntensiveCtn(fn string, requireCnt int, cpuThreshod float64) {
	logger.Infof("begin expand4MemIntensiveCtn for %s, %d", fn, requireCnt)

	failedCnt := 0
	req := r.constructAcquireCtnReq(fn)
	for i := 0; i < requireCnt; i++ {
		node, _ := r.getNodeWithMemAndCpuCheck(staticAcctId, req.FunctionConfig.MemoryInBytes, cpuThreshod)
		if _, err := r.createNewCntOnNode(req, 1.0, node);
			err != nil {
			failedCnt++;
		}
	}
	// todo add more again
	if failedCnt > 0 {
		logger.Warningf("expand failed %d requried %d for fn %s",
			failedCnt, requireCnt, fn)
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
		logger.Infof("fn qps %v", fn2cnt)
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
			finfoObj, ok := r.fn2finfoMap.Get(fn)
			if !ok {
				logger.Errorf("no func info for the fn %s when updateFinfo", fn)
				continue
			}
			finfo := finfoObj.(*model.FuncInfo)
			if finfo.Qps < cnt {
				logger.Infof("update %s qps from %d to %d", fn, finfo.Qps, cnt)
				finfo.Qps = cnt
			}
			finfo.DenseCnt += 1
			if finfo.CallMode != model.Dense && finfo.DenseCnt > reqOverThresholdNum {
				logger.Infof("change fn %s mode from %v to %v",
					fn, finfo.CallMode, model.Dense)
				finfo.CallMode = model.Dense
				go r.boostCtnAction(fn)
				//if finfo.Exemode != model.CpuIntensive {
				//	go r.addNewNodeAndCtnsAction(fn)
				//}
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
		r.createNewCntOnNode(req, 0.7, node)
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
		target := int(float64(parallelReqNum) / ratio)
		if target < 5 {
			target = 5
		}
		if finfo.ReqThreshold != target {
			logger.Warningf("fn %v time over 20%, reduce reqThreshold to %d", fn, target)
			finfo.ReqThreshold = target
		}
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
		// todo disable ctn and re aquire one
		logger.Warningf(" %v  %f of  avg for %s duration ",
			ctns[idx], max/avgDuration, fn)

	}
}

func (r *Router) releaseUnusedCtnAndAcuireNewOne(fn string) {
	rw, _ := r.fn2ctnSlice.Get(fn)
	ctnSlice := rw.(*RwLockSlice)
	currentCtnSize := len(ctnSlice.ctns)
	finfoObj, _ := r.fn2finfoMap.Get(fn)
	finfo := finfoObj.(*model.FuncInfo)
	qps := finfo.Qps
	if currentCtnSize < qps {
		logger.Infof("todo check whether need re-aquire ctn")
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
