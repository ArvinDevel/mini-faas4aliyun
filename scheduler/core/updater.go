package core

import (
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
	"context"
	"github.com/pkg/errors"
	"time"

	pb "aliyun/serverless/mini-faas/nodeservice/proto"
)


func (node *ExtendedNodeInfo) UpdateStats(nodeID, address string, port, memory int64) {
	//node.GetStats()
}

var statsReq = &pb.GetStatsRequest{
	RequestId: "mock-reqId",
}

func (r *Router) UpdateStats() {
	// get stats async
	for _, key := range r.nodeMap.Keys() {
		nmObj, _ := r.nodeMap.Get(key)
		node := nmObj.(*ExtendedNodeInfo)

		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()
		statsResp, error := node.GetStats(ctx, statsReq)
		if (error != nil) {
			logger.Errorf("GetStats from %s fail", node.nodeID)
			continue
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
				errors.Errorf("no container found with id %s", ctnStat.ContainerId)
				continue
			}
			container := ctnInfo.(*ExtendedContainerInfo)
			//container.Lock()
			//container.Unlock()
			container.TotalMemoryInBytes = ctnStat.TotalMemoryInBytes
			container.MemoryUsageInBytes = ctnStat.MemoryUsageInBytes
			container.CpuUsagePct = ctnStat.CpuUsagePct
		}
	}

}
