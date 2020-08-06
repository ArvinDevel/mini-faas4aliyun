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

// todo default use serial req mode, and fallback(to mem intensive wait ctn ready)
func (r *Router) getFuncExeMode(req *pb.AcquireContainerRequest) FuncExeMode {
	// todo use basic ratio FIRST PRIORITY ！use stats
	fn := req.FunctionName
	finfoObj, ok := r.fn2finfoMap.Get(fn)
	if ok {
		finfo := finfoObj.(*model.FuncInfo)
		memUsage := float64(finfo.MaxMemoryUsageInBytes) / float64(finfo.MemoryInBytes)
		if memUsage > ctnMemHighThreshold {
			return MemIntensive
		}
	}
	return ResourceLess
}

func (r *Router) pickCntAccording2ExeMode(exeMode FuncExeMode, req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	switch exeMode {
	case CpuIntensive:
		return r.pickCnt4CpuIntensive(req);
	case MemIntensive:
		return r.pickCnt4MemIntensive(req);
	case ResourceLess:
		return r.pickCnt4ResourceLess(req);
	}
	logger.Errorf("Unrecognized exeMode %s", exeMode)
	return nil, errors.Errorf("Unrecognized exeMode %s", exeMode)
}

func (r *Router) pickCnt4ResourceLess(req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	return r.pickCnt4ParallelReq(req);
}

func (r *Router) pickCnt4CpuIntensive(req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	// cpu intensive: reduce mem usage, guarantee cpu by use serial strategy
	r.reduceReqMem(req)
	return r.pickCnt4SerialReq(req);
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

func (r *Router) pickCnt4MemIntensive(req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	return r.pickCnt4SerialReq(req);
}

// 只适合串行执行的：资源竞争激烈的，cpu/mem占用率极高
func (r *Router) pickCnt4SerialReq(req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	var res *ExtendedContainerInfo

	fn := req.FunctionName
	// if not ok, panic
	rwSlice, _ := r.fn2ctnSlice.Get(fn)

	ctn_ids := rwSlice.(*RwLockSlice)

	ctns := []*ExtendedContainerInfo{}
	ctn_ids.RLock()
	for _, val := range ctn_ids.ctns {
		cmObj, ok := r.ctn2info.Get(val)
		if (!ok) {
			logger.Errorf("No ctn info found 4 ctn %s", val)
			continue
		}
		container := cmObj.(*ExtendedContainerInfo)
		container.Lock()
		if container.usable && len(container.requests) < 1 {
			container.requests[req.RequestId] = 1
			res = container
			container.Unlock()
			break
		}
		container.Unlock()
		ctns = append(ctns, container)
	}
	ctn_ids.RUnlock()

	if res == nil { // if no idle container exists
		logger.Infof("%d ctns  can't provide for %s", len(ctns), fn)
		ctn, err := r.CreateNewCntFromNode(req, 1.0)
		if err != nil {
			if len(ctns) > 0 {
				res = fallbackChooseCtn(ctns)
			} else {
				return nil, err
			}
		} else {
			res = ctn
		}
	}

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
	// if not ok, panic
	rwLockSlice, _ := r.fn2ctnSlice.Get(fn)

	ctn_ids := rwLockSlice.(*RwLockSlice)

	ctns := []*ExtendedContainerInfo{}
	ctn_ids.RLock()
	for _, val := range ctn_ids.ctns {
		cmObj, ok := r.ctn2info.Get(val)
		if (!ok) {
			logger.Errorf("No ctn info found 4 ctn %s", val)
			continue
		}
		container := cmObj.(*ExtendedContainerInfo)
		ctns = append(ctns, container)
	}
	ctn_ids.RUnlock()
	sortCtnByMemUsage(ctns)
	for _, ctn := range ctns {
		//if ctn.isCpuOrMemUsageHigh() {
		//	continue
		//}
		ctn.Lock()
		if ctn.usable && len(ctn.requests) < parallelReqNum {
			ctn.requests[req.RequestId] = 1
			res = ctn
			ctn.Unlock()
			break
		}
		ctn.Unlock()
	}
	if res == nil { // if no idle container exists
		logger.Infof("%d ctns  can't provide for %s", len(ctns), fn)
		ctn, err := r.CreateNewCntFromNode(req, 1.0)
		if err != nil {
			if len(ctns) > 0 {
				res = fallbackChooseCtn(ctns)
			} else {
				return nil, err
			}
		} else {
			res = ctn
		}
	}
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

// if no idle container exists
func (r *Router) CreateNewCntFromNode(req *pb.AcquireContainerRequest, priority float64) (*ExtendedContainerInfo, error) {
	now := time.Now().UnixNano()
	var res *ExtendedContainerInfo

	node, err := r.getNode(req.AccountId, req.FunctionConfig.MemoryInBytes)
	if err != nil {
		return nil, err
	}

	node.Lock()
	node.reqCnt += 1
	node.Unlock()
	ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	replyC, err := node.CreateContainer(ctx, &nsPb.CreateContainerRequest{
		Name: req.FunctionName + uuid.NewV4().String(),
		FunctionMeta: &nsPb.FunctionMeta{
			FunctionName:  req.FunctionName,
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
	// reset node info

	node.Lock()
	node.reqCnt -= 1
	node.Unlock()

	res = &ExtendedContainerInfo{
		id:               replyC.ContainerId,
		address:          node.address,
		port:             node.port,
		nodeId:           node.nodeID,
		requests:         make(map[string]int64),
		fn:               req.FunctionName,
		ReqMemoryInBytes: float64(req.FunctionConfig.MemoryInBytes),
		usable:           true,
		priority:         priority,
	}
	res.requests[req.RequestId] = 1 // The container hasn't been listed in the ctn_ids. So we don't need locking here.

	ctns, _ := r.fn2ctnSlice.Get(req.FunctionName)
	ctn_ids := ctns.(*RwLockSlice)
	ctn_ids.Lock()
	ctn_ids.ctns = append(ctn_ids.ctns, res.id)
	ctn_ids.Unlock()
	r.ctn2info.Set(res.id, res)
	r.cnt2node.Set(res.id, node)
	logger.Infof("CreateNewCntFromNode for %s to %s,rpc lat %d, lat %d ",
		req.FunctionName, node.address, rpcDelay, (time.Now().UnixNano()-now)/1e6)
	return res, nil
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
