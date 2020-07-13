package server

import (
	"aliyun/serverless/mini-faas/scheduler/proto"
	"context"
	"fmt"
	"google.golang.org/grpc/codes"
	"google.golang.org/grpc/status"
	nodeproto "mini-faas/nodeservice/proto"
	"mini-faas/scheduler/proto"
)

type MockServer struct {
}

var dep = Dep{}

func StartMock() {
	fmt.Printf("begin start mock\n")
	var server = MockServer{}
	req := schedulerproto.AcquireContainerRequest{}
	req.FunctionConfig = &schedulerproto.FunctionConfig{
		MemoryInBytes: 100,
	}
	cids := []string{}
	for i := 0; i < 100; i++ {
		req.FunctionName = "mockfn" + string(i)
		req.RequestId = string(i)
		reply, _ := server.AcquireContainer(nil, &req)
		cids = append(cids, reply.ContainerId)
	}
	ret := schedulerproto.ReturnContainerRequest{}

	for i := 0; i < 10; i++ {
		req.FunctionName = "mockfn" + string(i)
		req.RequestId = string(i)
		reply, _ := server.AcquireContainer(nil, &req)
		cids = append(cids, reply.ContainerId)
	}

	for i := 0; i < 110; i++ {
		ret.ContainerId = cids[i]
		server.ReturnContainer(nil, &ret)
	}
	fmt.Printf("end return cnts \n")
}

func (*MockServer) AcquireContainer(ctx context.Context, req *schedulerproto.AcquireContainerRequest) (*schedulerproto.AcquireContainerReply, error) {
	fmt.Printf("AcquireContainer %s \n", req)
	fn := req.FunctionName
	// todo update funconf.MemoryInMb type
	memReq := int64(req.FunctionConfig.MemoryInBytes)
	reply := schedulerproto.AcquireContainerReply{}

	if cnt := GetContainer(fn); cnt != nil {
		reply.ContainerId = cnt.(string)
		return &reply, nil
	}

	if rc := remoteGetCnt(memReq); rc != nil {
		PutContainer(fn, rc.ContainerId)
		reply.ContainerId = rc.ContainerId
		return &reply, nil
	}

	fmt.Printf("Errorf return nill \n")
	return nil, status.Errorf(codes.NotFound, "failed")
}
func (*MockServer) ReturnContainer(ctx context.Context, req *schedulerproto.ReturnContainerRequest) (*schedulerproto.ReturnContainerReply, error) {
	MarkDeletedCnt(req)
	return nil, nil
}

func remoteGetCnt(memReq int64) *nodeproto.CreateContainerReply {
	node := PickNode(memReq)
	req := nodeproto.CreateContainerRequest{}
	req.Name = node.Id
	reply, err := dep.CreateContainer(&req)
	if err == nil {
		PutContainer2node(reply.ContainerId, node)
		return reply
	}
	return nil
}
