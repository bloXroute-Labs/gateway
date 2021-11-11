package nodes

import (
	"context"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/blockchain/network"
	"github.com/bloXroute-Labs/gateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/config"
	"github.com/bloXroute-Labs/gateway/connections"
	"github.com/bloXroute-Labs/gateway/connections/handler"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/services"
	"github.com/bloXroute-Labs/gateway/services/loggers"
	"github.com/bloXroute-Labs/gateway/test/bxmock"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/sirupsen/logrus"
	"github.com/stretchr/testify/assert"
	"math/big"
	"sync"
	"testing"
	"time"
)

var (
	networkNum           types.NetworkNum = 5
	blockchainIPEndpoint                  = types.NodeEndpoint{IP: "127.0.0.1", Port: 8001}
)

func setup() (blockchain.Bridge, *gateway) {
	sslCerts := bxmock.TestCerts()
	nm := sdnmessage.NodeModel{
		NodeType:             "EXTERNAL_GATEWAY",
		BlockchainNetworkNum: networkNum,
	}

	sdn := connections.NewSDNHTTP(&sslCerts, "", nm)
	sdn.Networks = sdnmessage.BlockchainNetworks{5: bxmock.MockNetwork(networkNum, "Ethereum", "Mainnet", 0)}

	logConfig := config.Log{
		AppName:      "gateway-test",
		FileName:     "test-logfile",
		FileLevel:    logrus.TraceLevel,
		ConsoleLevel: logrus.TraceLevel,
		MaxSize:      100,
		MaxBackups:   2,
		MaxAge:       1,
		TxTrace: config.TxTraceLog{
			Enabled:      true,
			MaxFileSize:  100,
			MaxFileCount: 3,
		},
	}
	bxConfig := &config.Bx{
		Log: &logConfig,
	}

	bridge := blockchain.NewBxBridge(eth.Converter{})
	var blockchainPeers []types.NodeEndpoint
	blockchainPeers = append(blockchainPeers, types.NodeEndpoint{IP: "123.45.6.78", Port: 123})
	node, _ := NewGateway(context.Background(), bxConfig, bridge, blockchainPeers)

	g := node.(*gateway)
	g.sdn = sdn
	g.bdnStats = bxmessage.NewBDNStats()
	g.txTrace = loggers.NewTxTrace(nil)
	g.setSyncWithRelay()
	return bridge, g
}

func newBP() (*services.BxTxStore, services.BlockProcessor) {
	txStore := services.NewBxTxStore(time.Minute, time.Minute, time.Minute, services.NewEmptyShortIDAssigner(), services.NewHashHistory("seenTxs", time.Minute), nil)
	bp := services.NewRLPBlockProcessor(&txStore)
	return &txStore, bp
}

func addRelayConn(g *gateway) (bxmock.MockTLS, *handler.Relay) {
	mockTLS := bxmock.NewMockTLS("1.1.1.1", 1800, "", utils.Relay, "")
	relayConn := handler.NewRelay(g,
		func() (connections.Socket, error) {
			return &mockTLS, nil
		},
		&utils.SSLCerts{}, "1.1.1.1", 1800, "", utils.Relay, true, &g.sdn.Networks, true, true, connections.LocalInitiatedPort, utils.RealClock{})

	// set connection as established and ready for broadcast
	_ = relayConn.Connect()
	hello := bxmessage.Hello{
		NodeID:   "1234",
		Protocol: relayConn.Protocol(),
	}
	b, _ := hello.Pack(relayConn.Protocol())
	relayConn.ProcessMessage(b)

	// advance ack message
	ackBytes, err := mockTLS.MockAdvanceSent()
	if err != nil {
		panic(err)
	}
	var ack bxmessage.Ack
	err = ack.Unpack(ackBytes, relayConn.Protocol())
	if err != nil {
		panic(err)
	}

	// advance sync req
	syncReqBytes, err := mockTLS.MockAdvanceSent()
	if err != nil {
		panic(err)
	}
	var syncReq bxmessage.SyncReq
	err = syncReq.Unpack(syncReqBytes, relayConn.Protocol())
	if err != nil {
		panic(err)
	}

	return mockTLS, relayConn
}

func bxBlockFromEth(b blockchain.Bridge, height uint64, parentHash types.SHA256Hash) *types.BxBlock {
	ethBlock := bxmock.NewEthBlock(height, common.BytesToHash(parentHash.Bytes()))
	blockInfo := &eth.BlockInfo{Block: ethBlock}
	blockInfo.SetTotalDifficulty(big.NewInt(int64(10 * height)))
	bxBlock, _ := b.BlockBlockchainToBDN(blockInfo)
	return bxBlock
}

func TestGateway_PushBlockchainConfig(t *testing.T) {
	networkID := int64(1)
	td := "40000"
	hash := "0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"

	bridge, g := setup()

	var blockchainNetwork sdnmessage.BlockchainNetwork
	blockchainNetwork.DefaultAttributes.NetworkID = networkID
	blockchainNetwork.DefaultAttributes.ChainDifficulty = td
	blockchainNetwork.DefaultAttributes.GenesisHash = hash

	g.sdn.Networks[5] = &blockchainNetwork

	var ethConfig network.EthConfig
	wg := sync.WaitGroup{}
	wg.Add(1)

	go func() {
		defer wg.Done()
		ethConfig = <-bridge.ReceiveNetworkConfigUpdates()
	}()

	err := g.pushBlockchainConfig()
	assert.Nil(t, err)

	wg.Wait()
	assert.NotNil(t, ethConfig)

	expectedTD, _ := new(big.Int).SetString("40000", 16)
	expectedHash := common.HexToHash(hash)

	assert.Equal(t, uint64(networkID), ethConfig.Network)
	assert.Equal(t, expectedTD, ethConfig.TotalDifficulty)
	assert.Equal(t, expectedHash, ethConfig.Genesis)
	assert.Equal(t, expectedHash, ethConfig.Head)
}

func TestGateway_HandleTransactionFromBlockchain(t *testing.T) {
	bridge, g := setup()
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil)
	bdnTx, err := bridge.TransactionBlockchainToBDN(ethTx)
	assert.Nil(t, err)

	// send transactions over bridge from blockchain connection
	txs := []*types.BxTransaction{bdnTx}
	err = bridge.SendTransactionsToBDN(txs, blockchainIPEndpoint)
	assert.Nil(t, err)

	msgBytes, err := mockTLS.MockAdvanceSent()
	assert.Nil(t, err)

	// transaction is broadcast to relays
	var sentTx bxmessage.Tx
	err = sentTx.Unpack(msgBytes, relayConn.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, ethTx.Hash().Bytes(), sentTx.Hash().Bytes())
	assert.Equal(t, ethTxBytes, sentTx.Content())
}

func TestGateway_HandleTransactionHashesFromBlockchain(t *testing.T) {
	bridge, g := setup()

	go func() {
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	peerID := "go-ethereum-1"
	hashes := []types.SHA256Hash{
		types.GenerateSHA256Hash(),
		types.GenerateSHA256Hash(),
		types.GenerateSHA256Hash(),
		types.GenerateSHA256Hash(),
	}

	// hash 0 should be skipped since no content available
	g.TxStore.Add(hashes[0], types.TxContent{}, 1, networkNum, false, 0, time.Now(), 0)
	g.TxStore.Add(hashes[1], types.TxContent{1, 2, 3}, types.ShortIDEmpty, networkNum, false, 0, time.Now(), 0)

	err := bridge.AnnounceTransactionHashes(peerID, hashes)
	assert.Nil(t, err)

	request := <-bridge.ReceiveTransactionHashesRequest()

	assert.Equal(t, len(hashes)-2, len(request.Hashes))
	assert.Equal(t, hashes[2], request.Hashes[0])
	assert.Equal(t, hashes[3], request.Hashes[1])

	assert.Equal(t, peerID, request.PeerID)
}

func TestGateway_HandleTransactionFromRPC(t *testing.T) {
	bridge, g := setup()
	mockTLS, relayConn := addRelayConn(g)

	ethTx, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum)
	txMessage.SetFlags(types.TFDeliverToNode)

	err := g.HandleMsg(txMessage, connections.NewRPCConn("", "", networkNum), connections.RunForeground)
	assert.Nil(t, err)

	// transaction should be broadcast both to blockchain node and BDN
	txsSentToBlockchain := <-bridge.ReceiveBDNTransactions()
	assert.Equal(t, 1, len(txsSentToBlockchain))
	txSentToBlockchain := txsSentToBlockchain[0]

	msgBytes, err := mockTLS.MockAdvanceSent()
	assert.Nil(t, err)

	var txSentToBDN bxmessage.Tx
	err = txSentToBDN.Unpack(msgBytes, relayConn.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, ethTx.Hash().Bytes(), txSentToBDN.Hash().Bytes())
	assert.Equal(t, txSentToBlockchain.Hash(), txSentToBDN.Hash())
}

func TestGateway_HandleTransactionFromRelay(t *testing.T) {
	bridge, g := setup()
	_, relayConn := addRelayConn(g)

	deliveredEthTx, deliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum)
	deliveredTxMessage.SetFlags(types.TFDeliverToNode)

	err := g.HandleMsg(deliveredTxMessage, relayConn, connections.RunForeground)
	assert.Nil(t, err)

	bdnTxs := <-bridge.ReceiveBDNTransactions()
	assert.Equal(t, 1, len(bdnTxs))

	bdnTx := bdnTxs[0]
	assert.Equal(t, deliveredEthTx.Hash().Bytes(), bdnTx.Hash().Bytes())

	_, undeliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum)

	err = g.HandleMsg(undeliveredTxMessage, relayConn, connections.RunForeground)
	assert.Nil(t, err)

	select {
	case <-bridge.ReceiveBDNTransactions():
		assert.Fail(t, "unexpectedly received txs when TFDeliverToNode not set")
	default:
	}
}

func TestGateway_HandleTransactionFromRelayBlocksOnly(t *testing.T) {
	bridge, g := setup()
	_, relayConn := addRelayConn(g)
	g.BxConfig.BlocksOnly = true

	_, freeTx := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum)
	freeTx.SetFlags(types.TFDeliverToNode)

	_, paidTx := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 2, nil, networkNum)
	paidTx.SetFlags(types.TFPaidTx | types.TFDeliverToNode)

	err := g.HandleMsg(paidTx, relayConn, connections.RunForeground)
	assert.Nil(t, err)

	select {
	case <-bridge.ReceiveBDNTransactions():
		assert.Fail(t, "unexpectedly received txs when --blocks-only set")
	default:
	}
}

func TestGateway_HandleBlockFromBlockchain(t *testing.T) {
	bridge, g := setup()
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	assert.Nil(t, err)

	// send block over bridge from blockchain connection
	err = bridge.SendBlockToBDN(bxBlock, blockchainIPEndpoint)
	assert.Nil(t, err)

	msgBytes, err := mockTLS.MockAdvanceSent()
	assert.Nil(t, err)

	// block is broadcast to relays
	var sentBroadcast bxmessage.Broadcast
	err = sentBroadcast.Unpack(msgBytes, relayConn.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, ethBlock.Hash().Bytes(), sentBroadcast.BlockHash().Bytes())
	assert.Equal(t, networkNum, sentBroadcast.GetNetworkNum())
	assert.Equal(t, 0, len(sentBroadcast.ShortIDs()))

	// try sending block again
	err = bridge.SendBlockToBDN(bxBlock, blockchainIPEndpoint)
	assert.Nil(t, err)

	// times out, nothing sent (already processed)
	msgBytes, err = mockTLS.MockAdvanceSent()
	assert.NotNil(t, err, "unexpected bytes %v", string(msgBytes))
}

func TestGateway_HandleBlockFromRelay(t *testing.T) {
	bridge, g := setup()
	_, relayConn := addRelayConn(g)

	// separate service instance, to avoid already processed errors
	txStore, bp := newBP()

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))

	// compress a transaction
	bxTransaction, _ := bridge.TransactionBlockchainToBDN(ethBlock.Transactions()[0])
	txStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0)
	g.TxStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0)

	broadcastMessage, _, _ := bp.BxBlockToBroadcast(bxBlock, networkNum)

	err := g.HandleMsg(broadcastMessage, relayConn, connections.RunForeground)
	assert.Nil(t, err)

	receivedBxBlock := <-bridge.ReceiveBlockFromBDN()
	assert.Equal(t, bxBlock.Hash(), receivedBxBlock.Hash())
	assert.Equal(t, bxBlock.Header, receivedBxBlock.Header)
	assert.Equal(t, bxBlock.Trailer, receivedBxBlock.Trailer)
	assert.True(t, bxBlock.Equals(receivedBxBlock))

	// duplicate, no processing
	err = g.HandleMsg(broadcastMessage, relayConn, connections.RunForeground)
	assert.Nil(t, err)

	select {
	case <-bridge.ReceiveBlockFromBDN():
		assert.Fail(t, "unexpectedly processed block again")
	default:
	}
}
