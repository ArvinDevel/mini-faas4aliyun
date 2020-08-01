package core

import (
	"aliyun/serverless/mini-faas/scheduler/model"
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
	"context"
	"github.com/satori/go.uuid"
	"time"
	pb "aliyun/serverless/mini-faas/scheduler/proto"
)

func (node *ExtendedNodeInfo) UpdateStats(nodeID, address string, port, memory int64) {
	//node.GetStats()
}

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

	for _, ctnStat := range ctnStatList {
		ctnInfo, ok := r.ctn2info.Get(ctnStat.ContainerId)
		if !ok {
			//errors.Errorf("no container found with id %s", ctnStat.ContainerId)
			continue
		}
		container := ctnInfo.(*ExtendedContainerInfo)
		if (ctnStat.TotalMemoryInBytes == 0 || ctnStat.MemoryUsageInBytes == 0) {
			continue
		}
		container.Lock()
		container.MemoryUsageInBytes = float64(ctnStat.MemoryUsageInBytes)
		// 悲观估计 todo improve
		if (ctnStat.CpuUsagePct > 0) {
			container.CpuUsagePct = (container.CpuUsagePct + ctnStat.CpuUsagePct) / 2
		}
		container.Unlock()
	}
}

func (r *Router) CalQps() {
	// get stats async predicolly
	ticker := time.NewTicker(calQpsDuration)
	defer ticker.Stop()
	defer close(funChan)
	globalFn2cnt := make(map[string]int)
	fn2cnt := make(map[string]int)

	for {
		<-ticker.C
		cl := len(funChan)
		for i := 0; i < cl; i++ {
			fn := <-funChan
			if _, ok := globalFn2cnt[fn]; ok {
				globalFn2cnt[fn] += 1
			} else {
				globalFn2cnt[fn] = 1
			}
			if _, ok := fn2cnt[fn]; ok {
				fn2cnt[fn] += 1
			} else {
				fn2cnt[fn] = 1
			}
		}
		logger.Infof("fn qps local %v, global %v",
			fn2cnt, globalFn2cnt)
		r.updateFinfo(fn2cnt)
		for k := range fn2cnt {
			delete(fn2cnt, k)
		}
	}
}

// todo reschedule 不均衡的各个节点
func (r *Router) ReSchedule() {
}

var cntThreshold = reqQpsThreshold*10 - 5

func (r *Router) updateFinfo(fn2cnt map[string]int) {
	for fn, cnt := range fn2cnt {
		if cnt > cntThreshold {
			r.outputOutlierCtn(fn)
			finfoObj, ok := r.fn2finfoMap.Get(fn)
			if !ok {
				logger.Errorf("no func info for the fn %s when updateFinfo", fn)
				continue
			}
			finfo := finfoObj.(*model.FuncInfo)
			finfo.DenseCnt += 1
			if finfo.CallMode != model.Dense && finfo.DenseCnt > 6 {
				logger.Infof("change fn %s mode from %v to %v",
					fn, finfo.CallMode, model.Dense)
				finfo.CallMode = model.Dense
				// not affect ori logic to see diff
				//r.boostCtnAction(fn)
			}
		}
	}
}

func (r *Router) boostCtnAction(fn string) {
	rwLockSlice, _ := r.fn2ctnSlice.Get(fn)
	ctn_ids := rwLockSlice.(*RwLockSlice)
	usedNodeIds := []string{}
	ctn_ids.RLock()
	for _, val := range ctn_ids.ctns {
		cmObj, ok := r.ctn2info.Get(val)
		if (!ok) {
			logger.Errorf("No ctn info found 4 ctn %s when boostCtnAction", val)
			continue
		}
		container := cmObj.(*ExtendedContainerInfo)
		usedNodeIds = append(usedNodeIds, container.nodeId)
	}
	ctn_ids.RUnlock()

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
			ctn, err := r.CreateNewCntFromNode(req)
			if err == nil {
				ctn.Lock()
				delete(ctn.requests, req.RequestId)
				ctn.Unlock()
			}
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

func (r *Router) outputOutlierCtn(fn string) {
	rwLockSlice, _ := r.fn2ctnSlice.Get(fn)
	ctn_ids := rwLockSlice.(*RwLockSlice)
	ctns := []*ExtendedContainerInfo{}
	ctn_ids.RLock()
	for _, val := range ctn_ids.ctns {
		cmObj, ok := r.ctn2info.Get(val)
		if (!ok) {
			logger.Errorf("No ctn info found 4 ctn %s when outputOutlierCtn", val)
			continue
		}
		container := cmObj.(*ExtendedContainerInfo)
		ctns = append(ctns, container)
	}
	ctn_ids.RUnlock()
	total_cpu := 0.0
	total_mem := 0.0
	for _, ctn := range ctns {
		total_cpu += ctn.CpuUsagePct
		total_mem += ctn.MemoryUsageInBytes
	}
	size := float64(len(ctns))
	avgCpu := total_cpu / size
	avgCpuThreshold := avgCpu * 1.2
	avgMem := total_mem / size
	avgMemThreshold := avgMem * 1.2
	for _, ctn := range ctns {
		if ctn.CpuUsagePct > avgCpuThreshold {
			ctn.outlierCnt += 1
			if ctn.outlierCnt < 1000 {
				logger.Infof("ctn %v cpu over threshold %f", ctn, avgCpuThreshold)
			}
		}
		if ctn.MemoryUsageInBytes > avgMemThreshold {
			ctn.outlierCnt += 1
			if ctn.outlierCnt < 1000 {
				logger.Infof("ctn %v mem over threshold %f", ctn, avgMemThreshold)
			}
		}
	}
}
