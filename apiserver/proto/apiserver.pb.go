// Code generated by protoc-gen-gogo.
// source: apiserver.proto
// DO NOT EDIT!

/*
Package apiserverproto is a generated protocol buffer package.

It is generated from these files:
	apiserver.proto

It has these top-level messages:
	InvokeFunctionRequest
	InvokeFunctionReply
	ListFunctionsRequest
	ListFunctionsReply
	FunctionConfig
*/
package apiserverproto

import proto "github.com/gogo/protobuf/proto"
import fmt "fmt"
import math "math"

import (
	context "golang.org/x/net/context"
	grpc "google.golang.org/grpc"
)

// Reference imports to suppress errors if they are not otherwise used.
var _ = proto.Marshal
var _ = fmt.Errorf
var _ = math.Inf

// This is a compile-time assertion to ensure that this generated file
// is compatible with the proto package it is being compiled against.
// A compilation error at this line likely means your copy of the
// proto package needs to be updated.
const _ = proto.GoGoProtoPackageIsVersion2 // please upgrade the proto package

type InvokeFunctionRequest struct {
	AccountId    string `protobuf:"bytes,1,opt,name=account_id,json=accountId,proto3" json:"account_id,omitempty"`
	FunctionName string `protobuf:"bytes,2,opt,name=function_name,json=functionName,proto3" json:"function_name,omitempty"`
	Event        []byte `protobuf:"bytes,3,opt,name=event,proto3" json:"event,omitempty"`
}

func (m *InvokeFunctionRequest) Reset()                    { *m = InvokeFunctionRequest{} }
func (m *InvokeFunctionRequest) String() string            { return proto.CompactTextString(m) }
func (*InvokeFunctionRequest) ProtoMessage()               {}
func (*InvokeFunctionRequest) Descriptor() ([]byte, []int) { return fileDescriptorApiserver, []int{0} }

func (m *InvokeFunctionRequest) GetAccountId() string {
	if m != nil {
		return m.AccountId
	}
	return ""
}

func (m *InvokeFunctionRequest) GetFunctionName() string {
	if m != nil {
		return m.FunctionName
	}
	return ""
}

func (m *InvokeFunctionRequest) GetEvent() []byte {
	if m != nil {
		return m.Event
	}
	return nil
}

type InvokeFunctionReply struct {
	RequestId             string `protobuf:"bytes,1,opt,name=request_id,json=requestId,proto3" json:"request_id,omitempty"`
	Body                  []byte `protobuf:"bytes,2,opt,name=body,proto3" json:"body,omitempty"`
	ServerSideLatencyInMs int64  `protobuf:"varint,3,opt,name=server_side_latency_in_ms,json=serverSideLatencyInMs,proto3" json:"server_side_latency_in_ms,omitempty"`
}

func (m *InvokeFunctionReply) Reset()                    { *m = InvokeFunctionReply{} }
func (m *InvokeFunctionReply) String() string            { return proto.CompactTextString(m) }
func (*InvokeFunctionReply) ProtoMessage()               {}
func (*InvokeFunctionReply) Descriptor() ([]byte, []int) { return fileDescriptorApiserver, []int{1} }

func (m *InvokeFunctionReply) GetRequestId() string {
	if m != nil {
		return m.RequestId
	}
	return ""
}

func (m *InvokeFunctionReply) GetBody() []byte {
	if m != nil {
		return m.Body
	}
	return nil
}

func (m *InvokeFunctionReply) GetServerSideLatencyInMs() int64 {
	if m != nil {
		return m.ServerSideLatencyInMs
	}
	return 0
}

type ListFunctionsRequest struct {
}

func (m *ListFunctionsRequest) Reset()                    { *m = ListFunctionsRequest{} }
func (m *ListFunctionsRequest) String() string            { return proto.CompactTextString(m) }
func (*ListFunctionsRequest) ProtoMessage()               {}
func (*ListFunctionsRequest) Descriptor() ([]byte, []int) { return fileDescriptorApiserver, []int{2} }

type ListFunctionsReply struct {
	RequestId string            `protobuf:"bytes,1,opt,name=request_id,json=requestId,proto3" json:"request_id,omitempty"`
	Functions []*FunctionConfig `protobuf:"bytes,2,rep,name=functions" json:"functions,omitempty"`
}

func (m *ListFunctionsReply) Reset()                    { *m = ListFunctionsReply{} }
func (m *ListFunctionsReply) String() string            { return proto.CompactTextString(m) }
func (*ListFunctionsReply) ProtoMessage()               {}
func (*ListFunctionsReply) Descriptor() ([]byte, []int) { return fileDescriptorApiserver, []int{3} }

func (m *ListFunctionsReply) GetRequestId() string {
	if m != nil {
		return m.RequestId
	}
	return ""
}

func (m *ListFunctionsReply) GetFunctions() []*FunctionConfig {
	if m != nil {
		return m.Functions
	}
	return nil
}

type FunctionConfig struct {
	FunctionName  string `protobuf:"bytes,1,opt,name=function_name,json=functionName,proto3" json:"function_name,omitempty"`
	TimeoutInMs   int64  `protobuf:"varint,2,opt,name=timeout_in_ms,json=timeoutInMs,proto3" json:"timeout_in_ms,omitempty"`
	MemoryInBytes int64  `protobuf:"varint,3,opt,name=memory_in_bytes,json=memoryInBytes,proto3" json:"memory_in_bytes,omitempty"`
	Handler       string `protobuf:"bytes,4,opt,name=handler,proto3" json:"handler,omitempty"`
}

func (m *FunctionConfig) Reset()                    { *m = FunctionConfig{} }
func (m *FunctionConfig) String() string            { return proto.CompactTextString(m) }
func (*FunctionConfig) ProtoMessage()               {}
func (*FunctionConfig) Descriptor() ([]byte, []int) { return fileDescriptorApiserver, []int{4} }

func (m *FunctionConfig) GetFunctionName() string {
	if m != nil {
		return m.FunctionName
	}
	return ""
}

func (m *FunctionConfig) GetTimeoutInMs() int64 {
	if m != nil {
		return m.TimeoutInMs
	}
	return 0
}

func (m *FunctionConfig) GetMemoryInBytes() int64 {
	if m != nil {
		return m.MemoryInBytes
	}
	return 0
}

func (m *FunctionConfig) GetHandler() string {
	if m != nil {
		return m.Handler
	}
	return ""
}

func init() {
	proto.RegisterType((*InvokeFunctionRequest)(nil), "apiserverproto.InvokeFunctionRequest")
	proto.RegisterType((*InvokeFunctionReply)(nil), "apiserverproto.InvokeFunctionReply")
	proto.RegisterType((*ListFunctionsRequest)(nil), "apiserverproto.ListFunctionsRequest")
	proto.RegisterType((*ListFunctionsReply)(nil), "apiserverproto.ListFunctionsReply")
	proto.RegisterType((*FunctionConfig)(nil), "apiserverproto.FunctionConfig")
}

// Reference imports to suppress errors if they are not otherwise used.
var _ context.Context
var _ grpc.ClientConn

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
const _ = grpc.SupportPackageIsVersion4

// Client API for APIServer service

type APIServerClient interface {
	InvokeFunction(ctx context.Context, in *InvokeFunctionRequest, opts ...grpc.CallOption) (*InvokeFunctionReply, error)
	ListFunctions(ctx context.Context, in *ListFunctionsRequest, opts ...grpc.CallOption) (*ListFunctionsReply, error)
}

type aPIServerClient struct {
	cc *grpc.ClientConn
}

func NewAPIServerClient(cc *grpc.ClientConn) APIServerClient {
	return &aPIServerClient{cc}
}

func (c *aPIServerClient) InvokeFunction(ctx context.Context, in *InvokeFunctionRequest, opts ...grpc.CallOption) (*InvokeFunctionReply, error) {
	out := new(InvokeFunctionReply)
	err := grpc.Invoke(ctx, "/apiserverproto.APIServer/InvokeFunction", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *aPIServerClient) ListFunctions(ctx context.Context, in *ListFunctionsRequest, opts ...grpc.CallOption) (*ListFunctionsReply, error) {
	out := new(ListFunctionsReply)
	err := grpc.Invoke(ctx, "/apiserverproto.APIServer/ListFunctions", in, out, c.cc, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// Server API for APIServer service

type APIServerServer interface {
	InvokeFunction(context.Context, *InvokeFunctionRequest) (*InvokeFunctionReply, error)
	ListFunctions(context.Context, *ListFunctionsRequest) (*ListFunctionsReply, error)
}

func RegisterAPIServerServer(s *grpc.Server, srv APIServerServer) {
	s.RegisterService(&_APIServer_serviceDesc, srv)
}

func _APIServer_InvokeFunction_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(InvokeFunctionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(APIServerServer).InvokeFunction(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/apiserverproto.APIServer/InvokeFunction",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(APIServerServer).InvokeFunction(ctx, req.(*InvokeFunctionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _APIServer_ListFunctions_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(ListFunctionsRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(APIServerServer).ListFunctions(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/apiserverproto.APIServer/ListFunctions",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(APIServerServer).ListFunctions(ctx, req.(*ListFunctionsRequest))
	}
	return interceptor(ctx, in, info, handler)
}

var _APIServer_serviceDesc = grpc.ServiceDesc{
	ServiceName: "apiserverproto.APIServer",
	HandlerType: (*APIServerServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "InvokeFunction",
			Handler:    _APIServer_InvokeFunction_Handler,
		},
		{
			MethodName: "ListFunctions",
			Handler:    _APIServer_ListFunctions_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "apiserver.proto",
}

func init() { proto.RegisterFile("apiserver.proto", fileDescriptorApiserver) }

var fileDescriptorApiserver = []byte{
	// 388 bytes of a gzipped FileDescriptorProto
	0x1f, 0x8b, 0x08, 0x00, 0x00, 0x00, 0x00, 0x00, 0x02, 0xff, 0x8c, 0x92, 0xc1, 0x6a, 0xdb, 0x40,
	0x10, 0x86, 0x2b, 0xdb, 0x6d, 0xd1, 0xd8, 0xb2, 0x61, 0x6b, 0x17, 0xd5, 0xd0, 0x62, 0xd6, 0x6d,
	0xf1, 0xc9, 0x07, 0xf7, 0xd2, 0x43, 0x2f, 0x4d, 0x20, 0x20, 0x70, 0x42, 0x90, 0x8f, 0x81, 0x08,
	0x59, 0x1a, 0x27, 0x4b, 0xa4, 0x5d, 0x59, 0xbb, 0x32, 0xe8, 0x9a, 0xd7, 0xc8, 0x0b, 0xe5, 0xb1,
	0x82, 0x57, 0x52, 0x12, 0x29, 0x06, 0xe7, 0xa6, 0xf9, 0x67, 0x66, 0xff, 0xfd, 0xfe, 0x15, 0x0c,
	0xfc, 0x84, 0x49, 0x4c, 0x77, 0x98, 0xce, 0x93, 0x54, 0x28, 0x41, 0xfa, 0xcf, 0x82, 0xae, 0xe9,
	0x16, 0x46, 0x0e, 0xdf, 0x89, 0x3b, 0x3c, 0xcb, 0x78, 0xa0, 0x98, 0xe0, 0x2e, 0x6e, 0x33, 0x94,
	0x8a, 0x7c, 0x07, 0xf0, 0x83, 0x40, 0x64, 0x5c, 0x79, 0x2c, 0xb4, 0x8d, 0x89, 0x31, 0x33, 0x5d,
	0xb3, 0x54, 0x9c, 0x90, 0x4c, 0xc1, 0xda, 0x94, 0x1b, 0x1e, 0xf7, 0x63, 0xb4, 0x5b, 0x7a, 0xa2,
	0x57, 0x89, 0x17, 0x7e, 0x8c, 0x64, 0x08, 0x1f, 0x71, 0x87, 0x5c, 0xd9, 0xed, 0x89, 0x31, 0xeb,
	0xb9, 0x45, 0x41, 0xef, 0x0d, 0xf8, 0xd2, 0xf4, 0x4c, 0xa2, 0x7c, 0xef, 0x98, 0x16, 0xe6, 0xaf,
	0x1c, 0x4b, 0xc5, 0x09, 0x09, 0x81, 0xce, 0x5a, 0x84, 0xb9, 0x36, 0xea, 0xb9, 0xfa, 0x9b, 0xfc,
	0x85, 0x6f, 0x05, 0x8c, 0x27, 0x59, 0x88, 0x5e, 0xe4, 0x2b, 0xe4, 0x41, 0xee, 0x31, 0xee, 0xc5,
	0x52, 0x9b, 0xb6, 0xdd, 0x51, 0x31, 0xb0, 0x62, 0x21, 0x2e, 0x8b, 0xb6, 0xc3, 0xcf, 0x25, 0xfd,
	0x0a, 0xc3, 0x25, 0x93, 0xaa, 0xba, 0x81, 0x2c, 0xb1, 0xe9, 0x16, 0x48, 0x43, 0x7f, 0xc7, 0xd5,
	0xfe, 0x81, 0x59, 0x71, 0x4b, 0xbb, 0x35, 0x69, 0xcf, 0xba, 0x8b, 0x1f, 0xf3, 0x7a, 0xd0, 0xf3,
	0xea, 0xc4, 0x53, 0xc1, 0x37, 0xec, 0xc6, 0x7d, 0x59, 0xa0, 0x0f, 0x06, 0xf4, 0xeb, 0xdd, 0xb7,
	0xe9, 0x1a, 0x07, 0xd2, 0xa5, 0x60, 0x29, 0x16, 0xa3, 0xc8, 0x54, 0x09, 0xdc, 0xd2, 0xc0, 0xdd,
	0x52, 0xdc, 0x63, 0x92, 0xdf, 0x30, 0x88, 0x31, 0x16, 0xa9, 0xce, 0x64, 0x9d, 0x2b, 0xac, 0x62,
	0xb1, 0x0a, 0xd9, 0xe1, 0x27, 0x7b, 0x91, 0xd8, 0xf0, 0xf9, 0xd6, 0xe7, 0x61, 0x84, 0xa9, 0xdd,
	0xd1, 0x56, 0x55, 0xb9, 0x78, 0x34, 0xc0, 0xfc, 0x7f, 0xe9, 0xac, 0x34, 0x0a, 0xb9, 0x86, 0x7e,
	0xfd, 0xe9, 0xc8, 0xaf, 0x26, 0xe8, 0xc1, 0xdf, 0x69, 0x3c, 0x3d, 0x36, 0x96, 0x44, 0x39, 0xfd,
	0x40, 0xae, 0xc0, 0xaa, 0xc5, 0x4f, 0x7e, 0x36, 0xf7, 0x0e, 0xbd, 0xda, 0x98, 0x1e, 0x99, 0xd2,
	0x87, 0xaf, 0x3f, 0xe9, 0xde, 0x9f, 0xa7, 0x00, 0x00, 0x00, 0xff, 0xff, 0xf7, 0x6c, 0xbe, 0x03,
	0x15, 0x03, 0x00, 0x00,
}
