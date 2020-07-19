package core

import (
	"sync"

	"google.golang.org/grpc"

	pb "aliyun/serverless/mini-faas/nodeservice/proto"
)

type ExtendsNodeInfo struct {
	sync.Mutex
	nodeID              string
	address             string
	port                int64
	availableMemInBytes int64

	conn *grpc.ClientConn
	pb.NodeServiceClient
	// todo add stats which is used to best fit node
}

func (node *ExtendsNodeInfo) UpdateStats(nodeID, address string, port, memory int64) {
	//node.GetStats()
}
