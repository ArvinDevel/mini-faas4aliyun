package server

import (
	"fmt"
	"github.com/golang/groupcache/lru"
	"log"
	nodeproto "aliyun/serverless/mini-faas/nodeservice/proto"
	rmproto "aliyun/serverless/mini-faas/resourcemanager/proto"
	"aliyun/serverless/mini-faas/scheduler/proto"
)

var cacheSize = 1000
// funcName 2 container id
var fn2cntCache = lru.Cache{
	MaxEntries: cacheSize,
	OnEvicted:  evictAction,
}
var deletedCnt2stat = make(map[string]*schedulerproto.ReturnContainerRequest)
var unusedNodes [] *rmproto.NodeDesc
var usedNodes [] *rmproto.NodeDesc
var cnt2node = make(map[string]*rmproto.NodeDesc)

func PutContainer2node(cid string, node *rmproto.NodeDesc) {
	cnt2node[cid] = node
}

func rmContainer2node(cid string, usedMem int64) {
	node := cnt2node[cid]
	delete(cnt2node, cid)
	increaseUsedNodeStats(node, usedMem)
	// todo mv 2 unused cnt
}

func PutContainer(fnNm string, cid string) {
	fn2cntCache.Add(fnNm, cid)
}

func GetContainer(fnNm string) interface{} {
	if val, ok := fn2cntCache.Get(fnNm); ok {
		return val
	}
	return nil
}
func MarkDeletedCnt(req *schedulerproto.ReturnContainerRequest) {
	cid := req.ContainerId
	deletedCnt2stat[cid] = req
}

func evictAction(key lru.Key, value interface{}) {
	fmt.Printf("evict %s %v begin release cnt \n", key, value)
	remoteReleaseCnt(value.(string))
	clearCnt2node(value.(string))
}
func clearCnt2node(cid string) {
	if req, ok := deletedCnt2stat[cid]; ok {
		// todo cmp reqMem and mem here, use the consisent var
		rmContainer2node(cid, req.MaxMemoryUsageInBytes)
		delete(deletedCnt2stat, cid)
	} else {
		fmt.Printf("deleted cnt doen't has cnt %s \n", cid)
		return
	}
}
func remoteReleaseCnt(cid string) {
	// todo create grpc con with node
	//node := cnt2node[cid]
	req := nodeproto.RemoveContainerRequest{}
	req.ContainerId = cid
	_, err := dep.RemoveContainer(&req)
	if err != nil {
		log.Fatal("release cnt " + req.ContainerId)
	}
}

func PickNode(memReq int64) *rmproto.NodeDesc {
	if node := PickNodeFromUsed(memReq); node != nil {
		return node
	}
	node := PickNodeFromUnused(memReq)
	decreaseUsedNodeStats(node, memReq)
	usedNodes = append(usedNodes, node)
	return node
}
func PickNodeFromUsed(memReq int64) *rmproto.NodeDesc {
	// todo best fit
	for _, node := range usedNodes {
		if node.MemoryInBytes > memReq {
			decreaseUsedNodeStats(node, memReq)
			return node
		}
	}
	return nil
}

func decreaseUsedNodeStats(node *rmproto.NodeDesc, usedMem int64) {
	node.MemoryInBytes = node.MemoryInBytes - usedMem
}

func increaseUsedNodeStats(node *rmproto.NodeDesc, usedMem int64) {
	node.MemoryInBytes = node.MemoryInBytes + usedMem
}

func PickNodeFromUnused(memReq int64) *rmproto.NodeDesc {
	// todo sort before select according to mem resource
	for i, node := range unusedNodes {
		if node.MemoryInBytes > memReq {
			unusedNodes = append(unusedNodes[:i], unusedNodes[i+1:]...)
			return node
		}
	}
	return FetchNodeFromRemote(memReq)
}

func FetchNodeFromRemote(memReq int64) *rmproto.NodeDesc {
	req := rmproto.ReserveNodeRequest{}
	for {
		reply, err := dep.ReserveNode(&req)
		if err == nil {
			rn := reply.Node
			if rn.MemoryInBytes > memReq {
				return rn
			} else {
				fmt.Printf("fetched node %s 's mem is not enough", rn)
				unusedNodes = append(unusedNodes, rn)
			}
		}
	}
}
