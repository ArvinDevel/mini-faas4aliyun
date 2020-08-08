package core

import (
	"aliyun/serverless/mini-faas/scheduler/model"
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
	"context"
	"sort"

	nsPb "aliyun/serverless/mini-faas/nodeservice/proto"
	pb "aliyun/serverless/mini-faas/scheduler/proto"
	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
	"time"
)

func (r *Router) pickCntAccording2ExeMode(exeMode model.FuncExeMode, req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	switch exeMode {
	case model.CpuIntensive:
		return r.pickCnt4CpuIntensive(req, exeMode);
	case model.MemIntensive:
		return r.pickCnt4MemIntensive(req, exeMode);
	case model.ResourceLess:
	case model.None:
		return r.pickCnt4ResourceLess(req);
	}
	logger.Errorf("Unrecognized exeMode %s", exeMode)
	return nil, errors.Errorf("Unrecognized exeMode %s", exeMode)
}

func (r *Router) pickCnt4ResourceLess(req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	return r.pickCnt4ParallelReq(req);
}

func (r *Router) pickCnt4CpuIntensive(req *pb.AcquireContainerRequest, exemode model.FuncExeMode) (*pb.AcquireContainerReply, error) {
	// cpu intensive: reduce mem usage, guarantee cpu by use serial strategy
	r.reduceReqMem(req)
	return r.pickCnt4SerialReq(req, exemode);
}

func (r *Router) reduceReqMem(req *pb.AcquireContainerRequest) {
	fn := req.FunctionName
	finfoObj, ok := r.fn2finfoMap.Get(fn)
	if ok {
		finfo := finfoObj.(*model.FuncInfo)
		actualUsedMem := finfo.ActualUsedMemInBytes
		if (actualUsedMem > 0 && actualUsedMem < finfo.MemoryInBytes) {
			req.FunctionConfig.MemoryInBytes = actualUsedMem
			logger.Infof("change req mem from %d to %d for fn %s",
				finfo.MemoryInBytes, actualUsedMem, fn)
		}
	}
}

func (r *Router) pickCnt4MemIntensive(req *pb.AcquireContainerRequest, exemode model.FuncExeMode) (*pb.AcquireContainerReply, error) {
	return r.pickCnt4SerialReq(req, exemode);
}

// 只适合串行执行的：资源竞争激烈的，cpu/mem占用率极高
func (r *Router) pickCnt4SerialReq(req *pb.AcquireContainerRequest, exemode model.FuncExeMode) (*pb.AcquireContainerReply, error) {
	var res *ExtendedContainerInfo

	fn := req.FunctionName
	// if not ok, panic
	rwSlice, _ := r.fn2ctnSlice.Get(fn)

	rwLockSlice := rwSlice.(*RwLockSlice)

	ctns := rwLockSlice.ctns
	for _, val := range ctns {
		container := val
		container.Lock()
		if container.usable && len(container.requests) < 1 {
			container.requests[req.RequestId] = 1
			res = container
			container.Unlock()
			break
		}
		container.Unlock()
	}

	if res == nil { // if no idle container exists
		logger.Infof("%d ctns  can't provide for %s", len(ctns), fn)
		go r.CreateNewCntFromNode(req, 1.0)
		for {
			if len(rwLockSlice.ctns) > 0 {
				if exemode == model.MemIntensive {
					res = r.fallbackChooseCtn4MemIntensive(req.RequestId, rwLockSlice.ctns)
					if res != nil {
						break
					}
				} else {
					res = fallbackChooseCtn(rwLockSlice.ctns)
					break

				}
			}
		}
	}

	res.Lock()
	res.requests[req.RequestId] = 1
	res.Unlock()
	return &pb.AcquireContainerReply{
		NodeId:          res.nodeId,
		NodeAddress:     res.address,
		NodeServicePort: res.port,
		ContainerId:     res.id,
	}, nil
}

func (r *Router) pickCnt4ParallelReq(req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	var res *ExtendedContainerInfo

	fn := req.FunctionName
	finfoObj, _ := r.fn2finfoMap.Get(fn)
	finfo := finfoObj.(*model.FuncInfo)
	reqThreshold := finfo.ReqThreshold
	// if not ok, panic
	rwLockSlice, _ := r.fn2ctnSlice.Get(fn)

	lockSlice := rwLockSlice.(*RwLockSlice)

	ctns := lockSlice.ctns
	if len(ctns) > 0 {
		sortCtnByMemUsage(ctns)
	}
	for _, ctn := range ctns {
		//if ctn.isCpuOrMemUsageHigh() {
		//	continue
		//}
		ctn.Lock()
		if ctn.usable && len(ctn.requests) < reqThreshold {
			ctn.requests[req.RequestId] = 1
			res = ctn
			ctn.Unlock()
			break
		}
		ctn.Unlock()
	}
	if res == nil { // if no idle container exists
		logger.Infof("%d ctns  can't provide for %s", len(ctns), fn)
		if len(lockSlice.ctns) == 0 {
			r.createNewCntInEveryNode(req, 1.0)
		}
		if len(lockSlice.ctns) >= len(values) {
			logger.Warningf("begin CreateNewCntFromNode for %s,%v", fn, finfo)
			go r.CreateNewCntFromNode(req, 1.0)
		}
		for {
			if len(lockSlice.ctns) > 0 {
				res = fallbackChooseCtn(lockSlice.ctns)
				break
			}
		}
	}
	res.Lock()
	res.requests[req.RequestId] = 1
	res.Unlock()
	return &pb.AcquireContainerReply{
		NodeId:          res.nodeId,
		NodeAddress:     res.address,
		NodeServicePort: res.port,
		ContainerId:     res.id,
	}, nil
}

func fallbackChooseCtn(ctns []*ExtendedContainerInfo) *ExtendedContainerInfo {
	size := len(ctns)
	idx := random.Intn(size)
	return ctns[idx]
}

func (r *Router) fallbackChooseCtn4MemIntensive(req string, ctns []*ExtendedContainerInfo) *ExtendedContainerInfo {
	var res *ExtendedContainerInfo

	for _, container := range ctns {
		container.Lock()
		if container.usable && len(container.requests) < 1 {
			res = container
			container.Unlock()
			break
		}
		container.Unlock()
	}
	return res
}

// if no idle container exists
func (r *Router) CreateNewCntFromNode(req *pb.AcquireContainerRequest, priority float64) (*ExtendedContainerInfo, error) {
	node, err := r.getNode(req.AccountId, req.FunctionConfig.MemoryInBytes)
	if err != nil {
		return nil, err
	}
	fn := req.FunctionName
	node.Lock()
	if cnt, ok := node.fn2Cnt.Get(fn); ok {
		node.fn2Cnt.Set(fn, cnt.(int)+1)
	} else {
		logger.Warningf("node %v still doesn't has ctn for %s", node, fn)
		node.fn2Cnt.Set(fn, 1)
	}
	node.Unlock()

	return r.createNewCntOnNode(req, priority, node)
}

func (r *Router) CreateNewCntFromOnDemandNode(req *pb.AcquireContainerRequest, priority float64) (*ExtendedContainerInfo, error) {
	cpuThreshold := float64(req.FunctionConfig.MemoryInBytes) / gibyte * 67
	node, err := r.getNodeWithMemAndCpuCheck(req.AccountId, req.FunctionConfig.MemoryInBytes, cpuThreshold)
	if err != nil {
		return nil, err
	}
	fn := req.FunctionName
	node.Lock()
	if cnt, ok := node.fn2Cnt.Get(fn); ok {
		node.fn2Cnt.Set(fn, cnt.(int)+1)
	} else {
		logger.Warningf("node %v still doesn't has ctn for %s", node, fn)
		node.fn2Cnt.Set(fn, 1)
	}
	node.Unlock()

	return r.createNewCntOnNode(req, priority, node)
}

func (r *Router) createNewCntOnNode(req *pb.AcquireContainerRequest, priority float64, node *ExtendedNodeInfo) (*ExtendedContainerInfo, error) {
	now := time.Now().UnixNano()
	fn := req.FunctionName
	var res *ExtendedContainerInfo

	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	replyC, err := node.CreateContainer(ctx, &nsPb.CreateContainerRequest{
		Name: fn + uuid.NewV4().String(),
		FunctionMeta: &nsPb.FunctionMeta{
			FunctionName:  fn,
			Handler:       req.FunctionConfig.Handler,
			TimeoutInMs:   funTimeout,
			MemoryInBytes: req.FunctionConfig.MemoryInBytes,
		},
		RequestId: req.RequestId,
	})
	rpcDelay := (time.Now().UnixNano() - now) / 1e6
	if err != nil {
		r.handleContainerErr(node, req.FunctionConfig.MemoryInBytes)
		logger.Errorf("failed to create container on %s", node.address, err)
		return nil, errors.Wrapf(err, "failed to create container on %s", node.address)
	}

	res = &ExtendedContainerInfo{
		id:               replyC.ContainerId,
		address:          node.address,
		port:             node.port,
		nodeId:           node.nodeID,
		requests:         make(map[string]int64),
		fn:               fn,
		ReqMemoryInBytes: float64(req.FunctionConfig.MemoryInBytes),
		usable:           true,
		priority:         priority,
	}
	//res.requests[req.RequestId] = 1 // The container hasn't been listed in the ctn_ids. So we don't need locking here.

	ctns, _ := r.fn2ctnSlice.Get(fn)
	ctn_ids := ctns.(*RwLockSlice)
	ctn_ids.Lock()
	ctn_ids.ctns = append(ctn_ids.ctns, res)
	ctn_ids.Unlock()
	r.ctn2info.Set(res.id, res)
	r.cnt2node.Set(res.id, node)
	logger.Infof("createNewCntOnNode for %s to %s,rpc lat %d, lat %d ",
		fn, node.address, rpcDelay, (time.Now().UnixNano()-now)/1e6)
	return res, nil
}

// acuire ctn in every node:only expand one ctn in one node
func (r *Router) createNewCntInEveryNode(req *pb.AcquireContainerRequest, priority float64) {
	fn := req.FunctionName
	for _, node := range values {
		if ok := node.fn2Cnt.SetIfAbsent(fn, 1); ok {
			go r.createNewCntOnNode(req, priority, node)
		}
	}
}
func sortCtnByMemUsage(ctns []*ExtendedContainerInfo) {
	sort.Slice(ctns, func(i, j int) bool {
		if ctns[i].priority*float64(len(ctns[i].requests)) < ctns[j].priority*float64(len(ctns[j].requests)) {
			return true
		}
		if (ctns[i].MemoryUsageInBytes < ctns[j].MemoryUsageInBytes) {
			return true
		}
		return ctns[i].CpuUsagePct < ctns[j].CpuUsagePct
	})
}
