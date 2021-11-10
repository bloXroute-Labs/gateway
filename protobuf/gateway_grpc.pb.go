// Code generated by protoc-gen-go-grpc. DO NOT EDIT.

package gateway

import (
	context "context"
	grpc "google.golang.org/grpc"
	codes "google.golang.org/grpc/codes"
	status "google.golang.org/grpc/status"
)

// This is a compile-time assertion to ensure that this generated file
// is compatible with the grpc package it is being compiled against.
// Requires gRPC-Go v1.32.0 or later.
const _ = grpc.SupportPackageIsVersion7

// GatewayClient is the client API for Gateway service.
//
// For semantics around ctx use and closing/ending streaming RPCs, please refer to https://pkg.go.dev/google.golang.org/grpc/?tab=doc#ClientConn.NewStream.
type GatewayClient interface {
	Peers(ctx context.Context, in *PeersRequest, opts ...grpc.CallOption) (*PeersReply, error)
	TxStoreSummary(ctx context.Context, in *TxStoreRequest, opts ...grpc.CallOption) (*TxStoreReply, error)
	GetTx(ctx context.Context, in *GetBxTransactionRequest, opts ...grpc.CallOption) (*GetBxTransactionResponse, error)
	Stop(ctx context.Context, in *StopRequest, opts ...grpc.CallOption) (*StopReply, error)
	Version(ctx context.Context, in *VersionRequest, opts ...grpc.CallOption) (*VersionReply, error)
}

type gatewayClient struct {
	cc grpc.ClientConnInterface
}

func NewGatewayClient(cc grpc.ClientConnInterface) GatewayClient {
	return &gatewayClient{cc}
}

func (c *gatewayClient) Peers(ctx context.Context, in *PeersRequest, opts ...grpc.CallOption) (*PeersReply, error) {
	out := new(PeersReply)
	err := c.cc.Invoke(ctx, "/gateway.Gateway/Peers", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClient) TxStoreSummary(ctx context.Context, in *TxStoreRequest, opts ...grpc.CallOption) (*TxStoreReply, error) {
	out := new(TxStoreReply)
	err := c.cc.Invoke(ctx, "/gateway.Gateway/TxStoreSummary", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClient) GetTx(ctx context.Context, in *GetBxTransactionRequest, opts ...grpc.CallOption) (*GetBxTransactionResponse, error) {
	out := new(GetBxTransactionResponse)
	err := c.cc.Invoke(ctx, "/gateway.Gateway/GetTx", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClient) Stop(ctx context.Context, in *StopRequest, opts ...grpc.CallOption) (*StopReply, error) {
	out := new(StopReply)
	err := c.cc.Invoke(ctx, "/gateway.Gateway/Stop", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

func (c *gatewayClient) Version(ctx context.Context, in *VersionRequest, opts ...grpc.CallOption) (*VersionReply, error) {
	out := new(VersionReply)
	err := c.cc.Invoke(ctx, "/gateway.Gateway/Version", in, out, opts...)
	if err != nil {
		return nil, err
	}
	return out, nil
}

// GatewayServer is the server API for Gateway service.
// All implementations must embed UnimplementedGatewayServer
// for forward compatibility
type GatewayServer interface {
	Peers(context.Context, *PeersRequest) (*PeersReply, error)
	TxStoreSummary(context.Context, *TxStoreRequest) (*TxStoreReply, error)
	GetTx(context.Context, *GetBxTransactionRequest) (*GetBxTransactionResponse, error)
	Stop(context.Context, *StopRequest) (*StopReply, error)
	Version(context.Context, *VersionRequest) (*VersionReply, error)
	mustEmbedUnimplementedGatewayServer()
}

// UnimplementedGatewayServer must be embedded to have forward compatible implementations.
type UnimplementedGatewayServer struct {
}

func (UnimplementedGatewayServer) Peers(context.Context, *PeersRequest) (*PeersReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Peers not implemented")
}
func (UnimplementedGatewayServer) TxStoreSummary(context.Context, *TxStoreRequest) (*TxStoreReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method TxStoreSummary not implemented")
}
func (UnimplementedGatewayServer) GetTx(context.Context, *GetBxTransactionRequest) (*GetBxTransactionResponse, error) {
	return nil, status.Errorf(codes.Unimplemented, "method GetTx not implemented")
}
func (UnimplementedGatewayServer) Stop(context.Context, *StopRequest) (*StopReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Stop not implemented")
}
func (UnimplementedGatewayServer) Version(context.Context, *VersionRequest) (*VersionReply, error) {
	return nil, status.Errorf(codes.Unimplemented, "method Version not implemented")
}
func (UnimplementedGatewayServer) mustEmbedUnimplementedGatewayServer() {}

// UnsafeGatewayServer may be embedded to opt out of forward compatibility for this service.
// Use of this interface is not recommended, as added methods to GatewayServer will
// result in compilation errors.
type UnsafeGatewayServer interface {
	mustEmbedUnimplementedGatewayServer()
}

func RegisterGatewayServer(s grpc.ServiceRegistrar, srv GatewayServer) {
	s.RegisterService(&Gateway_ServiceDesc, srv)
}

func _Gateway_Peers_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(PeersRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayServer).Peers(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.Gateway/Peers",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayServer).Peers(ctx, req.(*PeersRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gateway_TxStoreSummary_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(TxStoreRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayServer).TxStoreSummary(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.Gateway/TxStoreSummary",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayServer).TxStoreSummary(ctx, req.(*TxStoreRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gateway_GetTx_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(GetBxTransactionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayServer).GetTx(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.Gateway/GetTx",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayServer).GetTx(ctx, req.(*GetBxTransactionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gateway_Stop_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(StopRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayServer).Stop(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.Gateway/Stop",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayServer).Stop(ctx, req.(*StopRequest))
	}
	return interceptor(ctx, in, info, handler)
}

func _Gateway_Version_Handler(srv interface{}, ctx context.Context, dec func(interface{}) error, interceptor grpc.UnaryServerInterceptor) (interface{}, error) {
	in := new(VersionRequest)
	if err := dec(in); err != nil {
		return nil, err
	}
	if interceptor == nil {
		return srv.(GatewayServer).Version(ctx, in)
	}
	info := &grpc.UnaryServerInfo{
		Server:     srv,
		FullMethod: "/gateway.Gateway/Version",
	}
	handler := func(ctx context.Context, req interface{}) (interface{}, error) {
		return srv.(GatewayServer).Version(ctx, req.(*VersionRequest))
	}
	return interceptor(ctx, in, info, handler)
}

// Gateway_ServiceDesc is the grpc.ServiceDesc for Gateway service.
// It's only intended for direct use with grpc.RegisterService,
// and not to be introspected or modified (even as a copy)
var Gateway_ServiceDesc = grpc.ServiceDesc{
	ServiceName: "gateway.Gateway",
	HandlerType: (*GatewayServer)(nil),
	Methods: []grpc.MethodDesc{
		{
			MethodName: "Peers",
			Handler:    _Gateway_Peers_Handler,
		},
		{
			MethodName: "TxStoreSummary",
			Handler:    _Gateway_TxStoreSummary_Handler,
		},
		{
			MethodName: "GetTx",
			Handler:    _Gateway_GetTx_Handler,
		},
		{
			MethodName: "Stop",
			Handler:    _Gateway_Stop_Handler,
		},
		{
			MethodName: "Version",
			Handler:    _Gateway_Version_Handler,
		},
	},
	Streams:  []grpc.StreamDesc{},
	Metadata: "gateway.proto",
}