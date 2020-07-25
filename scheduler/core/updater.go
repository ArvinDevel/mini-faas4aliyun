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
		node.TotalMemoryInBytes = nodeStat.TotalMemoryInBytes
		node.Unlock()
	}
	// todo check whether need add lock
	node.AvailableMemoryInBytes = nodeStat.AvailableMemoryInBytes
	node.CpuUsagePct = nodeStat.CpuUsagePct
	node.MemoryUsageInBytes = nodeStat.MemoryUsageInBytes
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
		container.TotalMemoryInBytes = ctnStat.TotalMemoryInBytes
		container.MemoryUsageInBytes = ctnStat.MemoryUsageInBytes
		// todo use avg? may not acurate
		container.CpuUsagePct = ctnStat.CpuUsagePct
		container.Unlock()
	}
}
