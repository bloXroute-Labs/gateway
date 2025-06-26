package grpc

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/ethereum/go-ethereum/common"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	bdn2 "github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/config"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/rpc"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

func TestNewBlocks(t *testing.T) {
	port := test.NextTestPort()
	ctx, cancel := context.WithCancel(context.Background())
	testServer, feedMngr := testGRPCServer(t, port, "", "")
	wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, feedMngr)
	defer func() {
		testServer.Shutdown()
		cancel()
		require.NoError(t, wait())
	}()

	bridge := testServer.gatewayServer.(*server).params.bridge
	clientConfig := config.NewGRPC("127.0.0.1", port, testGatewayAccountID, testGatewaySecretHash)
	client, err := rpc.GatewayClient(clientConfig)
	require.NoError(t, err)
	newBlocksStream, err := client.NewBlocks(ctx, &pb.BlocksRequest{})
	require.NoError(t, err)

	// wait for the subscription to be created
	for {
		var subs *pb.SubscriptionsReply
		subs, err = client.Subscriptions(ctx, &pb.SubscriptionsRequest{})
		require.NoError(t, err)
		if len(subs.Subscriptions) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := bridge.BlockBlockchainToBDN(bdn2.NewBlockInfo(ethBlock, nil))
	require.NoError(t, err)
	block, err := bridge.BlockBDNtoBlockchain(bxBlock)
	require.NoError(t, err)
	blockNotification, err := types.NewEthBlockNotification(common.Hash(bxBlock.ExecutionHash()), block.(*bdn2.BlockInfo).Block, nil, false)
	require.NoError(t, err)
	blockNotification.SetNotificationType(types.NewBlocksFeed)

	// notify the feed manager about the new transaction
	testServer.gatewayServer.(*server).params.feedManager.Notify(blockNotification)

	newBlocksNotification, err := newBlocksStream.Recv()
	assert.NoError(t, err)
	assert.NotNil(t, newBlocksNotification.Header)
	assert.Equal(t, ethBlock.Hash().String(), newBlocksNotification.Hash)
	assert.Equal(t, 5, len(ethBlock.Transactions()))
}

func TestBdnBlocks(t *testing.T) {
	port := test.NextTestPort()
	ctx, cancel := context.WithCancel(context.Background())
	testServer, feedMngr := testGRPCServer(t, port, "", "")
	wait := start(ctx, t, fmt.Sprintf("0.0.0.0:%v", port), testServer, feedMngr)
	defer func() {
		testServer.Shutdown()
		cancel()
		require.NoError(t, wait())
	}()

	bridge := testServer.gatewayServer.(*server).params.bridge
	clientConfig := config.NewGRPC("127.0.0.1", port, testGatewayAccountID, testGatewaySecretHash)
	client, err := rpc.GatewayClient(clientConfig)
	require.NoError(t, err)
	bdnBlocksStream, err := client.BdnBlocks(ctx, &pb.BlocksRequest{})
	require.NoError(t, err)

	// wait for the subscription to be created
	for {
		var subs *pb.SubscriptionsReply
		subs, err = client.Subscriptions(ctx, &pb.SubscriptionsRequest{})
		require.NoError(t, err)
		if len(subs.Subscriptions) > 0 {
			break
		}
		time.Sleep(10 * time.Millisecond)
	}

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := bridge.BlockBlockchainToBDN(bdn2.NewBlockInfo(ethBlock, nil))
	require.NoError(t, err)
	block, err := bridge.BlockBDNtoBlockchain(bxBlock)
	require.NoError(t, err)
	blockNotification, err := types.NewEthBlockNotification(common.Hash(bxBlock.ExecutionHash()), block.(*bdn2.BlockInfo).Block, nil, false)
	require.NoError(t, err)
	blockNotification.SetNotificationType(types.BDNBlocksFeed)

	// notify the feed manager about the new transaction
	testServer.gatewayServer.(*server).params.feedManager.Notify(blockNotification)

	bdnBlocksNotification, err := bdnBlocksStream.Recv()
	require.NoError(t, err)

	require.Nil(t, err)
	require.NotNil(t, bdnBlocksNotification.Header)
	require.Equal(t, ethBlock.Hash().String(), bdnBlocksNotification.Hash)
	require.Equal(t, 5, len(ethBlock.Transactions()))
}
