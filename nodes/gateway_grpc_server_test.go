package nodes

import (
	"context"
	"fmt"
	"net/http"
	"runtime"
	"testing"
	"time"

	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	mock_connections "github.com/bloXroute-Labs/gateway/v2/test/sdnhttpmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/version"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
)

const (
	testGatewayAccountID      = "user"
	testGatewaySecretHash     = "password"
	testTierName              = sdnmessage.ATierUltra
	testGatewayUserAuthHeader = "dXNlcjpwYXNzd29yZA==" // encoded testGatewayAccountID and testGatewaySecretHash
	testDifferentAccountID    = "user2"
	testDifferentSecretHash   = "password2"
	testDifferentAuthHeader   = "dXNlcjI6cGFzc3dvcmQy" // encoded testDifferentAccountId and testDifferentSecretHash
	testWrongAuthHeader       = "dXNlcjM6cGFzc3dvcmQz" // encoded user3 and password3

)

var (
	testAccountModel = sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.AccountID(testGatewayAccountID),
			TierName:  testGatewaySecretHash,
		},
		SecretHash: testGatewaySecretHash,
	}
)

func spawnGRPCServer(t *testing.T, port int, user string, password string) (*gateway, blockchain.Bridge, *gatewayGRPCServer) {
	serverConfig := config.NewGRPC("0.0.0.0", port, user, password)
	bridge, g := setup(t, 1)
	g.BxConfig.GRPC = serverConfig
	g.grpcHandler = servers.NewGrpcHandler(g.feedManager)
	s := newGatewayGRPCServer(g, serverConfig.Host, serverConfig.Port, serverConfig.User, serverConfig.Password)
	go func() {
		_ = s.Start()
	}()

	// small sleep for goroutine to start
	time.Sleep(1 * time.Millisecond)

	return g, bridge, s
}

func TestGatewayGRPCServerNoAuth(t *testing.T) {
	port := test.NextTestPort()

	_, _, s := spawnGRPCServer(t, port, "", "")
	defer s.Stop()

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")
	res, err := rpc.GatewayCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{})
	})

	require.Nil(t, err)

	versionReply, ok := res.(*pb.VersionReply)
	require.True(t, ok)
	require.Equal(t, version.BuildVersion, versionReply.GetVersion())
}

func TestGatewayGRPCServerAuth(t *testing.T) {
	port := test.NextTestPort()

	g, _, s := spawnGRPCServer(t, port, testGatewayAccountID, testGatewaySecretHash)
	defer s.Stop()

	ctl := gomock.NewController(t)
	mockedSdn := mock_connections.NewMockSDNHTTP(ctl)
	g.sdn = mockedSdn
	mockedSdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()

	authorizedClientConfig := config.NewGRPC("127.0.0.1", port, testGatewayAccountID, testGatewaySecretHash)
	res, err := rpc.GatewayCall(authorizedClientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{})
	})

	require.Nil(t, err)

	versionReply, ok := res.(*pb.VersionReply)
	require.True(t, ok)
	require.Equal(t, version.BuildVersion, versionReply.GetVersion())

	unauthorizedClientConfig := config.NewGRPC("127.0.0.1", port, "user", "wrongpassword")
	res, err = rpc.GatewayCall(unauthorizedClientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{})
	})

	require.NotNil(t, err)
}

func TestGatewayGRPCGatewayUserHeaderAuth(t *testing.T) {
	port := test.NextTestPort()

	g, _, s := spawnGRPCServer(t, port, "", "")
	defer s.Stop()

	ctl := gomock.NewController(t)
	mockedSdn := mock_connections.NewMockSDNHTTP(ctl)
	g.sdn = mockedSdn

	// there is no need to mock FetchCustomerAccountModel because
	// it's called only when not gateway user header auth is used

	// account model it's a gateway account
	mockedSdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()

	res, err := rpc.GatewayCall(&config.GRPC{Host: "127.0.0.1", Port: port, AuthEnabled: true, EncodedAuthSet: true, EncodedAuth: testGatewayUserAuthHeader, Timeout: 1 * time.Second}, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{})
	})

	require.Nil(t, err)

	versionReply, ok := res.(*pb.VersionReply)
	require.True(t, ok)
	require.Equal(t, version.BuildVersion, versionReply.GetVersion())
}

func TestGatewayGRPCNotGatewayUserHeaderAuth(t *testing.T) {
	port := test.NextTestPort()

	g, _, s := spawnGRPCServer(t, port, "", "")
	defer s.Stop()

	ctl := gomock.NewController(t)
	mockedSdn := mock_connections.NewMockSDNHTTP(ctl)

	fetchCustomerAccountModel := sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.AccountID(testDifferentAccountID),
			TierName:  sdnmessage.ATierUltra,
		},
		SecretHash: testDifferentSecretHash,
	}
	mockedSdn.EXPECT().FetchCustomerAccountModel(gomock.Any()).Return(fetchCustomerAccountModel, nil).AnyTimes()
	// account model it's a gateway account
	mockedSdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
	g.sdn = mockedSdn

	res, err := rpc.GatewayCall(&config.GRPC{Host: "127.0.0.1", Port: port, AuthEnabled: true, EncodedAuthSet: true, EncodedAuth: testDifferentAuthHeader, Timeout: 1 * time.Second}, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{})
	})

	require.Nil(t, err)

	versionReply, ok := res.(*pb.VersionReply)
	require.True(t, ok)
	require.Equal(t, version.BuildVersion, versionReply.GetVersion())
}

func TestInternalGatewayUnauthorizedAccess(t *testing.T) {

	port := test.NextTestPort()

	g, _, s := spawnGRPCServer(t, port, "", "")
	defer s.Stop()

	ctl := gomock.NewController(t)

	accountModel := sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.AccountID("account_id"),
			TierName:  "testTierName",
		},
		SecretHash: "account_id_pass",
	}

	fetchCustomerAccountModel := sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.AccountID("undefined_user"),
			TierName:  sdnmessage.ATierUltra,
		},
		SecretHash: "undefined_user_pass",
	}

	mockedSdn := mock_connections.NewMockSDNHTTP(ctl)
	g.sdn = mockedSdn

	mockedSdn.EXPECT().FetchCustomerAccountModel(gomock.Any()).Return(fetchCustomerAccountModel, fmt.Errorf("error %v", http.StatusNotFound)).AnyTimes()
	mockedSdn.EXPECT().AccountModel().Return(accountModel).AnyTimes()

	authorizedClientConfig := config.NewGRPC("127.0.0.1", port, "user", "password")

	_, err := rpc.GatewayCall(authorizedClientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{AuthHeader: "dXNlcjM6cGFzc3dvcmQz"})
	})

	require.NotNil(t, err)

	mockedSdn = mock_connections.NewMockSDNHTTP(ctl)
	g.sdn = mockedSdn

	mockedSdn.EXPECT().FetchCustomerAccountModel(gomock.Any()).Return(fetchCustomerAccountModel, fmt.Errorf("error %v", http.StatusUnauthorized)).AnyTimes()
	mockedSdn.EXPECT().AccountModel().Return(accountModel).AnyTimes()
	_, err = rpc.GatewayCall(authorizedClientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{AuthHeader: "dXNlcjM6cGFzc3dvcmQz"})
	})

	require.NotNil(t, err)

	mockedSdn = mock_connections.NewMockSDNHTTP(ctl)
	g.sdn = mockedSdn
	mockedSdn.EXPECT().FetchCustomerAccountModel(gomock.Any()).Return(fetchCustomerAccountModel, fmt.Errorf("error %v", http.StatusBadRequest)).AnyTimes()
	mockedSdn.EXPECT().AccountModel().Return(accountModel).AnyTimes()
	_, err = rpc.GatewayCall(authorizedClientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{AuthHeader: "dXNlcjM6cGFzc3dvcmQz"})
	})

	require.NotNil(t, err)
}

func TestGatewayGRPCServerPeers(t *testing.T) {
	port := test.NextTestPort()

	g, _, s := spawnGRPCServer(t, port, "", "")
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
	assert.Equal(t, conn.GetPeerIP(), peer.Ip)
}

func TestGatewayGRPCNewTxs(t *testing.T) {
	port := test.NextTestPort()

	g, _, s := spawnGRPCServer(t, port, "", "")
	defer s.Stop()
	g.BxConfig.WebsocketEnabled = true

	go g.feedManager.Start(context.Background())

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.NewTxs(ctx, &pb.TxsRequest{AuthHeader: "Og=="})
		require.NoError(t, err)

		time.Sleep(time.Millisecond)
		runtime.Gosched()

		newTxsStream, ok := res.(pb.Gateway_NewTxsClient)
		require.True(t, ok)

		_, relayConn1 := addRelayConn(g)
		_, deliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0)

		err = g.HandleMsg(deliveredTxMessage, relayConn1, connections.RunForeground)
		require.NoError(t, err)

		txNotification, err := newTxsStream.Recv()
		require.Nil(t, err)
		require.NotNil(t, txNotification.Tx)
		require.Equal(t, 1, len(txNotification.Tx))

		return txNotification, err
	})
}

func TestGatewayGRPCPendingTxs(t *testing.T) {
	port := test.NextTestPort()

	g, bridge, s := spawnGRPCServer(t, port, "", "")
	defer s.Stop()
	g.BxConfig.WebsocketEnabled = true

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	go g.feedManager.Start(context.Background())

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.PendingTxs(ctx, &pb.TxsRequest{AuthHeader: "Og=="})
		assert.Nil(t, err)

		pendingTxsStream, ok := res.(pb.Gateway_PendingTxsClient)
		assert.True(t, ok)

		ethTx, _ := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil)
		processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])

		txNotification, err := pendingTxsStream.Recv()
		assert.Nil(t, err)
		assert.NotNil(t, txNotification.Tx)
		assert.Equal(t, 1, len(txNotification.Tx))

		return txNotification, err
	})
}

func TestGatewayGRPCNewBlocks(t *testing.T) {
	port := test.NextTestPort()

	g, bridge, s := spawnGRPCServer(t, port, "", "")
	defer s.Stop()
	g.BxConfig.WebsocketEnabled = true
	g.BxConfig.SendConfirmation = true

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	go g.feedManager.Start(context.Background())

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.NewBlocks(ctx, &pb.BlocksRequest{AuthHeader: "Og=="})
		assert.Nil(t, err)

		newBlocksStream, ok := res.(pb.Gateway_NewBlocksClient)
		assert.True(t, ok)

		ethBlock := bxmock.NewEthBlock(10, common.Hash{})
		bxBlock, err := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
		assert.Nil(t, err)

		_ = bridge.SendBlockToBDN(bxBlock, types.NodeEndpoint{IP: "1.1.1.1", Port: 1800})

		newBlocksNotification, err := newBlocksStream.Recv()
		assert.Nil(t, err)
		assert.NotNil(t, newBlocksNotification.Header)
		assert.Equal(t, ethBlock.Hash().String(), newBlocksNotification.Hash)
		assert.NotNil(t, newBlocksNotification.SubscriptionID)
		assert.Equal(t, 3, len(ethBlock.Transactions()))

		return newBlocksNotification, err
	})
}

func TestGatewayGRPCBdnBlocks(t *testing.T) {
	port := test.NextTestPort()

	g, bridge, s := spawnGRPCServer(t, port, "", "")
	defer s.Stop()
	g.BxConfig.WebsocketEnabled = true
	g.BxConfig.SendConfirmation = true
	_, relayConn1 := addRelayConn(g)
	txStore, bp := newBP()

	go func() {
		err := g.handleBridgeMessages(context.Background())
		require.NoError(t, err)
	}()

	go g.feedManager.Start(context.Background())

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")
	err := rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.BdnBlocks(ctx, &pb.BlocksRequest{AuthHeader: "Og=="})
		require.NoError(t, err)

		time.Sleep(time.Millisecond)
		runtime.Gosched()

		bdnBlocksStream, ok := res.(pb.Gateway_BdnBlocksClient)
		require.True(t, ok)

		ethBlock := bxmock.NewEthBlock(10, common.Hash{})
		bxBlock, err := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
		require.NoError(t, err)

		// compress a transaction
		bxTransaction, _ := bridge.TransactionBlockchainToBDN(ethBlock.Transactions()[0])
		txStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)
		g.TxStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)

		broadcastMessage, _, err := bp.BxBlockToBroadcast(bxBlock, networkNum, g.sdn.MinTxAge())
		require.NoError(t, err)

		err = g.HandleMsg(broadcastMessage, relayConn1, connections.RunForeground)
		require.NoError(t, err)

		bdnBlocksNotification, err := bdnBlocksStream.Recv()
		require.NoError(t, err)

		require.Nil(t, err)
		require.NotNil(t, bdnBlocksNotification.Header)
		require.Equal(t, ethBlock.Hash().String(), bdnBlocksNotification.Hash)
		require.NotNil(t, bdnBlocksNotification.SubscriptionID)
		require.Equal(t, 3, len(ethBlock.Transactions()))

		return bdnBlocksNotification, err
	})

	require.NoError(t, err)
}
