package core

import (
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
	"context"
	"time"
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
		container.TotalMemoryInBytes = float64(ctnStat.TotalMemoryInBytes)
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
	globalFn2ctn := make(map[string]int)
	fn2ctn := make(map[string]int)

	for {
		<-ticker.C
		cl := len(funChan)
		logger.Infof("chan len %d", len(funChan))
		for i := 0; i < cl; i++ {
			fn := <-funChan
			if _, ok := globalFn2ctn[fn]; ok {
				globalFn2ctn[fn] += 1
			} else {
				globalFn2ctn[fn] = 1
			}
			if _, ok := fn2ctn[fn]; ok {
				fn2ctn[fn] += 1
			} else {
				fn2ctn[fn] = 1
			}
		}
		logger.Infof("fn qps local %v, global %v",
			fn2ctn, globalFn2ctn)
		for k := range fn2ctn {
			delete(fn2ctn, k)
		}
	}
}
