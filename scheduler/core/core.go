package core

import (
	"aliyun/serverless/mini-faas/scheduler/model"
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
	"context"

	nsPb "aliyun/serverless/mini-faas/nodeservice/proto"
	pb "aliyun/serverless/mini-faas/scheduler/proto"
	"github.com/pkg/errors"
	"github.com/satori/go.uuid"
	"time"
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

func getFuncExeMode(req *pb.AcquireContainerRequest) FuncExeMode {
	// todo use stats
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
	r.reduceReqMem(req)
	return r.pickCntBasic(req);
}

func (r *Router) pickCnt4CpuIntensive(req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	r.reduceReqMem(req)
	return r.pickCntBasic(req);
}

func (r *Router) reduceReqMem(req *pb.AcquireContainerRequest) {
	fn := req.FunctionName
	finfoObj, ok := r.fn2finfoMap.Get(fn)
	if ok {
		finfo := finfoObj.(*model.FuncInfo)
		actualUsedMem := finfo.ActualUsedMemInBytes
		if (actualUsedMem > 0 && actualUsedMem < finfo.MemoryInBytes) {
			req.FunctionConfig.MemoryInBytes = actualUsedMem
			logger.Infof("change req mem from %s to %s for fn %s",
				finfo.MemoryInBytes, actualUsedMem, fn)
		}
	}
}

func (r *Router) pickCnt4MemIntensive(req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	return r.pickCntBasic(req);
}

func (r *Router) pickCntBasic(req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	var res *ExtendedContainerInfo

	fn := req.FunctionName
	ctns, _ := r.functionMap.Get(fn)

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
		// todo add algo trigger to async add container and async add node
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
		node, err := r.getNode(req.AccountId, req.FunctionConfig.MemoryInBytes)
		if err != nil {
			return nil, err
		}

		ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
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
		res = &ExtendedContainerInfo{
			id:       replyC.ContainerId,
			address:  node.address,
			port:     node.port,
			nodeId:   node.nodeID,
			requests: make(map[string]int64),
			fn:       fn,
		}
		res.requests[req.RequestId] = 1 // The container hasn't been listed in the ctn_ids. So we don't need locking here.
		ctn_ids.Lock()
		ctn_ids.ctns = append(ctn_ids.ctns, res.id)
		ctn_ids.Unlock()
		r.ctn2info.Set(res.id, res)
		r.cnt2node.Set(res.id, node)
	}
	return &pb.AcquireContainerReply{
		NodeId:          res.nodeId,
		NodeAddress:     res.address,
		NodeServicePort: res.port,
		ContainerId:     res.id,
	}, nil
}
