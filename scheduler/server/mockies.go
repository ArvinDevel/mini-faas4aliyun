package server

import (
	nodeproto "aliyun/serverless/mini-faas/nodeservice/proto"
	rmproto "aliyun/serverless/mini-faas/resourcemanager/proto"
)

type Dep struct {
}

var nidCnt = 0
//rm
func (*Dep) ReserveNode(req *rmproto.ReserveNodeRequest) (*rmproto.ReserveNodeReply, error) {
	node := rmproto.NodeDesc{}
	node.Address = "http://"
	node.Id = "nid" + string(nidCnt)
	nidCnt++
	node.MemoryInBytes = 1234
	node.ReservedTimeTimestampMs = 0
	node.ReleasedTimeTimestampMs = 100
	reply := rmproto.ReserveNodeReply{}
	reply.Node = &node
	return &reply, nil
}

func (*Dep) ReleaseNode(req *rmproto.ReleaseNodeRequest) (*rmproto.ReleaseNodeReply, error) {
	return &rmproto.ReleaseNodeReply{}, nil
}

var cidCnt = 0
//node
func (*Dep) CreateContainer(req *nodeproto.CreateContainerRequest) (*nodeproto.CreateContainerReply, error) {
	ctn := nodeproto.CreateContainerReply{}
	ctn.ContainerId = "cid" + string(cidCnt)
	cidCnt++
	return &ctn, nil
}

func (*Dep) RemoveContainer(req *nodeproto.RemoveContainerRequest) (*nodeproto.RemoveContainerReply, error) {
	return &nodeproto.RemoveContainerReply{}, nil
}

func (*Dep) InvokeFunction(req *nodeproto.InvokeFunctionRequest) (*nodeproto.InvokeFunctionReply, error) {
	return &nodeproto.InvokeFunctionReply{}, nil
}

func (*Dep) GetStats(req *nodeproto.GetStatsRequest) (*nodeproto.GetStatsReply, error) {
	return &nodeproto.GetStatsReply{}, nil
}
