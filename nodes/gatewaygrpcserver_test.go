package nodes

import (
	"context"
	"github.com/bloXroute-Labs/gateway/config"
	pb "github.com/bloXroute-Labs/gateway/protobuf"
	"github.com/bloXroute-Labs/gateway/rpc"
	"github.com/bloXroute-Labs/gateway/test"
	"github.com/bloXroute-Labs/gateway/version"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func spawnGRPCServer(port int, user string, password string) (*gateway, *gatewayGRPCServer) {
	serverConfig := config.NewGRPC("0.0.0.0", port, user, password)
	_, g := setup()
	g.BxConfig.GRPC = serverConfig
	s := newGatewayGRPCServer(g, serverConfig.Host, serverConfig.Port, serverConfig.User, serverConfig.Password)
	go func() {
		_ = s.Start()
	}()

	// small sleep for goroutine to start
	time.Sleep(1 * time.Millisecond)

	return g, &s
}

func TestGatewayGRPCServerNoAuth(t *testing.T) {
	port := test.NextTestPort()

	_, s := spawnGRPCServer(port, "", "")
	defer s.Stop()

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")
	res, err := rpc.GatewayCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{})
	})

	assert.Nil(t, err)

	versionReply, ok := res.(*pb.VersionReply)
	assert.True(t, ok)
	assert.Equal(t, version.BuildVersion, versionReply.GetVersion())
}

func TestGatewayGRPCServerAuth(t *testing.T) {
	port := test.NextTestPort()

	_, s := spawnGRPCServer(port, "user", "password")
	defer s.Stop()

	authorizedClientConfig := config.NewGRPC("127.0.0.1", port, "user", "password")
	res, err := rpc.GatewayCall(authorizedClientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{})
	})

	assert.Nil(t, err)

	versionReply, ok := res.(*pb.VersionReply)
	assert.True(t, ok)
	assert.Equal(t, version.BuildVersion, versionReply.GetVersion())

	unauthorizedClientConfig := config.NewGRPC("127.0.0.1", port, "user", "wrongpassword")
	res, err = rpc.GatewayCall(unauthorizedClientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{})
	})

	assert.Nil(t, res)
	assert.NotNil(t, err)
}

func TestGatewayGRPCServerPeers(t *testing.T) {
	port := test.NextTestPort()

	g, s := spawnGRPCServer(port, "", "")
	defer s.Stop()

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	peersCall := func() *pb.PeersReply {
		res, err := rpc.GatewayCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Peers(ctx, &pb.PeersRequest{})
		})

		assert.Nil(t, err)

		peersReply, ok := res.(*pb.PeersReply)
		assert.True(t, ok)
		return peersReply
	}

	peers := peersCall()
	assert.Equal(t, 0, len(peers.GetPeers()))

	_, conn := addRelayConn(g)

	peers = peersCall()
	assert.Equal(t, 1, len(peers.GetPeers()))

	peer := peers.GetPeers()[0]
	assert.Equal(t, conn.Info().PeerIP, peer.Ip)
}
