package nodes

import (
	"context"
	"errors"
	"fmt"
	"net"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
)

const (
	windowSize = 128 * 1024
	// bufferSize determines how much data can be batched before doing a write
	// on the wire. Zero or negative values will disable the write buffer such that each
	// write will be on underlying connection.
	bufferSize = 0
)

type gatewayGRPCServer struct {
	gateway     *gateway
	listenAddr  string
	encodedAuth string
	server      *grpc.Server
}

func newGatewayGRPCServer(gateway *gateway, host string, port int, user string, secret string) *gatewayGRPCServer {
	grpcHostPort := fmt.Sprintf("%v:%v", host, port)

	var encodedAuth string
	if user != "" && secret != "" {
		encodedAuth = rpc.EncodeUserSecret(user, secret)
	} else {
		encodedAuth = ""
	}

	return &gatewayGRPCServer{
		gateway:     gateway,
		listenAddr:  grpcHostPort,
		encodedAuth: encodedAuth,
	}
}

func (ggs *gatewayGRPCServer) Start() error {
	return ggs.run()
}

func (ggs *gatewayGRPCServer) Stop() {
	if ggs.server != nil {
		ggs.server.Stop()
	}
}

func (ggs *gatewayGRPCServer) run() error {
	listener, err := net.Listen("tcp", ggs.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	serverOptions := []grpc.ServerOption{
		grpc.WriteBufferSize(bufferSize),
		grpc.InitialConnWindowSize(windowSize),
		grpc.UnaryInterceptor(ggs.authenticate),
		grpc.ChainUnaryInterceptor(ggs.authenticate, ggs.reqSDKStats),
	}

	ggs.server = grpc.NewServer(serverOptions...)
	pb.RegisterGatewayServer(ggs.server, ggs.gateway)

	log.Infof("GRPC server is starting on %v", ggs.listenAddr)

	err = ggs.server.Serve(listener)
	if err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}

	return nil
}

func (ggs *gatewayGRPCServer) authenticate(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if ggs.encodedAuth != "" {
		auth, err := rpc.ReadAuthMetadata(ctx)
		if err != nil {
			return nil, err
		}

		if auth != ggs.encodedAuth {
			return nil, errors.New("provided auth information was incorrect")
		}
	}
	return handler(ctx, req)
}

func (ggs *gatewayGRPCServer) reqSDKStats(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	start := time.Now()

	resp, err := handler(ctx, req)

	if ok && len(md.Get(types.SDKVersionHeaderKey)) > 0 {
		method := string(jsonrpc.RPCMethodToRPCRequestType[info.FullMethod])
		if method == "" {
			// in case the method is not mapped
			method = info.FullMethod
		}
		go ggs.sdkStat(md, method, start)
	}

	return resp, err
}

func (ggs *gatewayGRPCServer) sdkStat(md metadata.MD, method string, start time.Time) {
	var blockchain, sourceCode, version string

	if blockchainHeader := md.Get(types.SDKBlockchainHeaderKey); len(blockchainHeader) > 0 {
		blockchain = blockchainHeader[0]
	}
	if sourceCodeHeader := md.Get(types.SDKCodeLanguageHeaderKey); len(sourceCodeHeader) > 0 {
		sourceCode = sourceCodeHeader[0]
	}
	if sdkVersionHeader := md.Get(types.SDKVersionHeaderKey); len(sdkVersionHeader) > 0 {
		version = sdkVersionHeader[0]
	}

	// blockchain, method, feed, sourceCode string, start, end time.Time, count int
	ggs.gateway.stats.LogSDKInfo(blockchain, method, sourceCode, version, types.GRPCFeed, start, time.Now())
}
