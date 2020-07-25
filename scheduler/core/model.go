package core

import (
	"github.com/orcaman/concurrent-map"
	"sync"
	"time"

	nspb "aliyun/serverless/mini-faas/nodeservice/proto"
	rmPb "aliyun/serverless/mini-faas/resourcemanager/proto"
)

type FuncCallMode int

//pre do action(swarm up or release quickly) according to stats or predict
const (
	Periodical FuncCallMode = iota // 0 周期型
	Sparse                         // 1 稀疏型
	Dense                          // 2 密集型
)

type FuncExeMode int

//choose adequate resource according to FuncExeMode
const (
	CpuIntensive FuncExeMode = iota // acquire resource as fast as , cpu locate
	MemIntensive                    // acquire resource as fast as , mem locate
	ResourceLess                    // reuse old container
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
	fn2ctnSlice cmap.ConcurrentMap // function_name -> RwLockSlice
	requestMap  cmap.ConcurrentMap // request_id -> FunctionName
	ctn2info    cmap.ConcurrentMap // ctn_id -> ExtendedContainerInfo
	cnt2node    cmap.ConcurrentMap // ctn_id -> ExtendedNodeInfo todo release node
	rmClient    rmPb.ResourceManagerClient
}

var slack int64 = 5 * 1000 * 1000

var mockStr = "mock-reqId/acctId"

var mockAccountId = mockStr

var getStatsReq = &nspb.GetStatsRequest{
	RequestId: mockStr,
}

var fetchStatsDuration = time.Duration(time.Millisecond * 300)
