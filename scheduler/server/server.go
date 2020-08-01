package server

import (
	"aliyun/serverless/mini-faas/scheduler/utils/logger"
	"sync"
	"time"

	"golang.org/x/net/context"
	"google.golang.org/grpc"
	"google.golang.org/grpc/codes"

	"aliyun/serverless/mini-faas/scheduler/core"
	"aliyun/serverless/mini-faas/scheduler/model"
	pb "aliyun/serverless/mini-faas/scheduler/proto"
)

type Server struct {
	sync.WaitGroup
	router *core.Router
}

func NewServer(router *core.Router) *Server {
	return &Server{
		router: router,
	}
}

func (s *Server) Start() {
	// Just in case the router has internal loops.
	s.router.Start()
}

func (s *Server) AcquireContainer(ctx context.Context, req *pb.AcquireContainerRequest) (*pb.AcquireContainerReply, error) {
	if req.AccountId == "" {
		return nil, grpc.Errorf(codes.InvalidArgument, "account ID cannot be empty")
	}
	if req.FunctionConfig == nil {
		return nil, grpc.Errorf(codes.InvalidArgument, "function config cannot be nil")
	}

	//logger.WithFields(logger.Fields{
	//	"Operation": "AcquireContainer",
	//	"FunctionName": req.FunctionName,
	//	"RequestId": req.RequestId,
	//	"MemoryInBytes": req.FunctionConfig.MemoryInBytes,
	//}).Infof("")
	now := time.Now().UnixNano()
	reply, err := s.router.AcquireContainer(ctx, req)
	if err != nil {
		logger.WithFields(logger.Fields{
			"Operation": "AcquireContainer",
			"Latency":   (time.Now().UnixNano() - now) / 1e6,
			"Error":     true,
		}).Errorf("Failed to acquire due to %v", err)
		return nil, err
	}
	return reply, nil
}

func (s *Server) ReturnContainer(ctx context.Context, req *pb.ReturnContainerRequest) (*pb.ReturnContainerReply, error) {
	go s.router.ReturnContainer(&model.ResponseInfo{
		ID:                    req.RequestId,
		ContainerId:           req.ContainerId,
		DurationInMs:          req.DurationInNanos / 1e6,
		MaxMemoryUsageInBytes: req.MaxMemoryUsageInBytes,
		ErrorCode:             req.ErrorCode,
		ErrorMessage:          req.ErrorMessage,
	})

	return &pb.ReturnContainerReply{}, nil
}
