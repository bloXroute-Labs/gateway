package servers

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
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"google.golang.org/grpc"
	"google.golang.org/grpc/metadata"
	"google.golang.org/grpc/peer"
)

const (
	windowSize = 128 * 1024
	// bufferSize determines how much data can be batched before doing a writing
	// on the wire. Zero or negative values will disable the write buffer such that each
	// writing will be on underlying connection.
	bufferSize = 0
)

// GRPCServer implementation for grpc server
type GRPCServer struct {
	listenAddr       string
	encodedAuth      string
	stats            statistics.Stats
	gatewayAccountID types.AccountID
	server           *grpc.Server
}

// NewGRPCServer is the constructor of the GRPCServer object
func NewGRPCServer(host string, port int, user string, secret string, stats statistics.Stats, accountID types.AccountID, g pb.GatewayServer) *GRPCServer {
	grpcHostPort := fmt.Sprintf("%v:%v", host, port)

	var encodedAuth string
	if user != "" && secret != "" {
		encodedAuth = rpc.EncodeUserSecret(user, secret)
	} else {
		encodedAuth = ""
	}
	gRPCServer := &GRPCServer{
		listenAddr:       grpcHostPort,
		encodedAuth:      encodedAuth,
		stats:            stats,
		gatewayAccountID: accountID,
	}

	serverOptions := []grpc.ServerOption{
		grpc.WriteBufferSize(bufferSize),
		grpc.InitialConnWindowSize(windowSize),
		grpc.UnaryInterceptor(gRPCServer.authenticate),
		grpc.ChainUnaryInterceptor(gRPCServer.authenticate, gRPCServer.reqSDKStats),
	}
	gRPCServer.server = grpc.NewServer(serverOptions...)
	pb.RegisterGatewayServer(gRPCServer.server, g)
	return gRPCServer
}

// Run run grpc server
func (gs *GRPCServer) Run() error {
	listener, err := net.Listen("tcp", gs.listenAddr)
	if err != nil {
		return fmt.Errorf("failed to listen: %v", err)
	}

	log.Infof("GRPC server is starting on %v", gs.listenAddr)

	err = gs.server.Serve(listener)
	if err != nil {
		return fmt.Errorf("failed to serve: %v", err)
	}

	return nil
}

// Shutdown shutdown grpc server
func (gs *GRPCServer) Shutdown() {
	log.Infof("shutting down gRPC server")
	gs.server.Stop()
}

func (gs *GRPCServer) authenticate(ctx context.Context, req interface{}, _ *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	if gs.encodedAuth != "" {
		auth, err := rpc.ReadAuthMetadata(ctx)
		if err != nil {
			return nil, err
		}

		if auth != gs.encodedAuth {
			return nil, errors.New("provided auth information was incorrect")
		}
	}
	return handler(ctx, req)
}

func (gs *GRPCServer) reqSDKStats(ctx context.Context, req interface{}, info *grpc.UnaryServerInfo, handler grpc.UnaryHandler) (interface{}, error) {
	md, ok := metadata.FromIncomingContext(ctx)
	start := time.Now()

	resp, err := handler(ctx, req)

	if ok && len(md.Get(types.SDKVersionHeaderKey)) > 0 {
		method := string(jsonrpc.RPCMethodToRPCRequestType[info.FullMethod])
		if method == "" {
			// in case the method is not mapped
			method = info.FullMethod
		}
		go gs.sdkStat(md, method, start)
	}

	return resp, err
}

func (gs *GRPCServer) sdkStat(md metadata.MD, method string, start time.Time) {
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

	// blockchain, method, Feed, sourceCode string, start, end time.Time, count int
	gs.stats.LogSDKInfo(blockchain, method, sourceCode, version, gs.gatewayAccountID, types.GRPCFeed, start, time.Now())
}

// GetPeerAddr returns the address of the gRPC connected client given its context
func GetPeerAddr(ctx context.Context) string {
	var peerAddress string
	if p, ok := peer.FromContext(ctx); ok {
		peerAddress = p.Addr.String()
	}
	return peerAddress
}
