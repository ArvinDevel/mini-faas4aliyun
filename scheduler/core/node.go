package core

import (
	"fmt"
	"sync"

	"github.com/pkg/errors"
	"google.golang.org/grpc"

	pb "aliyun/serverless/mini-faas/nodeservice/proto"
)

type ExtendedNodeInfo struct {
	sync.Mutex
	nodeID              string
	address             string
	port                int64
	availableMemInBytes int64

	conn *grpc.ClientConn
	pb.NodeServiceClient

	// one moment static view: todo use a list of stats
	TotalMemoryInBytes     float64 `protobuf:"varint,1,opt,name=total_memory_in_bytes,json=totalMemoryInBytes,proto3" json:"total_memory_in_bytes,omitempty"`
	MemoryUsageInBytes     float64 `protobuf:"varint,2,opt,name=memory_usage_in_bytes,json=memoryUsageInBytes,proto3" json:"memory_usage_in_bytes,omitempty"`
	AvailableMemoryInBytes float64 `protobuf:"varint,3,opt,name=available_memory_in_bytes,json=availableMemoryInBytes,proto3" json:"available_memory_in_bytes,omitempty"`
	CpuUsagePct            float64
	ctnCnt                 int
	// used to forbidden access to failed node
	failedCnt int
}

func NewNode(nodeID, address string, port, memory int64) (*ExtendedNodeInfo, error) {
	conn, err := grpc.Dial(fmt.Sprintf("%s:%d", address, port), grpc.WithInsecure())
	if err != nil {
		return nil, errors.WithStack(err)
	}
	return &ExtendedNodeInfo{
		Mutex:               sync.Mutex{},
		nodeID:              nodeID,
		address:             address,
		port:                port,
		availableMemInBytes: memory,
		conn:                conn,
		NodeServiceClient:   pb.NewNodeServiceClient(conn),
	}, nil
}

// Close closes the connection
func (n *ExtendedNodeInfo) Close() {
	n.conn.Close()
}

func (node *ExtendedNodeInfo) String() string {
	return fmt.Sprintf("Node [%s,%s ],mem:%f/%f,%f, %f, cpu:%f ,failed: %d ",
		node.address, node.nodeID,
		node.MemoryUsageInBytes, node.TotalMemoryInBytes, node.availableMemInBytes, node.AvailableMemoryInBytes, node.CpuUsagePct,
		node.failedCnt)
}
