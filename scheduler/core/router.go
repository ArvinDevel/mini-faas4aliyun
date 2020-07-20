package core

import (
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
	"context"
	"sort"
	"sync"
	"time"

	rmPb "aliyun/serverless/mini-faas/resourcemanager/proto"
	cp "aliyun/serverless/mini-faas/scheduler/config"
	"aliyun/serverless/mini-faas/scheduler/model"
	pb "aliyun/serverless/mini-faas/scheduler/proto"
	"github.com/orcaman/concurrent-map"
	"github.com/pkg/errors"
)

type ContainerInfo struct {
	sync.Mutex
	id       string
	address  string
	port     int64
	nodeId   string
	requests map[string]int64 // request_id -> status
}

type Router struct {
	nodeMap     cmap.ConcurrentMap // instance_id -> NodeInfo
	functionMap cmap.ConcurrentMap // function_name -> ContainerMap (container_id -> ContainerInfo)
	requestMap  cmap.ConcurrentMap // request_id -> FunctionName
	cnt2node    cmap.ConcurrentMap // ctn_id -> NodeInfo todo release node
	rmClient    rmPb.ResourceManagerClient
}

func NewRouter(config *cp.Config, rmClient rmPb.ResourceManagerClient) *Router {
	return &Router{
		nodeMap:     cmap.New(),
		functionMap: cmap.New(),
		requestMap:  cmap.New(),
		cnt2node:    cmap.New(),
		rmClient:    rmClient,
	}
}

func (r *Router) Start() {
	// Just in case the router has internal loops.
}

func (r *Router) AcquireContainer(ctx context.Context, req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	logger.Infof("AcquireContainer fn %s timeout %s mem %s",
		req.FunctionName, req.FunctionConfig.TimeoutInMs, req.FunctionConfig.MemoryInBytes)
	// Save the name for later ReturnContainer
	r.requestMap.Set(req.RequestId, req.FunctionName)
	r.functionMap.SetIfAbsent(req.FunctionName, cmap.New())

	funcExeMode := getFuncExeMode(req)
	return r.pickCntAccording2ExeMode(funcExeMode, req)
}

func (r *Router) getNode(accountId string, memoryReq int64) (*NodeInfo, error) {
	for _, key := range sortedKeys(r.nodeMap.Keys()) {
		nmObj, _ := r.nodeMap.Get(key)
		node := nmObj.(*NodeInfo)
		node.Lock()
		if node.availableMemInBytes > memoryReq {
			node.availableMemInBytes -= memoryReq
			node.Unlock()
			return node, nil
		}
		node.Unlock()
	}
	return r.remoteGetNode(accountId)
}

func (r *Router) remoteGetNode(accountId string) (*NodeInfo, error) {
	ctxR, cancelR := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelR()
	now := time.Now().UnixNano()
	replyRn, err := r.rmClient.ReserveNode(ctxR, &rmPb.ReserveNodeRequest{
		AccountId: accountId,
	})
	if err != nil {
		logger.WithFields(logger.Fields{
			"Operation": "ReserveNode",
			"Latency": (time.Now().UnixNano() - now)/1e6,
			"Error": true,
		}).Errorf("Failed to reserve node due to %v", err)
		return nil, errors.WithStack(err)
	}
	logger.WithFields(logger.Fields{
		"Operation": "ReserveNode",
		"Latency": (time.Now().UnixNano() - now)/1e6,
	}).Infof("")

	nodeDesc := replyRn.Node
	node, err := NewNode(nodeDesc.Id, nodeDesc.Address, nodeDesc.NodeServicePort, nodeDesc.MemoryInBytes)
	if err != nil {
		// TODO: Release the Node
		return nil, err
	}
	r.nodeMap.Set(nodeDesc.Id, node)
	return node, nil
}

func (r *Router) handleContainerErr(node *NodeInfo, functionMem int64) {
	node.Lock()
	node.availableMemInBytes += functionMem
	node.Unlock()
}

func (r *Router) ReturnContainer(ctx context.Context, res *model.ResponseInfo) error {
	logger.Infof("ReturnContainer fn %s ctn %s",
		res.FunctionName, res.ContainerId)
	rmObj, ok := r.requestMap.Get(res.ID)
	if !ok {
		return errors.Errorf("no request found with id %s", res.ID)
	}
	fmObj, ok := r.functionMap.Get(rmObj.(string))
	if !ok {
		return errors.Errorf("no container acquired for the request %s", res.ID)
	}
	containerMap := fmObj.(cmap.ConcurrentMap)
	cmObj, ok := containerMap.Get(res.ContainerId)
	if !ok {
		return errors.Errorf("no container found with id %s", res.ContainerId)
	}
	container := cmObj.(*ContainerInfo)
	container.Lock()
	delete(container.requests, res.ID)
	container.Unlock()
	r.requestMap.Remove(res.ID)
	// todo clean containerMap and cnt2node
	//// tmp out stats
	//for key,_ := range r.nodeMap.Keys() {
	//	node := r.nodeMap.Get(key).(*NodeInfo)
	//
	//	node.GetStats(ctx,)
	//}
	return nil
}

func sortedKeys(keys []string) []string {
	sort.Strings(keys)
	return keys
}
