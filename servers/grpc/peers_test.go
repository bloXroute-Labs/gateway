package grpc

import (
	"context"
	"fmt"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
)

func TestServerPeers(t *testing.T) {
	port := test.NextTestPort()
	ctx, cancel := context.WithCancel(context.Background())
	testServer, feedMngr := testGRPCServer(t, port, "", "")
	wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, feedMngr)
	defer func() {
		testServer.Shutdown()
		cancel()
		require.NoError(t, wait())
	}()

	ctl := gomock.NewController(t)
	mockedSdn := mock.NewMockSDNHTTP(ctl)
	mockedSdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
	testServer.gatewayServer.(*server).params.sdn = mockedSdn

	req1 := &pb.PeersRequest{
		Type: "type1",
	}
	req2 := &pb.PeersRequest{
		Type: "type2",
	}

	// create conditions based on the request type
	mockedConnector := mock.NewMockConnector(ctl)
	cond1 := gomock.Cond(func(x any) bool { return x.(string) == "type1" })
	cond2 := gomock.Cond(func(x any) bool { return x.(string) == "type2" })
	mockedConnector.EXPECT().Peers(cond1).Return([]bxmessage.PeerInfo{}).AnyTimes()
	mockedConnector.EXPECT().Peers(cond2).Return([]bxmessage.PeerInfo{{IP: "1.2.3.4"}}).AnyTimes()
	testServer.gatewayServer.(*server).params.connector = mockedConnector

	defer func() {
		testServer.Shutdown()
		cancel()
		require.NoError(t, wait())
	}()

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	peersCall := func(req *pb.PeersRequest) *pb.PeersReply {
		res, err := rpc.GatewayCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Peers(ctx, req)
		})
		require.NoError(t, err)
		peersReply, ok := res.(*pb.PeersReply)
		assert.True(t, ok)

		return peersReply
	}

	peers := peersCall(req1)
	assert.Equal(t, 0, len(peers.GetPeers()))

	peers = peersCall(req2)
	require.Equal(t, 1, len(peers.GetPeers()))
	peer := peers.GetPeers()[0]
	assert.Equal(t, "1.2.3.4", peer.Ip)
}
