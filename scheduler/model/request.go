package model

type RequestInfo struct {
	ID               string
	FunctionName     string
	RequestTimeoutMs int64
	AccountID        string
}

type ResponseInfo struct {
	ID                    string
	ContainerId           string
	DurationInNanos       int64 `protobuf:"varint,3,opt,name=duration_in_nanos,json=durationInNanos,proto3" json:"duration_in_nanos,omitempty"`
	MaxMemoryUsageInBytes int64 `protobuf:"varint,4,opt,name=max_memory_usage_in_bytes,json=maxMemoryUsageInBytes,proto3" json:"max_memory_usage_in_bytes,omitempty"`
	ErrorCode             string `protobuf:"bytes,5,opt,name=error_code,json=errorCode,proto3" json:"error_code,omitempty"`
	ErrorMessage          string
}

type FuncInfo struct {
	TimeoutInMs           int64
	MemoryInBytes         int64
	DurationInNanos       int64 `protobuf:"varint,3,opt,name=duration_in_nanos,json=durationInNanos,proto3" json:"duration_in_nanos,omitempty"`
	MaxMemoryUsageInBytes int64 `protobuf:"varint,4,opt,name=max_memory_usage_in_bytes,json=maxMemoryUsageInBytes,proto3" json:"max_memory_usage_in_bytes,omitempty"`
	ActualUsedMemInBytes  int64
}
