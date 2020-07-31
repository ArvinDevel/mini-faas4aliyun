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
	ctns, _ := r.fn2ctnSlice.Get(fn)

	ctn_ids := ctns.(*RwLockSlice)

	ctn_ids.RLock()
	for _, val := range ctn_ids.ctns {
		cmObj, ok := r.ctn2info.Get(val)
		if (!ok) {
			logger.Errorf("No ctn info found 4 ctn %s", val)
			continue
		}
		container := cmObj.(*ExtendedContainerInfo)
		container.Lock()
		if len(container.requests) < 1 {
			container.requests[req.RequestId] = 1
			res = container
			container.Unlock()
			break
		}
		container.Unlock()
	}
	ctn_ids.RUnlock()

	if res == nil { // if no idle container exists
		ctn, err := r.createNewCntFromNode(req)
		if err != nil {
			return nil, err
		}
		res = ctn
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

	ctn_ids.RLock()
	ctns := []*ExtendedContainerInfo{}
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
		ctn.Lock()
		if len(ctn.requests) < parallelReqNum {
			ctn.requests[req.RequestId] = 1
			res = ctn
			ctn.Unlock()
			break
		}
		ctn.Unlock()
	}
	if res == nil { // if no idle container exists
		ctn, err := r.createNewCntFromNode(req)
		if err != nil {
			return nil, err
		}
		res = ctn
	}
	return &pb.AcquireContainerReply{
		NodeId:          res.nodeId,
		NodeAddress:     res.address,
		NodeServicePort: res.port,
		ContainerId:     res.id,
	}, nil
}

// if no idle container exists
func (r *Router) createNewCntFromNode(req *pb.AcquireContainerRequest) (*ExtendedContainerInfo, error) {
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
			TimeoutInMs:   req.FunctionConfig.TimeoutInMs,
			MemoryInBytes: req.FunctionConfig.MemoryInBytes,
		},
		RequestId: req.RequestId,
	})
	if err != nil {
		r.handleContainerErr(node, req.FunctionConfig.MemoryInBytes)
		return nil, errors.Wrapf(err, "failed to create container on %s", node.address)
	}
	// reset node info
	lastFailedCnt := node.failedCnt
	if lastFailedCnt > 0 {
		node.Lock()
		node.failedCnt = 0
		node.reqCnt -= 1
		node.Unlock()
		logger.Infof("Reset node %s fail cnt from %d to 0", node.address, lastFailedCnt)
	}
	res = &ExtendedContainerInfo{
		id:       replyC.ContainerId,
		address:  node.address,
		port:     node.port,
		nodeId:   node.nodeID,
		requests: make(map[string]int64),
		fn:       req.FunctionName,
	}
	res.requests[req.RequestId] = 1 // The container hasn't been listed in the ctn_ids. So we don't need locking here.

	ctns, _ := r.fn2ctnSlice.Get(req.FunctionName)
	ctn_ids := ctns.(*RwLockSlice)
	ctn_ids.Lock()
	ctn_ids.ctns = append(ctn_ids.ctns, res.id)
	ctn_ids.Unlock()
	r.ctn2info.Set(res.id, res)
	r.cnt2node.Set(res.id, node)
	logger.Infof("createNewCntFromNode for %s to %s,lat %d ",
		req.FunctionName, node.address, (time.Now().UnixNano()-now)/1e6)
	return res, nil
}

func sortCtnByMemUsage(values []*ExtendedContainerInfo) {
	sort.Slice(values, func(i, j int) bool {
		if (values[i].MemoryUsageInBytes < values[j].MemoryUsageInBytes) {
			return true
		}
		return values[i].CpuUsagePct < values[j].CpuUsagePct
	})
}
