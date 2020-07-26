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
	// todo release node :first mark, then delete

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
