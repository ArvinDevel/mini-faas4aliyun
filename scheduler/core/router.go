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

type ExtendedContainerInfo struct {
	sync.Mutex
	id       string
	address  string
	port     int64
	nodeId   string
	requests map[string]int64 // request_id -> status

	TotalMemoryInBytes int64 `protobuf:"varint,2,opt,name=total_memory_in_bytes,json=totalMemoryInBytes,proto3" json:"total_memory_in_bytes,omitempty"`
	MemoryUsageInBytes int64 `protobuf:"varint,3,opt,name=memory_usage_in_bytes,json=memoryUsageInBytes,proto3" json:"memory_usage_in_bytes,omitempty"`
	CpuUsagePct        float64

	fn string
}

type RwLockSlice struct {
	sync.RWMutex
	ctns []string
}
type Router struct {
	nodeMap cmap.ConcurrentMap // instance_id -> ExtendedNodeInfo
	// todo add stats info to fnInfo to predict func type
	fn2finfoMap cmap.ConcurrentMap // function_name -> FuncInfo(update info predically)
	functionMap cmap.ConcurrentMap // function_name -> RwLockSlice
	requestMap  cmap.ConcurrentMap // request_id -> FunctionName
	ctn2info    cmap.ConcurrentMap // ctn_id -> ExtendedContainerInfo
	cnt2node    cmap.ConcurrentMap // ctn_id -> ExtendedNodeInfo todo release node
	rmClient    rmPb.ResourceManagerClient
}

var slack int64 = 5 * 1000 * 1000

func NewRouter(config *cp.Config, rmClient rmPb.ResourceManagerClient) *Router {
	return &Router{
		nodeMap:     cmap.New(),
		fn2finfoMap: cmap.New(),
		functionMap: cmap.New(),
		requestMap:  cmap.New(),
		ctn2info:    cmap.New(),
		cnt2node:    cmap.New(),
		rmClient:    rmClient,
	}
}

func (r *Router) Start() {
	// Just in case the router has internal loops.
}

func (r *Router) AcquireContainer(ctx context.Context, req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	// Save the name for later ReturnContainer
	fn := req.FunctionName
	r.requestMap.Set(req.RequestId, fn)
	r.functionMap.SetIfAbsent(fn, &RwLockSlice{})
	r.fn2finfoMap.SetIfAbsent(fn, &model.FuncInfo{
		TimeoutInMs:   req.FunctionConfig.TimeoutInMs,
		MemoryInBytes: req.FunctionConfig.MemoryInBytes,
	})
	funcExeMode := getFuncExeMode(req)
	return r.pickCntAccording2ExeMode(funcExeMode, req)
}

func (r *Router) getNode(accountId string, memoryReq int64) (*ExtendedNodeInfo, error) {
	for _, key := range sortedKeys(r.nodeMap.Keys()) {
		nmObj, _ := r.nodeMap.Get(key)
		node := nmObj.(*ExtendedNodeInfo)
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

func (r *Router) remoteGetNode(accountId string) (*ExtendedNodeInfo, error) {
	ctxR, cancelR := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancelR()
	now := time.Now().UnixNano()
	replyRn, err := r.rmClient.ReserveNode(ctxR, &rmPb.ReserveNodeRequest{
		AccountId: accountId,
	})
	if err != nil {
		logger.WithFields(logger.Fields{
			"Operation": "ReserveNode",
			"Latency":   (time.Now().UnixNano() - now) / 1e6,
			"Error":     true,
		}).Errorf("Failed to reserve node due to %v", err)
		return nil, errors.WithStack(err)
	}
	logger.WithFields(logger.Fields{
		"Operation": "ReserveNode",
		"Latency":   (time.Now().UnixNano() - now) / 1e6,
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

func (r *Router) handleContainerErr(node *ExtendedNodeInfo, functionMem int64) {
	node.Lock()
	node.availableMemInBytes += functionMem
	node.Unlock()
}

// todo use stat info from node to predict func type[first priority]
func (r *Router) ReturnContainer(ctx context.Context, res *model.ResponseInfo) error {
	rmObj, ok := r.requestMap.Get(res.ID)
	if !ok {
		return errors.Errorf("no request found with id %s", res.ID)
	}
	fn := rmObj.(string)
	if (res.ErrorCode != "") {
		logger.Errorf("ctn error for %s, reqId %s, errorCd %s, errMsg %s",
			fn, res.ID, res.ErrorCode, res.ErrorMessage)
	}
	finfoObj, ok := r.fn2finfoMap.Get(fn)
	if !ok {
		return errors.Errorf("no func info for the fn %s", finfoObj)
	}
	finfo := finfoObj.(*model.FuncInfo)
	lastDuration := finfo.DurationInNanos
	lastMaxMem := finfo.MaxMemoryUsageInBytes

	curentDuration := res.DurationInNanos
	curentMaxMem := res.MaxMemoryUsageInBytes

	if (curentDuration > lastDuration) {
		logger.Infof("ReturnContainer ctn for %s, update info 2: time %d",
			fn, curentDuration)
		finfo.DurationInNanos = curentDuration
	}

	if (curentMaxMem > lastMaxMem) {
		logger.Infof("ReturnContainer ctn for %s, update info 2: mem %d, req mem %d",
			fn, curentMaxMem, finfo.MemoryInBytes)
		finfo.MaxMemoryUsageInBytes = curentMaxMem
		finfo.ActualUsedMemInBytes = curentMaxMem + slack
	}

	ctnInfo, ok := r.ctn2info.Get(res.ContainerId)
	if !ok {
		return errors.Errorf("no container found with id %s", res.ContainerId)
	}
	container := ctnInfo.(*ExtendedContainerInfo)
	container.Lock()
	delete(container.requests, res.ID)
	container.Unlock()
	r.requestMap.Remove(res.ID)
	go r.UpdateStats()
	// todo clean containerMap and cnt2node
	return nil
}

func sortedKeys(keys []string) []string {
	sort.Strings(keys)
	return keys
}
