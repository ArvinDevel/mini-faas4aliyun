package core

import (
	"fmt"
	"github.com/orcaman/concurrent-map"
	"math/rand"
	"sync"
	"time"

	nspb "aliyun/serverless/mini-faas/nodeservice/proto"
	rmPb "aliyun/serverless/mini-faas/resourcemanager/proto"
)

type ExtendedContainerInfo struct {
	sync.Mutex
	id       string
	address  string
	port     int64
	nodeId   string
	requests map[string]int64 // request_id -> status

	ReqMemoryInBytes float64 // set when req new ctn

	MemoryUsageInBytes float64 `protobuf:"varint,3,opt,name=memory_usage_in_bytes,json=memoryUsageInBytes,proto3" json:"memory_usage_in_bytes,omitempty"`
	CpuUsagePct        float64

	fn         string
	outlierCnt int
	usable     bool    `whether is usable used for indicate deleting`
	priority   float64 `(0-1]more less more high priority`

	SumDurationInMs float64
	Cnt             float64 `finished fn cnt`
}

type RwLockSlice struct {
	sync.RWMutex
	ctns []*ExtendedContainerInfo
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

const funTimeout = 60000

var slack int64 = 5 * 1000 * 1000

var gibyte float64 = 1024 * 1024 * 1024

var mockStr = "mock-reqId/acctId"

var staticAcctId = "1001210857578086"

var getStatsReq = &nspb.GetStatsRequest{
	RequestId: mockStr,
}

var fetchStatsDuration = time.Duration(time.Millisecond * 300)
// todo ajustment according to preorica
var releaseResourcesDuration = time.Duration(time.Second * 3)

var ctnCpuHighThreshold = 0.5
var ctnMemHighThreshold = 0.6
var parallelReqNum = 15

var nodeCpuHighThreshold = 150.0
var nodeMemHighThreshold = 0.75
var nodeFailedCntThreshold = 10
var reqQpsThreshold = 9
// todo 自适应调整阈值，防止为少量的突增的申请单独的node；
var reqOverThresholdNum = 20

var outlierThreshold = 1000

var calQpsDuration = time.Duration(time.Second)
var funChan = make(chan string, 10000)
var rtnCtnChan = make(chan interface{}, 1000)

var reScheduleDuration = time.Duration(time.Second * 10)

var random = rand.New(rand.NewSource(time.Now().UnixNano()))
// used to limit aquire req, too many parallel will hang node service
var globalLock = sync.Mutex{}

// util/helper
func (ctn *ExtendedContainerInfo) isCpuOrMemUsageHigh() bool {
	if ctn.MemoryUsageInBytes/ctn.ReqMemoryInBytes > ctnMemHighThreshold {
		return true
	}
	if ctn.CpuUsagePct > ctnCpuHighThreshold {
		return true
	}
	return false
}

func (ctn *ExtendedContainerInfo) String() string {
	return fmt.Sprintf("Ctn for %s, [%s,%s,%v],mem:%f/%f, cpu:%f ,active req: %d ,duration: %f/%f",
		ctn.fn, ctn.address, ctn.id, ctn.usable,
		ctn.MemoryUsageInBytes, ctn.ReqMemoryInBytes, ctn.CpuUsagePct,
		len(ctn.requests),
		ctn.SumDurationInMs, ctn.Cnt)
}
