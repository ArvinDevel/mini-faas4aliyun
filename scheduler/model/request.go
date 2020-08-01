package model

import "fmt"

type FuncCallMode int

//pre do action(swarm up or release quickly) according to stats or predict
const (
	Unkown     FuncCallMode = iota
	Periodical  // 周期型
	Sparse      // 稀疏型
	Dense       // 密集型
)

type RequestInfo struct {
	ID               string
	FunctionName     string
	RequestTimeoutMs int64
	AccountID        string
}

type ResponseInfo struct {
	ID                    string
	ContainerId           string
	DurationInMs          int64  `protobuf:"varint,3,opt,name=duration_in_nanos,json=durationInNanos,proto3" json:"duration_in_nanos,omitempty"`
	MaxMemoryUsageInBytes int64  `protobuf:"varint,4,opt,name=max_memory_usage_in_bytes,json=maxMemoryUsageInBytes,proto3" json:"max_memory_usage_in_bytes,omitempty"`
	ErrorCode             string `protobuf:"bytes,5,opt,name=error_code,json=errorCode,proto3" json:"error_code,omitempty"`
	ErrorMessage          string
}

type FuncInfo struct {
	// static info(req info)
	TimeoutInMs   int64
	MemoryInBytes int64

	// dynamic info(runtime stats)
	MinDurationInMs       int64
	MaxDurationInMs       int64 `protobuf:"varint,3,opt,name=duration_in_nanos,json=durationInNanos,proto3" json:"duration_in_nanos,omitempty"`
	SumDurationInMs       int64
	Cnt                   int64
	MaxMemoryUsageInBytes int64 `protobuf:"varint,4,opt,name=max_memory_usage_in_bytes,json=maxMemoryUsageInBytes,proto3" json:"max_memory_usage_in_bytes,omitempty"`

	ActualUsedMemInBytes int64

	CallMode          FuncCallMode
	DenseCnt          int
	Handler           string
	TimeOverThreshold bool
}

func (finfo *FuncInfo) String() string {
	return fmt.Sprintf("Fn cnt: %d, duration:%d,%d, mem:%d/%d",
		finfo.Cnt,
		finfo.MinDurationInMs, finfo.MaxDurationInMs,
		finfo.MaxMemoryUsageInBytes, finfo.MemoryInBytes)
}
