package nodes

import (
	"context"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/version"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func spawnGRPCServer(t *testing.T, port int, user string, password string) (*gateway, blockchain.Bridge, *gatewayGRPCServer) {
	serverConfig := config.NewGRPC("0.0.0.0", port, user, password)
	bridge, g := setup(t, 1)
	g.BxConfig.GRPC = serverConfig
	s := newGatewayGRPCServer(g, serverConfig.Host, serverConfig.Port, serverConfig.User, serverConfig.Password)
	go func() {
		_ = s.Start()
	}()

	// small sleep for goroutine to start
	time.Sleep(1 * time.Millisecond)

	return g, bridge, &s
}

func TestGatewayGRPCServerNoAuth(t *testing.T) {
	port := test.NextTestPort()

	_, _, s := spawnGRPCServer(t, port, "", "")
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

	_, _, s := spawnGRPCServer(t, port, "user", "password")
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

	_ = g.feedManager.Start()

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.NewTxs(ctx, &pb.TxsRequest{})
		assert.Nil(t, err)

		newTxsStream, ok := res.(pb.Gateway_NewTxsClient)
		assert.True(t, ok)

		_, relayConn1 := addRelayConn(g)
		_, deliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0)

		err = g.HandleMsg(deliveredTxMessage, relayConn1, connections.RunForeground)
		assert.Nil(t, err)

		txNotification, err := newTxsStream.Recv()
		assert.Nil(t, err)
		assert.NotNil(t, txNotification.Tx)
		assert.Equal(t, 1, len(txNotification.Tx))

		return txNotification, err
	})
}

func TestGatewayGRPCPendingTxs(t *testing.T) {
	port := test.NextTestPort()

	g, bridge, s := spawnGRPCServer(t, port, "", "")
	defer s.Stop()
	g.BxConfig.WebsocketEnabled = true

	go func() {
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	_ = g.feedManager.Start()

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.PendingTxs(ctx, &pb.TxsRequest{})
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
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	_ = g.feedManager.Start()

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")

	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.NewBlocks(ctx, &pb.BlocksRequest{})
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
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	_ = g.feedManager.Start()

	clientConfig := config.NewGRPC("127.0.0.1", port, "", "")
	_ = rpc.GatewayConsoleCall(clientConfig, func(ctx context.Context, client pb.GatewayClient) (interface{}, error) {
		res, err := client.BdnBlocks(ctx, &pb.BlocksRequest{})
		assert.Nil(t, err)

		bdnBlocksStream, ok := res.(pb.Gateway_BdnBlocksClient)
		assert.True(t, ok)

		ethBlock := bxmock.NewEthBlock(10, common.Hash{})
		bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))

		// compress a transaction
		bxTransaction, _ := bridge.TransactionBlockchainToBDN(ethBlock.Transactions()[0])
		txStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)
		g.TxStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)

		broadcastMessage, _, err := bp.BxBlockToBroadcast(bxBlock, networkNum, g.sdn.MinTxAge())
		assert.Nil(t, err)

		err = g.HandleMsg(broadcastMessage, relayConn1, connections.RunForeground)

		bdnBlocksNotification, err := bdnBlocksStream.Recv()

		assert.Nil(t, err)
		assert.NotNil(t, bdnBlocksNotification.Header)
		assert.Equal(t, ethBlock.Hash().String(), bdnBlocksNotification.Hash)
		assert.NotNil(t, bdnBlocksNotification.SubscriptionID)
		assert.Equal(t, 3, len(ethBlock.Transactions()))

		return bdnBlocksNotification, err
	})
}
