package nodes

import (
	"context"
	"errors"
	"fmt"
	"net"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"google.golang.org/grpc"
)

type gatewayGRPCServer struct {
	gateway     *gateway
	listenAddr  string
	encodedAuth string
	server      *grpc.Server
}

func newGatewayGRPCServer(gateway *gateway, host string, port int, user string, secret string) gatewayGRPCServer {
	grpcHostPort := fmt.Sprintf("%v:%v", host, port)

	var encodedAuth string
	if user != "" && secret != "" {
		encodedAuth = rpc.EncodeUserSecret(user, secret)
	} else {
		encodedAuth = ""
	}

	return gatewayGRPCServer{
		gateway:     gateway,
		listenAddr:  grpcHostPort,
		encodedAuth: encodedAuth,
	}
}

func (ggs *gatewayGRPCServer) Start() error {
	ggs.run()
	return nil
}

func (ggs *gatewayGRPCServer) Stop() {
	server := ggs.server
	if server != nil {
		ggs.server.Stop()
	}
}

func (ggs *gatewayGRPCServer) run() {
	listener, err := net.Listen("tcp", ggs.listenAddr)
	if err != nil {
		log.Fatalf("failed to listen: %v", err)
	}

	ggs.server = grpc.NewServer(grpc.UnaryInterceptor(ggs.authenticate))
	pb.RegisterGatewayServer(ggs.server, ggs.gateway)

	log.Infof("GRPC server is starting on %v", ggs.listenAddr)
	if err := ggs.server.Serve(listener); err != nil {
		log.Fatalf("failed to serve: %v", err)
	}
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
