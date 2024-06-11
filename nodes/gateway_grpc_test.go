package nodes

import (
	"context"
	"crypto/ecdsa"
	"encoding/hex"
	"fmt"
	"math/big"
	"net/http"
	"runtime"
	"strings"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/utils/orderedmap"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/stretchr/testify/require"
	"go.uber.org/mock/gomock"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/test/mock"
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
	testWalletID              = "0x00112233445566778899AABBCCDDEEFFGGHHIIJJ"
	testWalletID2             = "0xAABBCCDDEEFFGGHHIIJJ00112233445566778899"
)

var testAccountModel = sdnmessage.Account{
	AccountInfo: sdnmessage.AccountInfo{
		AccountID: types.AccountID(testGatewayAccountID),
		TierName:  testGatewaySecretHash,
	},
	SecretHash: testGatewaySecretHash,
}

func spawnGRPCServer(t *testing.T, port int, user string, password string) (*gateway, blockchain.Bridge, *servers.GRPCServer, *GatewayGrpc) {
	serverConfig := config.NewGRPC("0.0.0.0", port, user, password)
	bridge, g := setup(t, 1)
	g.BxConfig.GRPC = serverConfig
	gwGrpc := NewGatewayGrpc(&g.Bx, GatewayGrpcParams{
		sdn: g.sdn, authorize: g.authorize, bridge: g.bridge, blockchainPeers: g.blockchainPeers, wsManager: g.wsManager,
		bdnStats: g.bdnStats, timeStarted: g.timeStarted, txsQueue: g.txsQueue, txsOrderQueue: g.txsOrderQueue, gatewayPublicKey: g.gatewayPublicKey,
		feedManager: g.feedManager, txFromFieldIncludable: false, blockProposer: g.blockProposer,
		feedManagerChan: g.feedManagerChan, intentsManager: services.NewIntentsManager(), grpcFeedManager: g.feedManager})
	grpcServer := servers.NewGRPCServer("0.0.0.0", port, user, password, g.stats, g.accountID, gwGrpc)
	go func() {
		_ = grpcServer.Run()
	}()

	test.WaitServerStarted(t, fmt.Sprintf("%v:%v", serverConfig.Host, serverConfig.Port))

	return g, bridge, grpcServer, gwGrpc
}

func TestGatewayGRPCServerNoAuth(t *testing.T) {
	port := test.NextTestPort()

	_, _, s, _ := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()

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

	g, _, s, gg := spawnGRPCServer(t, port, testGatewayAccountID, testGatewaySecretHash)
	defer s.Shutdown()

	ctl := gomock.NewController(t)
	mockedSdn := mock.NewMockSDNHTTP(ctl)
	g.sdn = mockedSdn
	gg.params.sdn = mockedSdn
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

	g, _, s, gg := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()

	ctl := gomock.NewController(t)
	mockedSdn := mock.NewMockSDNHTTP(ctl)
	g.sdn = mockedSdn
	gg.params.sdn = mockedSdn

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

	g, _, s, _ := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()

	ctl := gomock.NewController(t)
	mockedSdn := mock.NewMockSDNHTTP(ctl)

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

	g, _, s, _ := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()

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

	mockedSdn := mock.NewMockSDNHTTP(ctl)
	g.sdn = mockedSdn

	mockedSdn.EXPECT().FetchCustomerAccountModel(gomock.Any()).Return(fetchCustomerAccountModel, fmt.Errorf("error %v", http.StatusNotFound)).AnyTimes()
	mockedSdn.EXPECT().AccountModel().Return(accountModel).AnyTimes()

	authorizedClientConfig := config.NewGRPC("127.0.0.1", port, "user", "password")

	_, err := rpc.GatewayCall(authorizedClientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{AuthHeader: "dXNlcjM6cGFzc3dvcmQz"})
	})

	require.NotNil(t, err)

	mockedSdn = mock.NewMockSDNHTTP(ctl)
	g.sdn = mockedSdn

	mockedSdn.EXPECT().FetchCustomerAccountModel(gomock.Any()).Return(fetchCustomerAccountModel, fmt.Errorf("error %v", http.StatusUnauthorized)).AnyTimes()
	mockedSdn.EXPECT().AccountModel().Return(accountModel).AnyTimes()
	_, err = rpc.GatewayCall(authorizedClientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		return client.Version(ctx, &pb.VersionRequest{AuthHeader: "dXNlcjM6cGFzc3dvcmQz"})
	})

	require.NotNil(t, err)

	mockedSdn = mock.NewMockSDNHTTP(ctl)
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

	/*
		port := test.NextTestPort()

			_, _, s, _ := spawnGRPCServer(t, port, "", "")
			defer s.Shutdown()

			clientConfig := config.NewGRPC("127.0.0.1", port, "", "")
			res, err := rpc.GatewayCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				return client.Version(ctx, &pb.VersionRequest{})
			})

			require.Nil(t, err)
	*/

	g, _, s, _ := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	peersCall := func() *pb.PeersReply {
		res, err := rpc.GatewayCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
			return client.Peers(ctx, &pb.PeersRequest{})
		})

		assert.NoError(t, err)

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

	g, _, s, _ := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()
	g.BxConfig.WebsocketEnabled = true

	go g.feedManager.Start(context.Background())

	time.Sleep(5 * time.Millisecond)

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.NewTxs(ctx, &pb.TxsRequest{AuthHeader: "Og=="})
		require.NoError(t, err)

		time.Sleep(time.Millisecond)
		runtime.Gosched()

		newTxsStream, ok := res.(pb.Gateway_NewTxsClient)
		require.True(t, ok)

		_, relayConn1 := addRelayConn(g)
		_, deliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0, nil)

		err = g.HandleMsg(deliveredTxMessage, relayConn1, connections.RunForeground)
		require.NoError(t, err)

		test.WaitChanConsumed(t, g.feedManagerChan)

		txNotification, err := newTxsStream.Recv()
		require.NoError(t, err)
		require.NotNil(t, txNotification.Tx)
		require.Equal(t, 1, len(txNotification.Tx))

		return txNotification, err
	})
}

// TestGatewayGRPCNewTxs_withNonTxIncludes tests that the gateway will not crash if the client includes non-tx fields
func TestGatewayGRPCNewTxs_withNonTxIncludes(t *testing.T) {
	port := test.NextTestPort()

	g, _, s, _ := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()
	g.BxConfig.WebsocketEnabled = true

	go g.feedManager.Start(context.Background())

	time.Sleep(5 * time.Millisecond)

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.NewTxs(ctx, &pb.TxsRequest{AuthHeader: "Og==", Filters: "", Includes: []string{"tx_hash", "time"}})
		require.NoError(t, err)

		time.Sleep(time.Millisecond)
		runtime.Gosched()

		newTxsStream, ok := res.(pb.Gateway_NewTxsClient)
		require.True(t, ok)

		_, relayConn1 := addRelayConn(g)
		_, deliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0, nil)

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

	g, _, s, _ := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()
	g.BxConfig.WebsocketEnabled = true

	go g.feedManager.Start(context.Background())

	time.Sleep(5 * time.Millisecond)

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.PendingTxs(ctx, &pb.TxsRequest{AuthHeader: "Og=="})
		assert.NoError(t, err)

		time.Sleep(time.Millisecond)
		runtime.Gosched()

		pendingTxsStream, ok := res.(pb.Gateway_PendingTxsClient)
		assert.True(t, ok)

		_, deliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0, big.NewInt(network.EthMainnetChainID))

		err = g.HandleMsg(deliveredTxMessage, connections.NewBlockchainConn(types.NodeEndpoint{IP: "1.1.1.1", Port: 1800}), connections.RunForeground)
		require.NoError(t, err)

		test.WaitChanConsumed(t, g.feedManagerChan)

		txNotification, err := pendingTxsStream.Recv()
		assert.NoError(t, err)
		assert.NotNil(t, txNotification.Tx)
		assert.Equal(t, 1, len(txNotification.Tx))

		return txNotification, err
	})
}

func TestGatewayGRPCNewBlocks(t *testing.T) {
	port := test.NextTestPort()

	g, bridge, s, _ := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()
	g.BxConfig.WebsocketEnabled = true
	g.BxConfig.SendConfirmation = true

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.NoError(t, err)
	}()

	go g.feedManager.Start(context.Background())

	time.Sleep(5 * time.Millisecond)

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.NewBlocks(ctx, &pb.BlocksRequest{AuthHeader: "Og=="})
		assert.NoError(t, err)

		newBlocksStream, ok := res.(pb.Gateway_NewBlocksClient)
		assert.True(t, ok)

		ethBlock := bxmock.NewEthBlock(10, common.Hash{})
		bxBlock, err := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
		assert.NoError(t, err)

		_ = bridge.SendBlockToBDN(bxBlock, types.NodeEndpoint{IP: "1.1.1.1", Port: 1800})

		test.WaitChanConsumed(t, bridge.ReceiveBlockFromNode())
		test.WaitChanConsumed(t, g.feedManagerChan)

		newBlocksNotification, err := newBlocksStream.Recv()
		assert.NoError(t, err)
		assert.NotNil(t, newBlocksNotification.Header)
		assert.Equal(t, ethBlock.Hash().String(), newBlocksNotification.Hash)
		assert.NotNil(t, newBlocksNotification.SubscriptionID)
		assert.Equal(t, 4, len(ethBlock.Transactions()))

		return newBlocksNotification, err
	})
}

func TestGatewayGRPCBdnBlocks(t *testing.T) {
	port := test.NextTestPort()

	g, bridge, s, _ := spawnGRPCServer(t, port, "", "")
	defer s.Shutdown()
	g.BxConfig.WebsocketEnabled = true
	g.BxConfig.SendConfirmation = true
	_, relayConn1 := addRelayConn(g)
	txStore, bp := newBP()

	go func() {
		err := g.handleBridgeMessages(context.Background())
		require.NoError(t, err)
	}()

	go g.feedManager.Start(context.Background())

	time.Sleep(5 * time.Millisecond)

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

		test.WaitChanConsumed(t, g.feedManagerChan)

		bdnBlocksNotification, err := bdnBlocksStream.Recv()
		require.NoError(t, err)

		require.Nil(t, err)
		require.NotNil(t, bdnBlocksNotification.Header)
		require.Equal(t, ethBlock.Hash().String(), bdnBlocksNotification.Hash)
		require.NotNil(t, bdnBlocksNotification.SubscriptionID)
		require.Equal(t, 4, len(ethBlock.Transactions()))

		return bdnBlocksNotification, err
	})

	require.NoError(t, err)
}

func TestGatewayGRPCBlxrTx(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	port := test.NextTestPort()

	generateLegacyTxAndHash := func() (string, string) {
		tx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
		return hex.EncodeToString(ethTxBytes), tx.Hash().String()
	}
	generateDynamicFeeTxAndHash := func() (string, string) {
		tx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, nil)
		return hex.EncodeToString(ethTxBytes), tx.Hash().String()
	}

	setupSdn := func() connections.SDNHTTP {
		ctl := gomock.NewController(t)
		sdn := mock.NewMockSDNHTTP(ctl)
		sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
		sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
		return sdn
	}

	setupFeedManager := func(g *gateway) *servers.FeedManager {
		return servers.NewFeedManager(g.context, g, g.feedManagerChan, nil, services.NewNoOpSubscriptionServices(),
			networkNum, types.NetworkID(10), g.sdn.NodeModel().NodeID,
			g.wsManager, g.sdn.AccountModel(), nil,
			"", "", *g.BxConfig, g.stats, nil, nil)
	}

	testCases := []struct {
		description          string
		setupSdnFunc         func() connections.SDNHTTP
		setupFeedManagerFunc func(*gateway) *servers.FeedManager
		request              *pb.BlxrTxRequest
		expectedErrSubStr    string
		generateTxAndHash    func() (string, string)
	}{
		{
			description:  "Wrong chainID",
			setupSdnFunc: setupSdn,
			setupFeedManagerFunc: func(g *gateway) *servers.FeedManager {
				return servers.NewFeedManager(g.context, g, g.feedManagerChan, nil, services.NewNoOpSubscriptionServices(),
					networkNum, types.NetworkID(chainID), g.sdn.NodeModel().NodeID,
					g.wsManager, g.sdn.AccountModel(), nil,
					"", "", *g.BxConfig, g.stats, nil, nil)
			},
			request:           &pb.BlxrTxRequest{},
			generateTxAndHash: generateLegacyTxAndHash,
			expectedErrSubStr: "chainID mismatch",
		},
		{
			description: "Empty account",
			setupSdnFunc: func() connections.SDNHTTP {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(sdnmessage.Account{}).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
				return sdn
			},
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrTxRequest{},
			generateTxAndHash:    generateLegacyTxAndHash,
			expectedErrSubStr:    "not authorized to call this method",
		},
		{
			description:          "Wrong transaction format",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrTxRequest{},
			generateTxAndHash: func() (string, string) {
				return "f800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000", "0x00"
			},

			expectedErrSubStr: "failed to unmarshal tx",
		},
		{
			description:          "Send legacy type transaction",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrTxRequest{},
			generateTxAndHash:    generateLegacyTxAndHash,
		},
		{
			description:          "Send dynamic fee type transaction",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrTxRequest{},
			generateTxAndHash:    generateDynamicFeeTxAndHash,
		},
		{
			description:          "Send transaction with node validation",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrTxRequest{
				NodeValidation: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description:          "Send transaction with validators_only",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrTxRequest{
				ValidatorsOnly: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description: "Send transaction with next validator",
			setupSdnFunc: func() connections.SDNHTTP {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(bxgateway.BSCMainnetNum).AnyTimes()
				return sdn
			},
			setupFeedManagerFunc: func(g *gateway) *servers.FeedManager {
				validatorStatusMap := syncmap.NewStringMapOf[bool]()
				validatorStatusMap.Store(testWalletID, true)
				validatorStatusMap.Store(testWalletID2, true)
				nextValidatorMap := orderedmap.New()
				nextValidatorMap.Set(1, testWalletID)
				nextValidatorMap.Set(2, testWalletID2)

				return servers.NewFeedManager(g.context, g, g.feedManagerChan, nil, services.NewNoOpSubscriptionServices(),
					bxgateway.BSCMainnetNum, types.NetworkID(10), g.sdn.NodeModel().NodeID,
					g.wsManager, g.sdn.AccountModel(), nil,
					"", "", *g.BxConfig, g.stats, nextValidatorMap, validatorStatusMap)
			},
			request: &pb.BlxrTxRequest{
				NextValidator: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description:  "Wrong network on next validator",
			setupSdnFunc: setupSdn,
			setupFeedManagerFunc: func(g *gateway) *servers.FeedManager {
				return servers.NewFeedManager(g.context, g, g.feedManagerChan, nil, services.NewNoOpSubscriptionServices(),
					1, types.NetworkID(10), g.sdn.NodeModel().NodeID,
					g.wsManager, g.sdn.AccountModel(), nil,
					"", "", *g.BxConfig, g.stats, nil, nil)
			}, request: &pb.BlxrTxRequest{
				NextValidator: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "currently next_validator is only supported on BSC and Polygon networks",
		},
		{
			description: "Nil validator map on next validator",
			setupSdnFunc: func() connections.SDNHTTP {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(bxgateway.BSCMainnetNum).AnyTimes()
				return sdn
			},
			setupFeedManagerFunc: func(g *gateway) *servers.FeedManager {
				return servers.NewFeedManager(g.context, g, g.feedManagerChan, nil, services.NewNoOpSubscriptionServices(),
					bxgateway.BSCMainnetNum, types.NetworkID(10), g.sdn.NodeModel().NodeID,
					g.wsManager, g.sdn.AccountModel(), nil,
					"", "", *g.BxConfig, g.stats, nil, nil)
			},
			request: &pb.BlxrTxRequest{
				NextValidator: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "failed to send next validator tx",
		},
		{
			description: "Empty validator map on next validator",
			setupSdnFunc: func() connections.SDNHTTP {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(bxgateway.BSCMainnetNum).AnyTimes()
				return sdn
			},
			setupFeedManagerFunc: func(g *gateway) *servers.FeedManager {
				validatorStatusMap := syncmap.NewStringMapOf[bool]()
				nextValidatorMap := orderedmap.New()

				return servers.NewFeedManager(g.context, g, g.feedManagerChan, nil, services.NewNoOpSubscriptionServices(),
					bxgateway.BSCMainnetNum, types.NetworkID(10), g.sdn.NodeModel().NodeID,
					g.wsManager, g.sdn.AccountModel(), nil,
					"", "", *g.BxConfig, g.stats, nextValidatorMap, validatorStatusMap)
			},
			request: &pb.BlxrTxRequest{
				NextValidator: true,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "can't send tx with next_validator because the gateway encountered an issue fetching the epoch block",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			g, _, s, gg := spawnGRPCServer(t, port, "", "")
			defer s.Shutdown()
			gg.params.feedManager = tc.setupFeedManagerFunc(g)
			go gg.params.feedManager.Start(context.Background())
			g.sdn = tc.setupSdnFunc()
			gg.params.sdn = tc.setupSdnFunc()

			tx, hash := tc.generateTxAndHash()
			tc.request.Transaction = tx

			clientConfig := &config.GRPC{
				Enabled:        true,
				Host:           "127.0.0.1",
				Port:           port,
				AuthEnabled:    true,
				EncodedAuthSet: true,
				EncodedAuth:    testGatewayUserAuthHeader,
				Timeout:        1 * time.Second,
			}

			_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				res, err := client.BlxrTx(ctx, tc.request)
				if tc.expectedErrSubStr != "" {
					require.True(t, strings.Contains(err.Error(), tc.expectedErrSubStr))
				} else {
					require.Nil(t, err)
					require.Equal(t, fmt.Sprintf("0x%v", res.TxHash), hash)
				}
				return nil, err
			})
		})
	}
}

func TestGatewayGRPCBlxrBatchTx(t *testing.T) {
	privKey, _ := crypto.GenerateKey()
	port := test.NextTestPort()

	generateLegacyTxAndHash := func() ([]string, []string) {
		tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
		tx2, ethTxBytes2 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
		return []string{hex.EncodeToString(ethTxBytes1), hex.EncodeToString(ethTxBytes2)}, []string{tx1.Hash().String(), tx2.Hash().String()}
	}
	generateDynamicFeeTxAndHash := func() ([]string, []string) {
		tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, nil)
		tx2, ethTxBytes2 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, nil)
		return []string{hex.EncodeToString(ethTxBytes1), hex.EncodeToString(ethTxBytes2)}, []string{tx1.Hash().String(), tx2.Hash().String()}
	}

	setupSdn := func() connections.SDNHTTP {
		ctl := gomock.NewController(t)
		sdn := mock.NewMockSDNHTTP(ctl)
		sdn.EXPECT().AccountModel().Return(testAccountModel).AnyTimes()
		sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
		return sdn
	}

	setupFeedManager := func(g *gateway) *servers.FeedManager {
		return servers.NewFeedManager(g.context, g, g.feedManagerChan, nil, services.NewNoOpSubscriptionServices(),
			networkNum, types.NetworkID(10), g.sdn.NodeModel().NodeID,
			g.wsManager, g.sdn.AccountModel(), nil,
			"", "", *g.BxConfig, g.stats, nil, nil)
	}

	testCases := []struct {
		description          string
		setupSdnFunc         func() connections.SDNHTTP
		setupFeedManagerFunc func(*gateway) *servers.FeedManager
		request              *pb.BlxrBatchTXRequest
		expectedErrSubStr    []string
		generateTxAndHash    func() ([]string, []string)
	}{
		{
			description:          "Send legacy type transaction",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrBatchTXRequest{},
			generateTxAndHash:    generateLegacyTxAndHash,
		},
		{
			description:          "Send dynamic fee type transaction",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrBatchTXRequest{},
			generateTxAndHash:    generateDynamicFeeTxAndHash,
		},
		{
			description:          "Batch limit exceeded",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrBatchTXRequest{},
			generateTxAndHash: func() ([]string, []string) {
				batchLimit := 10
				var txs []string
				var hashes []string

				for i := 0; i <= batchLimit; i++ {
					tx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
					txs = append(txs, hex.EncodeToString(ethTxBytes))
					hashes = append(hashes, tx.Hash().String())
				}
				return txs, hashes
			},
			expectedErrSubStr: []string{"blxr-batch-tx currently supports a maximum of"},
		},
		{
			description:          "First successful, second failed",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrBatchTXRequest{},
			generateTxAndHash: func() ([]string, []string) {
				tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
				return []string{hex.EncodeToString(ethTxBytes1), "00"}, []string{tx1.Hash().String(), "0x00"}
			},
			expectedErrSubStr: []string{"", "typed transaction too short"},
		},
		{
			description:          "First failed, second successful",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrBatchTXRequest{},
			generateTxAndHash: func() ([]string, []string) {
				tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, nil)
				return []string{"00", hex.EncodeToString(ethTxBytes1)}, []string{"0x00", tx1.Hash().String()}
			},
			expectedErrSubStr: []string{"typed transaction too short", ""},
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			g, _, s, gg := spawnGRPCServer(t, port, "", "")
			defer s.Shutdown()
			gg.params.feedManager = tc.setupFeedManagerFunc(g)
			go gg.params.feedManager.Start(context.Background())
			g.sdn = tc.setupSdnFunc()
			gg.params.sdn = tc.setupSdnFunc()

			txs, hashes := tc.generateTxAndHash()
			var txsAndSenders []*pb.TxAndSender
			var expectedHashes []string

			for idx, tx := range txs {
				txsAndSenders = append(txsAndSenders, &pb.TxAndSender{Transaction: tx})
				expectedHashes = append(expectedHashes, hashes[idx])
			}
			tc.request.TransactionsAndSenders = txsAndSenders

			clientConfig := &config.GRPC{
				Enabled:        true,
				Host:           "127.0.0.1",
				Port:           port,
				AuthEnabled:    true,
				EncodedAuthSet: true,
				EncodedAuth:    testGatewayUserAuthHeader,
				Timeout:        1 * time.Second,
			}

			_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				res, err := client.BlxrBatchTX(ctx, tc.request)
				require.Nil(t, err)
				for _, respErrAndIdx := range res.TxErrors {
					require.NotEqual(t, "", tc.expectedErrSubStr[respErrAndIdx.Idx])
					require.True(t, strings.Contains(respErrAndIdx.Error, tc.expectedErrSubStr[respErrAndIdx.Idx]))
				}
				for _, respHashAndIdx := range res.TxHashes {
					require.Equal(t, fmt.Sprintf("0x%v", respHashAndIdx.TxHash), expectedHashes[respHashAndIdx.Idx])
				}
				return nil, err
			})
		})
	}
}

func TestGatewaySubmitBundle(t *testing.T) {
	generateLegacyTxAndHash := func(privKey *ecdsa.PrivateKey, chain *big.Int) ([]string, []string) {
		tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, chain)
		tx2, ethTxBytes2 := bxmock.NewSignedEthTxBytes(ethtypes.LegacyTxType, 1, privKey, chain)
		return []string{hex.EncodeToString(ethTxBytes1), hex.EncodeToString(ethTxBytes2)}, []string{tx1.Hash().String(), tx2.Hash().String()}
	}
	generateDynamicFeeTxAndHash := func(privKey *ecdsa.PrivateKey, chain *big.Int) ([]string, []string) {
		tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, chain)
		tx2, ethTxBytes2 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, chain)
		return []string{hex.EncodeToString(ethTxBytes1), hex.EncodeToString(ethTxBytes2)}, []string{tx1.Hash().String(), tx2.Hash().String()}
	}

	setupSdn := func() connections.SDNHTTP {
		ctl := gomock.NewController(t)
		sdn := mock.NewMockSDNHTTP(ctl)
		sdn.EXPECT().AccountModel().Return(sdnmessage.Account{
			AccountInfo: sdnmessage.AccountInfo{
				AccountID: testGatewayAccountID,
				TierName:  testTierName,
			},
			SecretHash: testGatewaySecretHash,
		}).AnyTimes()
		sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
		return sdn
	}

	setupFeedManager := func(g *gateway) *servers.FeedManager {
		return servers.NewFeedManager(g.context, g, g.feedManagerChan, nil, services.NewNoOpSubscriptionServices(),
			networkNum, types.NetworkID(1), g.sdn.NodeModel().NodeID,
			g.wsManager, g.sdn.AccountModel(), nil,
			"", "", *g.BxConfig, g.stats, nil, nil)
	}

	testCases := []struct {
		description          string
		setupSdnFunc         func() connections.SDNHTTP
		setupFeedManagerFunc func(*gateway) *servers.FeedManager
		request              *pb.BlxrSubmitBundleRequest
		expectedErrSubStr    string
		privKey              *ecdsa.PrivateKey
		chainID              *big.Int
		generateTxAndHash    func(*ecdsa.PrivateKey, *big.Int) ([]string, []string)
	}{
		{
			description:          "Send legacy type transactions",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateLegacyTxAndHash,
		},
		{
			description:          "Send dynamic fee type transactions",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
		},
		{
			description: "Txs limit exceeded",
			setupSdnFunc: func() connections.SDNHTTP {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(sdnmessage.Account{
					AccountInfo: sdnmessage.AccountInfo{
						AccountID: testGatewayAccountID,
						TierName:  testTierName,
					},
					SecretHash: testGatewaySecretHash,
					Bundles: sdnmessage.BDNBundlesService{
						Networks: map[string]sdnmessage.BundleProperties{
							"Mainnet": {
								TxsLenLimit: 1,
							},
						},
					},
				}).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
				return sdn
			},
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "txs limit exceeded",
		},
		{
			description: "Account wrong tier",
			setupSdnFunc: func() connections.SDNHTTP {
				ctl := gomock.NewController(t)
				sdn := mock.NewMockSDNHTTP(ctl)
				sdn.EXPECT().AccountModel().Return(sdnmessage.Account{
					AccountInfo: sdnmessage.AccountInfo{
						AccountID: testGatewayAccountID,
						TierName:  sdnmessage.ATierIntroductory,
					},
					SecretHash: testGatewaySecretHash,
				}).AnyTimes()
				sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
				return sdn
			},
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "enterprise elite account is required in order to send bundle",
		},

		{
			description:          "Wrong chainID",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			chainID:           big.NewInt(10),
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "invalid chain id for signer",
		},
		{
			description:          "Bundle with missing block number",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrSubmitBundleRequest{},
			generateTxAndHash:    generateDynamicFeeTxAndHash,
			expectedErrSubStr:    "bundle missing blockNumber",
		},
		{
			description:          "Bundle with block number wrong format",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "1",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "blockNumber must be hex",
		},
		{
			description:          "Bundle with no txs",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request:              &pb.BlxrSubmitBundleRequest{},
			generateTxAndHash: func(privKey *ecdsa.PrivateKey, chain *big.Int) ([]string, []string) {
				return []string{}, []string{}
			},
			expectedErrSubStr: "bundle missing txs",
		},
		{
			description:          "Bundle with negative min timestamp",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber:  "0x1f71710",
				MinTimestamp: -1,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "min timestamp must be greater than or equal to 0",
		},
		{
			description:          "Bundle with negative max timestamp",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber:  "0x1f71710",
				MaxTimestamp: -1,
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "max timestamp must be greater than or equal to 0",
		},
		{
			description:          "Bundle with wrong UUID",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
				Uuid:        "0",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "invalid UUID",
		},
		{
			description:  "Wrong network number",
			setupSdnFunc: setupSdn,
			setupFeedManagerFunc: func(g *gateway) *servers.FeedManager {
				return servers.NewFeedManager(g.context, g, g.feedManagerChan, nil, services.NewNoOpSubscriptionServices(),
					36, types.NetworkID(137), g.sdn.NodeModel().NodeID,
					g.wsManager, g.sdn.AccountModel(), nil,
					"", "", *g.BxConfig, g.stats, nil, nil)
			},
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: generateDynamicFeeTxAndHash,
			expectedErrSubStr: "invalid network",
		},
		{
			description:          "Wrong tx format",
			setupSdnFunc:         setupSdn,
			setupFeedManagerFunc: setupFeedManager,
			request: &pb.BlxrSubmitBundleRequest{
				BlockNumber: "0x1f71710",
			},
			generateTxAndHash: func(privKey *ecdsa.PrivateKey, chain *big.Int) ([]string, []string) {
				tx1, ethTxBytes1 := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, privKey, chain)
				return []string{hex.EncodeToString(ethTxBytes1), "f800000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000000"},
					[]string{tx1.Hash().String(), "0x00"}
			},
			expectedErrSubStr: "unable to parse bundle",
		},
	}

	for _, tc := range testCases {
		t.Run(tc.description, func(t *testing.T) {
			privKey, _ := crypto.GenerateKey()
			port := test.NextTestPort()
			chain := big.NewInt(1)

			g, _, s, gg := spawnGRPCServer(t, port, "", "")
			defer s.Shutdown()
			gg.params.feedManager = tc.setupFeedManagerFunc(g)
			go gg.params.feedManager.Start(context.Background())
			g.sdn = tc.setupSdnFunc()
			gg.params.sdn = tc.setupSdnFunc()

			if tc.privKey != nil {
				privKey = tc.privKey
			}
			if tc.chainID != nil {
				chain = tc.chainID
			}

			txs, _ := tc.generateTxAndHash(privKey, chain)
			tc.request.Transactions = txs

			clientConfig := &config.GRPC{
				Enabled:        true,
				Host:           "127.0.0.1",
				Port:           port,
				AuthEnabled:    true,
				EncodedAuthSet: true,
				EncodedAuth:    testGatewayUserAuthHeader,
				Timeout:        1 * time.Second,
			}

			_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
				_, err := client.BlxrSubmitBundle(ctx, tc.request)
				if tc.expectedErrSubStr != "" {
					require.True(t, strings.Contains(err.Error(), tc.expectedErrSubStr))
				} else {
					require.Nil(t, err)
				}
				return nil, err
			})
		})
	}
}
