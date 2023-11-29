package nodes

import (
	"context"
	"fmt"
	"math"
	"math/big"
	"os"
	"sync"
	"sync/atomic"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	ethtest "github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/connections/handler"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	pb "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/servers"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/loggers"
	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	mock_connections "github.com/bloXroute-Labs/gateway/v2/test/sdnhttpmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/utilmock"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/golang/mock/gomock"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"google.golang.org/grpc/metadata"
)

var (
	networkNum           types.NetworkNum = 5
	chainID              int64            = 1
	blockchainIPEndpoint                  = types.NodeEndpoint{IP: "123.45.6.78", Port: 8001}
)

func setup(t *testing.T, numPeers int) (blockchain.Bridge, *gateway) {
	nm := sdnmessage.NodeModel{
		NodeType:             "EXTERNAL_GATEWAY",
		BlockchainNetworkNum: networkNum,
		ExternalIP:           "172.0.0.1",
	}
	utils.IPResolverHolder = &utilmock.MockIPResolver{IP: "11.111.111.111"}
	ctl := gomock.NewController(t)
	sdn := mock_connections.NewMockSDNHTTP(ctl)
	sdn.EXPECT().FetchAllBlockchainNetworks().Return(nil).AnyTimes()
	sdn.EXPECT().FetchCustomerAccountModel(gomock.Any()).Return(sdnmessage.Account{}, nil).AnyTimes()
	sdn.EXPECT().MinTxAge().Return(time.Millisecond).AnyTimes()
	sdn.EXPECT().NetworkNum().Return(networkNum).AnyTimes()
	sdn.EXPECT().NodeModel().Return(&nm).AnyTimes()
	sdn.EXPECT().AccountModel().Return(sdnmessage.Account{}).AnyTimes()
	networks := sdnmessage.BlockchainNetworks{5: bxmock.MockNetwork(networkNum, "Ethereum", "Mainnet", 0)}
	sdn.EXPECT().Networks().Return(&networks).AnyTimes()
	sdn.EXPECT().FindNetwork(gomock.Any()).DoAndReturn(func(num types.NetworkNum) (*sdnmessage.BlockchainNetwork, error) {
		return (networks)[5], nil
	}).AnyTimes()

	logConfig := log.Config{
		AppName:      "gateway-test",
		FileName:     "test-logfile",
		FileLevel:    log.TraceLevel,
		ConsoleLevel: log.TraceLevel,
		MaxSize:      100,
		MaxBackups:   2,
		MaxAge:       1,
	}
	txTraceLog := config.TxTraceLog{
		Enabled:        true,
		MaxFileSize:    100,
		MaxBackupFiles: 3,
	}

	bxConfig := &config.Bx{
		Config:     &logConfig,
		NodeType:   utils.Gateway,
		TxTraceLog: &txTraceLog,
		GRPC:       &config.GRPC{Enabled: true},
	}

	bridge := blockchain.NewBxBridge(eth.Converter{}, true)
	blockchainPeers, blockchainPeersInfo := ethtest.GenerateBlockchainPeersInfo(numPeers)
	node, _ := NewGateway(
		context.Background(),
		bxConfig,
		bridge,
		eth.NewEthWSManager(blockchainPeersInfo,
			eth.NewMockWSProvider,
			bxgateway.WSProviderTimeout,
			false),
		blockchainPeers,
		blockchainPeersInfo,
		make(map[string]struct{}),
		"",
		sdn,
		nil,
		0,
		"",
		0,
		0,
		false,
		0,
		0,
		false,
	)

	g := node.(*gateway)

	// Required for TxReceipts feed
	g.wsManager.UpdateNodeSyncStatus(blockchainPeers[0], blockchain.Synced)

	g.setupTxStore()
	g.txTrace = loggers.NewTxTrace(nil)
	g.setSyncWithRelay()
	g.feedManagerChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)

	g.feedManager = servers.NewFeedManager(g.context, g, g.feedManagerChan, services.NewNoOpSubscriptionServices(),
		networkNum, types.NetworkID(chainID), g.sdn.NodeModel().NodeID,
		g.wsManager, g.sdn.AccountModel(), nil,
		"", "", *g.BxConfig, g.stats, nil, nil)
	return bridge, g
}

func newBP() (*services.BxTxStore, services.BlockProcessor) {
	txStore := services.NewBxTxStore(time.Minute, time.Minute, time.Minute, services.NewEmptyShortIDAssigner(), services.NewHashHistory("seenTxs", time.Minute), nil, 30*time.Minute, services.NoOpBloomFilter{})
	bp := services.NewBlockProcessor(&txStore)
	return &txStore, bp
}

func addRelayConn(g *gateway) (bxmock.MockTLS, *handler.Relay) {
	mockTLS := bxmock.NewMockTLS("1.1.1.1", 1800, "", utils.Relay, "")
	relayConn := handler.NewRelay(g,
		func() (connections.Socket, error) {
			return &mockTLS, nil
		},
		&utils.SSLCerts{}, "1.1.1.1", 1800, "", utils.Relay, true, g.sdn.Networks(), true, true, connections.LocalInitiatedPort, utils.RealClock{},
		false, true)

	// set connection as established and ready for broadcast
	_ = relayConn.Connect()
	hello := bxmessage.Hello{
		NodeID:   "1234",
		Protocol: relayConn.Protocol(),
	}
	b, _ := hello.Pack(relayConn.Protocol())
	msg := bxmessage.NewMessageBytes(b, time.Now())
	relayConn.ProcessMessage(msg)

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

func processEthTxOnBridge(t *testing.T, bridge blockchain.Bridge, ethTx *ethtypes.Transaction, blockchainPeer types.NodeEndpoint) {
	bdnTx, err := bridge.TransactionBlockchainToBDN(ethTx)
	assert.Nil(t, err)

	// send transactions over bridge from blockchain connection
	txs := []*types.BxTransaction{bdnTx}
	err = bridge.SendTransactionsToBDN(txs, blockchainPeer)
	assert.Nil(t, err)
}

func assertTransactionSentToRelay(t *testing.T, ethTx *ethtypes.Transaction, ethTxBytes []byte, relayTLS bxmock.MockTLS, relayConn *handler.Relay) {
	msgBytes, err := relayTLS.MockAdvanceSent()
	if err != nil {
		assert.FailNow(t, "no messages sent on relay connection")
	}

	// transaction is broadcast to relays
	var sentTx bxmessage.Tx
	err = sentTx.Unpack(msgBytes, relayConn.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, ethTx.Hash().Bytes(), sentTx.Hash().Bytes())
	if ethTx.Type() == ethtypes.LegacyTxType {
		assert.Equal(t, ethTxBytes, sentTx.Content())
	} else {
		// first two bytes for tx type thus gets dropped
		assert.Equal(t, ethTxBytes, sentTx.Content()[2:])
	}
}

func assertNoTransactionSentToRelay(t *testing.T, relayTLS bxmock.MockTLS) {
	_, err := relayTLS.MockAdvanceSent()
	assert.NotNil(t, err)
}

func assertNoBlockSentToRelay(t *testing.T, relayTLS bxmock.MockTLS) {
	_, err := relayTLS.MockAdvanceSent()
	assert.NotNil(t, err)
}

func waitUntillTXQueueChannelIsEmpty(g *gateway) {
	for {
		if g.txsQueue.Len() == 0 {
			time.Sleep(10 * time.Millisecond)
			return
		}
		time.Sleep(5 * time.Millisecond)
	}
}

func TestGateway_PushBlockchainConfig(t *testing.T) {
	networkID := types.NetworkID(1)
	td := "40000"
	// ttd := "500000000000000"
	hash := "0xd4e56740f876aef8c010b86a40d5f56745a118d0906a34e69aec8c0db1cb8fa3"

	bridge, g := setup(t, 1)

	var blockchainNetwork sdnmessage.BlockchainNetwork
	blockchainNetwork.DefaultAttributes.NetworkID = networkID
	blockchainNetwork.DefaultAttributes.ChainDifficulty = td
	// blockchainNetwork.DefaultAttributes.TerminalTotalDifficulty = ttd
	blockchainNetwork.DefaultAttributes.GenesisHash = hash

	(*g.sdn.Networks())[5] = &blockchainNetwork

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
	expectedTTD := big.NewInt(math.MaxInt)
	expectedHash := common.HexToHash(hash)

	assert.Equal(t, uint64(networkID), ethConfig.Network)
	assert.Equal(t, expectedTD, ethConfig.TotalDifficulty)
	assert.Equal(t, expectedTTD, ethConfig.TerminalTotalDifficulty)
	assert.Equal(t, expectedHash, ethConfig.Genesis)
	assert.Equal(t, expectedHash, ethConfig.Head)
}

func TestGateway_HandleTransactionFromBlockchain_SeenInBloomFilter(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	bf, err := services.NewBloomFilter(context.Background(), utils.RealClock{}, time.Hour, "", 1e6)
	defer func() { _ = os.Remove("bloom") }()
	require.NoError(t, err)

	g.TxStore = services.NewEthTxStore(g.clock, 30*time.Minute, 72*time.Hour, 10*time.Minute,
		services.NewEmptyShortIDAssigner(), services.NewHashHistory("seenTxs", 30*time.Minute), nil,
		*g.sdn.Networks(), bf)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil, nil)
	txHash := ethTx.Hash().String()
	txHash = txHash[2:]
	assert.Equal(t, false, g.pendingTxs.Exists(txHash))
	processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])
	assertTransactionSentToRelay(t, ethTx, ethTxBytes, mockTLS, relayConn)
	assert.Equal(t, true, g.pendingTxs.Exists(txHash))

	// Bloom filter is adding asynchronously, so wait a bit
	time.Sleep(time.Microsecond)

	// create empty TxStore to make sure transaction is ignored due to bloom_filter
	g.TxStore = services.NewEthTxStore(g.clock, 30*time.Minute, 72*time.Hour, 10*time.Minute,
		services.NewEmptyShortIDAssigner(), services.NewHashHistory("seenTxs", 30*time.Minute), nil,
		*g.sdn.Networks(), bf)

	processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])
	assertNoTransactionSentToRelay(t, mockTLS)

	select {
	case <-bridge.ReceiveBDNTransactions():
		assert.Fail(t, "unexpectedly received txs when tx was from only blockchain peer")
	default:
	}
}

func TestGateway_HandleTransactionFromBlockchain(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil, nil)
	txHash := ethTx.Hash().String()
	txHash = txHash[2:]
	assert.Equal(t, false, g.pendingTxs.Exists(txHash))
	processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])
	assertTransactionSentToRelay(t, ethTx, ethTxBytes, mockTLS, relayConn)
	assert.Equal(t, true, g.pendingTxs.Exists(txHash))

	select {
	case <-bridge.ReceiveBDNTransactions():
		assert.Fail(t, "unexpectedly received txs when tx was from only blockchain peer")
	default:
	}
}

func TestGateway_HandleTransactionFromInboundBlockchain(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil, nil)
	txHash := ethTx.Hash().String()
	txHash = txHash[2:]
	g.blockchainPeers[0].Dynamic = true
	assert.Equal(t, false, g.pendingTxs.Exists(txHash))
	processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])
	assertTransactionSentToRelay(t, ethTx, ethTxBytes, mockTLS, relayConn)
	assert.Equal(t, false, g.pendingTxs.Exists(txHash))

	select {
	case <-bridge.ReceiveBDNTransactions():
		assert.Fail(t, "unexpectedly received txs when tx was from only blockchain peer")
	default:
	}
}

func TestGateway_HandleTransactionFromBlockchain_MultiNode(t *testing.T) {
	bridge, g := setup(t, 2)
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil, nil)
	processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])
	assertTransactionSentToRelay(t, ethTx, ethTxBytes, mockTLS, relayConn)

	bdnTxs := <-bridge.ReceiveBDNTransactions()
	assert.Equal(t, 1, len(bdnTxs.Transactions))
	assert.Equal(t, g.blockchainPeers[0], bdnTxs.PeerEndpoint)

	bdnTx := bdnTxs.Transactions[0]
	assert.Equal(t, ethTx.Hash().Bytes(), bdnTx.Hash().Bytes())
}

func TestGateway_HandleTransactionFromBlockchain_TwoRelays(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS1, relayConn1 := addRelayConn(g)
	mockTLS2, relayConn2 := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil, nil)
	processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])

	assertTransactionSentToRelay(t, ethTx, ethTxBytes, mockTLS1, relayConn1)
	assertTransactionSentToRelay(t, ethTx, ethTxBytes, mockTLS2, relayConn2)
}

func TestGateway_HandleTransactionFromBlockchain_BurstLimit(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)
	test.ConfigureLogger(log.TraceLevel)

	mockClock := &utils.MockClock{}
	g.accountID = "foobar"
	g.burstLimiter = services.NewAccountBurstLimiter(mockClock)
	limit := sdnmessage.BDNServiceLimit(50)

	account := mockAccountBurstRateLimit(g, limit)
	g.burstLimiter.Register(account)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	for i := uint64(1); i <= uint64(limit); i++ {
		ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, i, nil, nil)
		processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])
		assertTransactionSentToRelay(t, ethTx, ethTxBytes, mockTLS, relayConn)
	}

	ethTx, _ := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, uint64(limit+1), nil, nil)
	processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])
	assertNoTransactionSentToRelay(t, mockTLS)

	countLimitUnpaid := g.bdnStats.BurstLimitedTransactionsUnpaid()
	assert.Equal(t, countLimitUnpaid, uint16(1))
	countLimitPaid := g.bdnStats.BurstLimitedTransactionsPaid()
	assert.Equal(t, countLimitPaid, uint16(0))
}

func TestGateway_HandleTransactionFromRPC_BurstLimitPaid(t *testing.T) {
	_, g := setup(t, 1)
	test.ConfigureLogger(log.TraceLevel)

	mockClock := &utils.MockClock{}
	g.accountID = "foobar"
	g.burstLimiter = services.NewAccountBurstLimiter(mockClock)
	limit := sdnmessage.BDNServiceLimit(50)

	account := mockAccountBurstRateLimit(g, limit)
	g.burstLimiter.Register(account)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	for i := uint64(1); i <= uint64(limit); i++ {
		_, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, i, nil, networkNum, 0, nil)
		txMessage.SetFlags(types.TFPaidTx)
		err := g.HandleMsg(txMessage, connections.NewRPCConn(account.AccountID, "", networkNum, utils.Websocket), connections.RunForeground)
		assert.Nil(t, err)
	}

	_, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, uint64(limit+1), nil, networkNum, 0, nil)
	txMessage.SetFlags(types.TFPaidTx)
	err := g.HandleMsg(txMessage, connections.NewRPCConn(account.AccountID, "", networkNum, utils.Websocket), connections.RunForeground)
	assert.Nil(t, err)
	waitUntillTXQueueChannelIsEmpty(g)
	countLimitPaid := g.bdnStats.BurstLimitedTransactionsPaid()
	assert.Equal(t, countLimitPaid, uint16(1))
}

func TestGateway_HandleTransactionFromRPC_BurstLimitUnpaid(t *testing.T) {
	_, g := setup(t, 1)
	test.ConfigureLogger(log.TraceLevel)

	mockClock := &utils.MockClock{}
	g.accountID = "foobar"
	g.burstLimiter = services.NewAccountBurstLimiter(mockClock)
	limit := sdnmessage.BDNServiceLimit(50)

	account := mockAccountBurstRateLimit(g, limit)
	g.burstLimiter.Register(account)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	for i := uint64(1); i <= uint64(limit); i++ {
		_, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, i, nil, networkNum, 0, nil)
		txMessage.SetFlags(types.TFDeliverToNode)
		err := g.HandleMsg(txMessage, connections.NewRPCConn(account.AccountID, "", networkNum, utils.Websocket), connections.RunForeground)
		assert.Nil(t, err)
	}

	for i := uint64(1); i <= uint64(5); i++ {
		_, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, uint64(limit)+i, nil, networkNum, 0, nil)
		txMessage.SetFlags(types.TFDeliverToNode)
		err := g.HandleMsg(txMessage, connections.NewRPCConn(account.AccountID, "", networkNum, utils.Websocket), connections.RunForeground)
		assert.Nil(t, err)
	}
	waitUntillTXQueueChannelIsEmpty(g)
	countLimitUnpaid := g.bdnStats.BurstLimitedTransactionsUnpaid()
	assert.Equal(t, countLimitUnpaid, uint16(5))
}

func mockAccountBurstRateLimit(g *gateway, limit sdnmessage.BDNServiceLimit) *sdnmessage.Account {
	account := &sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: g.accountID,
		},
		UnpaidTransactionBurstLimit: sdnmessage.BDNQuotaService{
			ExpireDateTime: time.Now().Add(12 * time.Hour),
			MsgQuota: sdnmessage.BDNService{
				Limit:             limit,
				BehaviorLimitFail: sdnmessage.BehaviorBlock,
			},
		},
		PaidTransactionBurstLimit: sdnmessage.BDNQuotaService{
			ExpireDateTime: time.Now().Add(12 * time.Hour),
			MsgQuota: sdnmessage.BDNService{
				Limit:             limit,
				BehaviorLimitFail: sdnmessage.BehaviorBlock,
			},
		},
	}
	return account
}

func TestGateway_HandleBlockConfirmationFromBackend(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, _ := addRelayConn(g)
	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	g.BxConfig.SendConfirmation = true

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	assert.Nil(t, err)

	bridge.SendConfirmedBlockToGateway(bxBlock, types.NodeEndpoint{IP: "1.1.1.1", Port: 1800})

	_, err = mockTLS.MockAdvanceSent()
	assert.Nil(t, err)
}

func TestGateway_HandleTransactionHashesFromBlockchain(t *testing.T) {
	bridge, g := setup(t, 1)

	go func() {
		err := g.handleBridgeMessages(context.Background())
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
	g.TxStore.Add(hashes[0], types.TxContent{}, 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)
	g.TxStore.Add(hashes[1], types.TxContent{1, 2, 3}, types.ShortIDEmpty, networkNum, false, 0, time.Now(), 0, types.EmptySender)

	err := bridge.AnnounceTransactionHashes(peerID, hashes, types.NodeEndpoint{})
	assert.Nil(t, err)

	request := <-bridge.ReceiveTransactionHashesRequest()

	assert.Equal(t, len(hashes)-2, len(request.Hashes))
	assert.Equal(t, hashes[2], request.Hashes[0])
	assert.Equal(t, hashes[3], request.Hashes[1])

	assert.Equal(t, peerID, request.PeerID)
}

func Test_HandleBlxrSubmitBundleFromRPC(t *testing.T) {
	_, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	// construct bundlemessage
	bundleMessage := bxmessage.MEVBundle{
		ID:         "1",
		JSONRPC:    "2.0",
		Method:     "eth_sendBundle",
		UUID:       "123e4567-e89b-12d3-a456-426614174000",
		BundleHash: "0x5ac23d9a014dfbfc3ec5d06435f74c3dbb616a5a7d5dc77b152b1db2c524083d",
		Transactions: []string{
			"0xf85d808080945ac6ba4e9b9a4bb23be58af43f15351f70b71769808025a05a35c20b14e4bae033357c7ff5772dbb84a831b290e98ff26fb4073c7483afdba0492ac5720a1c153ca1a35a6214b9811fd04c7ba434c2d0cdf93f8d23080458cb",
			"0xf85d8080809419503078a85ceb93e3d7b12721bedcdfc0978ddc808026a05f3cff439aa83fcbde58c2011b5577e98148e7408a170d08acad4e3eb1d64e39a07e4295280fbfff13fbd2e6605bd20b7de0816c25aab8106fdd1ee280cc96a027",
			"0xf85d8080809415817f1d896737cbdf4b3dc1cc175be63a9d347f808025a07e809211aff74a5c0af15ee389f53dd0cab085d373f6cef7897948c8518c574fa0024a201974175b06e28bf4ba63709188a30961417394576b5b2ba008dead802f",
		},
		BlockNumber:  "0x8",
		MinTimestamp: 1633036800,
		MaxTimestamp: 1633123200,
		RevertingHashes: []string{
			"0xa74b582bd1505262d2b9154d87b181600f5a6fe7b49bd7f0b0192407773f0143",
			"0x2d283f3b4bfc4a7fe37ec935566b92e9255d1413a2d0c814125e43750d952ecd",
		},
		Frontrunning: true,
		MEVBuilders: map[string]string{
			"builder1": "0x123",
			"builder2": "0x456",
		},
	}

	err := g.HandleMsg(&bundleMessage, connections.NewRPCConn("", "", networkNum, utils.Websocket), connections.RunForeground)
	assert.Nil(t, err)

	msgBytes, err := mockTLS.MockAdvanceSent()
	assert.Nil(t, err)

	var bundleSentToBDN bxmessage.MEVBundle
	err = bundleSentToBDN.Unpack(msgBytes, relayConn.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, bundleSentToBDN.BundleHash, bundleMessage.BundleHash)
	assert.Equal(t, bundleSentToBDN.UUID, bundleMessage.UUID)
	assert.Equal(t, bundleSentToBDN.MinTimestamp, bundleMessage.MinTimestamp)
	assert.Equal(t, bundleSentToBDN.MaxTimestamp, bundleMessage.MaxTimestamp)
	assert.Equal(t, bundleSentToBDN.BlockNumber, bundleMessage.BlockNumber)
	assert.Equal(t, bundleSentToBDN.Frontrunning, bundleMessage.Frontrunning)
	assert.Equal(t, bundleSentToBDN.Hash(), bundleMessage.Hash())
	assert.True(t, test.MapsEqual(bundleSentToBDN.MEVBuilders, bundleMessage.MEVBuilders))
	assert.Equal(t, bundleSentToBDN.Transactions[0], bundleMessage.Transactions[0])
	assert.Equal(t, bundleSentToBDN.Transactions[1], bundleMessage.Transactions[1])
	assert.Equal(t, bundleSentToBDN.Transactions[2], bundleMessage.Transactions[2])
}

func TestGateway_HandleTransactionFromRPC(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	ethTx, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0, nil)
	txMessage.SetFlags(types.TFDeliverToNode)

	err := g.HandleMsg(txMessage, connections.NewRPCConn("", "", networkNum, utils.Websocket), connections.RunForeground)
	assert.Nil(t, err)

	// transaction should be broadcast both to blockchain node and BDN
	txsSentToBlockchain := <-bridge.ReceiveBDNTransactions()
	assert.Equal(t, 1, len(txsSentToBlockchain.Transactions))
	txSentToBlockchain := txsSentToBlockchain.Transactions[0]

	msgBytes, err := mockTLS.MockAdvanceSent()
	assert.Nil(t, err)

	var txSentToBDN bxmessage.Tx
	err = txSentToBDN.Unpack(msgBytes, relayConn.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, ethTx.Hash().Bytes(), txSentToBDN.Hash().Bytes())
	assert.Equal(t, txSentToBlockchain.Hash(), txSentToBDN.Hash())
}

func TestGateway_HandleTransactionFromRelay(t *testing.T) {
	bridge, g := setup(t, 1)
	_, relayConn1 := addRelayConn(g)
	mockTLS2, _ := addRelayConn(g)

	deliveredEthTx, deliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0, nil)
	deliveredTxMessage.SetFlags(types.TFDeliverToNode)

	err := g.HandleMsg(deliveredTxMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)
	assertNoTransactionSentToRelay(t, mockTLS2)

	bdnTxs := <-bridge.ReceiveBDNTransactions()
	assert.Equal(t, 1, len(bdnTxs.Transactions))
	assert.Equal(t, types.NodeEndpoint{IP: relayConn1.GetPeerIP(), Port: int(relayConn1.GetPeerPort())}, bdnTxs.PeerEndpoint)

	bdnTx := bdnTxs.Transactions[0]
	assert.Equal(t, deliveredEthTx.Hash().Bytes(), bdnTx.Hash().Bytes())

	_, undeliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0, nil)

	err = g.HandleMsg(undeliveredTxMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)

	select {
	case <-bridge.ReceiveBDNTransactions():
		assert.Fail(t, "unexpectedly received txs when TFDeliverToNode not set")
	default:
	}
}

func TestGateway_ReprocessTransactionFromRelay(t *testing.T) {
	bridge, g := setup(t, 1)
	_, relayConn1 := addRelayConn(g)
	mockTLS2, _ := addRelayConn(g)

	// send new tx
	ethTx, ethTxMsg := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0, nil)
	err := g.HandleMsg(ethTxMsg, relayConn1, connections.RunForeground)
	assert.Nil(t, err)
	assertNoTransactionSentToRelay(t, mockTLS2)

	hash, _ := types.NewSHA256HashFromString(ethTx.Hash().String())
	_, exists := g.TxStore.Get(hash)
	assert.True(t, exists)

	// reprocess resent tx
	resentTxMessage := ethTxMsg
	resentTxMessage.SetFlags(types.TFDeliverToNode)
	err = g.HandleMsg(resentTxMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)

	bdnTxs := <-bridge.ReceiveBDNTransactions()
	assert.Equal(t, 1, len(bdnTxs.Transactions))
	assert.Equal(t, types.NodeEndpoint{IP: relayConn1.GetPeerIP(), Port: int(relayConn1.GetPeerPort())}, bdnTxs.PeerEndpoint)

	bdnTx := bdnTxs.Transactions[0]
	assert.Equal(t, ethTx.Hash().Bytes(), bdnTx.Hash().Bytes())

	// only reprocess once
	err = g.HandleMsg(resentTxMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)
	select {
	case <-bridge.ReceiveBDNTransactions():
		assert.Fail(t, "unexpectedly reprocessed tx more than once")
	default:
	}
}

func TestGateway_HandleTransactionFromRelayBlocksOnly(t *testing.T) {
	bridge, g := setup(t, 1)
	_, relayConn := addRelayConn(g)
	g.BxConfig.BlocksOnly = true

	_, freeTx := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0, nil)
	freeTx.SetFlags(types.TFDeliverToNode)

	_, paidTx := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 2, nil, networkNum, 0, nil)
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
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	g.BxConfig.WebsocketEnabled = true
	g.feedManager.Subscribe(types.BDNBlocksFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManager.Subscribe(types.TxReceiptsFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManagerChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	var count int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := range g.feedManagerChan {
			fmt.Println(v)
			atomic.AddInt32(&count, 1)
		}
	}()

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	assert.Nil(t, err)

	// send block over bridge from blockchain connection
	err = bridge.SendBlockToBDN(bxBlock, blockchainIPEndpoint)
	assert.Nil(t, err)
	assert.Equal(t, int(ethBlock.Size()), bxBlock.Size())

	msgBytes, err := mockTLS.MockAdvanceSent()
	assert.Nil(t, err)

	// block is broadcast to relays
	var sentBroadcast bxmessage.Broadcast
	err = sentBroadcast.Unpack(msgBytes, relayConn.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, ethBlock.Hash().Bytes(), sentBroadcast.Hash().Bytes())
	assert.Equal(t, networkNum, sentBroadcast.GetNetworkNum())
	assert.Equal(t, 0, len(sentBroadcast.ShortIDs()))

	// try sending block again
	err = bridge.SendBlockToBDN(bxBlock, blockchainIPEndpoint)
	assert.Nil(t, err)

	// times out, nothing sent (already processed)
	msgBytes, err = mockTLS.MockAdvanceSent()
	assert.NotNil(t, err, "unexpected bytes %v", string(msgBytes))
	close(g.feedManagerChan)
	wg.Wait()
	assert.Equal(t, int32(4), atomic.LoadInt32(&count))
}

func TestGateway_HandleBlockFromInboundBlockchain(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	g.BxConfig.WebsocketEnabled = true
	g.feedManager.Subscribe(types.BDNBlocksFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManager.Subscribe(types.TxReceiptsFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManagerChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	var count int32
	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		defer wg.Done()
		for v := range g.feedManagerChan {
			fmt.Println(v.NotificationType())
			atomic.AddInt32(&count, 1)
		}
	}()

	blockchainIPEndpoint = types.NodeEndpoint{IP: "127.0.0.1", Port: 8001, Dynamic: true}
	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	assert.Nil(t, err)

	// send block over bridge from blockchain connection
	err = bridge.SendBlockToBDN(bxBlock, blockchainIPEndpoint)
	assert.Nil(t, err)
	assert.Equal(t, int(ethBlock.Size()), bxBlock.Size())

	msgBytes, err := mockTLS.MockAdvanceSent()
	assert.Nil(t, err)

	// block is broadcast to relays
	var sentBroadcast bxmessage.Broadcast
	err = sentBroadcast.Unpack(msgBytes, relayConn.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, ethBlock.Hash().Bytes(), sentBroadcast.Hash().Bytes())
	assert.Equal(t, networkNum, sentBroadcast.GetNetworkNum())
	assert.Equal(t, 0, len(sentBroadcast.ShortIDs()))

	// try sending block again
	err = bridge.SendBlockToBDN(bxBlock, blockchainIPEndpoint)
	assert.Nil(t, err)

	// times out, nothing sent (already processed)
	msgBytes, err = mockTLS.MockAdvanceSent()
	assert.NotNil(t, err, "unexpected bytes %v", string(msgBytes))
	close(g.feedManagerChan)
	wg.Wait()
	assert.Equal(t, int32(3), atomic.LoadInt32(&count))
}

func TestGateway_HandleBlockFromBlockchain_TwoRelays(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS1, relayConn1 := addRelayConn(g)
	mockTLS2, relayConn2 := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	assert.Nil(t, err)

	// send block over bridge from blockchain connection
	err = bridge.SendBlockToBDN(bxBlock, blockchainIPEndpoint)
	assert.Nil(t, err)
	assert.Equal(t, int(ethBlock.Size()), bxBlock.Size())

	msgBytes1, err := mockTLS1.MockAdvanceSent()
	assert.Nil(t, err)
	msgBytes2, err := mockTLS2.MockAdvanceSent()
	assert.Nil(t, err)

	// block is broadcast to relays
	var sentBlock1 bxmessage.Broadcast
	var sentBlock2 bxmessage.Broadcast
	err = sentBlock1.Unpack(msgBytes1, relayConn1.Protocol())
	assert.Nil(t, err)
	err = sentBlock2.Unpack(msgBytes2, relayConn2.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, ethBlock.Hash().Bytes(), sentBlock1.Hash().Bytes())
	assert.Equal(t, networkNum, sentBlock1.GetNetworkNum())
	assert.Equal(t, 0, len(sentBlock1.ShortIDs()))

	assert.Equal(t, ethBlock.Hash().Bytes(), sentBlock2.Hash().Bytes())
	assert.Equal(t, networkNum, sentBlock2.GetNetworkNum())
	assert.Equal(t, 0, len(sentBlock2.ShortIDs()))

	// try sending block again
	err = bridge.SendBlockToBDN(bxBlock, blockchainIPEndpoint)
	assert.Nil(t, err)

	// times out, nothing sent (already processed)
	msgBytes1, err = mockTLS1.MockAdvanceSent()
	assert.NotNil(t, err, "unexpected bytes %v", string(msgBytes1))
	msgBytes2, err = mockTLS2.MockAdvanceSent()
	assert.NotNil(t, err, "unexpected bytes %v", string(msgBytes2))
}

func TestGateway_HandleBlockFromRelay(t *testing.T) {
	bridge, g := setup(t, 1)
	g.feedManager.Subscribe(types.BDNBlocksFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	_, relayConn1 := addRelayConn(g)
	mockTLS2, _ := addRelayConn(g)

	// separate service instance, to avoid already processed errors
	txStore, bp := newBP()

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))

	// compress a transaction
	bxTransaction, _ := bridge.TransactionBlockchainToBDN(ethBlock.Transactions()[0])
	txStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)
	g.TxStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)

	broadcastMessage, _, err := bp.BxBlockToBroadcast(bxBlock, networkNum, g.sdn.MinTxAge())
	assert.Nil(t, err)

	err = g.HandleMsg(broadcastMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)
	assertNoBlockSentToRelay(t, mockTLS2)
	time.Sleep(1 * time.Millisecond)

	receivedBxBlock := <-bridge.ReceiveEthBlockFromBDN()
	if receivedBxBlock == nil {
		t.FailNow()
	}
	assert.Equal(t, bxBlock.Hash(), receivedBxBlock.Hash())
	assert.Equal(t, bxBlock.Header, receivedBxBlock.Header)
	assert.Equal(t, bxBlock.Trailer, receivedBxBlock.Trailer)
	assert.True(t, bxBlock.Equals(receivedBxBlock))
	assert.Equal(t, bxBlock.Size(), receivedBxBlock.Size())

	// duplicate, no processing
	err = g.HandleMsg(broadcastMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)

	select {
	case <-bridge.ReceiveEthBlockFromBDN():
		assert.Fail(t, "unexpectedly processed block again")
	default:
	}
}

func TestGateway_HandleBeaconBlockFromRelay(t *testing.T) {
	bridge, g := setup(t, 1)
	g.feedManager.Subscribe(types.BDNBlocksFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManager.Subscribe(types.TxReceiptsFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	_, relayConn1 := addRelayConn(g)
	mockTLS2, _ := addRelayConn(g)

	// separate service instance, to avoid already processed errors
	txStore, bp := newBP()

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, _ := bridge.BlockBlockchainToBDN(ethBlock)

	// compress a transaction
	bxTransaction, _ := bridge.TransactionBlockchainToBDN(ethBlock.Transactions()[0])
	txStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)
	g.TxStore.Add(bxTransaction.Hash(), bxTransaction.Content(), 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)

	broadcastMessage, _, err := bp.BxBlockToBroadcast(bxBlock, networkNum, g.sdn.MinTxAge())
	assert.Nil(t, err)

	err = g.HandleMsg(broadcastMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)
	assertNoBlockSentToRelay(t, mockTLS2)
	time.Sleep(1 * time.Millisecond)

	select {
	case receivedBxBlock := <-bridge.ReceiveEthBlockFromBDN():
		if receivedBxBlock == nil {
			t.FailNow()
		}

		assert.Equal(t, bxBlock.Hash(), receivedBxBlock.Hash())
		assert.Equal(t, bxBlock.Header, receivedBxBlock.Header)
		assert.Equal(t, bxBlock.Trailer, receivedBxBlock.Trailer)
		assert.True(t, bxBlock.Equals(receivedBxBlock))
		assert.Equal(t, bxBlock.Size(), receivedBxBlock.Size())
	case <-bridge.ReceiveBeaconBlockFromBDN():
		assert.Fail(t, "unexpectedly processed beacon block")
	default:
		assert.Fail(t, "eth block expected")
	}

	// duplicate, no processing
	err = g.HandleMsg(broadcastMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)

	assertNoBlockSentToRelay(t, mockTLS2)
	time.Sleep(1 * time.Millisecond)

	select {
	case <-bridge.ReceiveBeaconBlockFromBDN():
		assert.Fail(t, "unexpectedly processed block again")
	case <-bridge.ReceiveEthBlockFromBDN():
		assert.Fail(t, "unexpectedly processed eth block")
	default:
	}

	// beacon block after eth block is not treated as duplicate
	beaconBlock := bxmock.NewBellatrixBeaconBlock(t, 11, nil, ethBlock)
	bxBlock, _ = bridge.BlockBlockchainToBDN(beaconBlock)

	broadcastMessage, _, err = bp.BxBlockToBroadcast(bxBlock, networkNum, g.sdn.MinTxAge())
	assert.Nil(t, err)

	err = g.HandleMsg(broadcastMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)
	assertNoBlockSentToRelay(t, mockTLS2)
	time.Sleep(1 * time.Millisecond)

	select {
	case receivedBxBlock := <-bridge.ReceiveBeaconBlockFromBDN():
		if receivedBxBlock == nil {
			t.FailNow()
		}

		assert.Equal(t, bxBlock.Hash(), receivedBxBlock.Hash())
		assert.Nil(t, bxBlock.Header)
		assert.Equal(t, bxBlock.Trailer, receivedBxBlock.Trailer)
		assert.True(t, bxBlock.Equals(receivedBxBlock))
		assert.Equal(t, bxBlock.Size(), receivedBxBlock.Size())
	case <-bridge.ReceiveEthBlockFromBDN():
		assert.Fail(t, "unexpectedly processed block again")
	default:
		assert.Fail(t, "beacon block expected")
	}

	// on other hand same beacon block is duplicate
	err = g.HandleMsg(broadcastMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)
	assertNoBlockSentToRelay(t, mockTLS2)
	time.Sleep(1 * time.Millisecond)

	select {
	case <-bridge.ReceiveEthBlockFromBDN():
		assert.Fail(t, "unexpectedly processed eth block")
	case <-bridge.ReceiveBeaconBlockFromBDN():
		assert.Fail(t, "unexpectedly processed beacon block")
	default:
	}
}

func TestGateway_ValidateHeightBDNBlocksWithNode(t *testing.T) {
	bridge, g := setup(t, 1)
	g.feedManager.Subscribe(types.BDNBlocksFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManager.Subscribe(types.TxReceiptsFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManagerChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	g.BxConfig.WebsocketEnabled = true

	// block from node
	heightFromNode := 10
	expectFeedNotification(t, bridge, g, false, heightFromNode, heightFromNode, 0)

	// skip 1st too far ahead block from BDN
	tooFarAheadHeight := heightFromNode + bxgateway.BDNBlocksMaxBlocksAway + 1
	expectNoFeedNotification(t, bridge, g, true, tooFarAheadHeight, heightFromNode, 1)

	// skip 2nd too far ahead block from BDN
	offset := 5
	expectNoFeedNotification(t, bridge, g, true, tooFarAheadHeight+offset, heightFromNode, 2)

	// skip 3rd too far ahead block from BDN
	offset++
	expectNoFeedNotification(t, bridge, g, true, tooFarAheadHeight+offset, heightFromNode, 3)

	// should publish block from BDN after 3 skipped, clear best height, clear skip count
	offset++
	expectFeedNotification(t, bridge, g, true, tooFarAheadHeight+offset, 0, 0)

	// block from node - set bestBlockHeight
	offset++
	bestBlockHeight := tooFarAheadHeight + offset
	expectFeedNotification(t, bridge, g, false, tooFarAheadHeight+offset, bestBlockHeight, 0)

	// skip 1st too far ahead block from BDN
	tooFarAheadHeight = tooFarAheadHeight + offset + bxgateway.BDNBlocksMaxBlocksAway + 1
	expectNoFeedNotification(t, bridge, g, true, tooFarAheadHeight, bestBlockHeight, 1)

	// skip 2nd too far ahead block from BDN
	offset = 1
	expectNoFeedNotification(t, bridge, g, true, tooFarAheadHeight+offset, bestBlockHeight, 2)

	// skip 3rd too far block from BDN
	offset++
	expectNoFeedNotification(t, bridge, g, true, tooFarAheadHeight+offset, bestBlockHeight, 3)

	// old block from BDN - skip
	tooOldHeight := bestBlockHeight - bxgateway.BDNBlocksMaxBlocksAway - 1
	expectNoFeedNotification(t, bridge, g, true, tooOldHeight, bestBlockHeight, 3)

	// publish 4th too far ahead block, clear best height and skipped block count
	offset++
	expectFeedNotification(t, bridge, g, true, tooFarAheadHeight+offset, 0, 0)

	// block from node - set bestBlockHeight
	expectFeedNotification(t, bridge, g, false, bestBlockHeight+1, bestBlockHeight+1, 0)

	// skip 1st too far ahead block from BDN
	offset++
	expectNoFeedNotification(t, bridge, g, true, tooFarAheadHeight+offset, bestBlockHeight+1, 1)

	// block from node - set bestBlockHeight and clear skipped block count
	expectFeedNotification(t, bridge, g, false, bestBlockHeight+2, bestBlockHeight+2, 0)
}

func TestGateway_BlockFeedIfSubscribeOnly(t *testing.T) {
	bridge, g := setup(t, 1)
	g.feedManagerChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	g.BxConfig.WebsocketEnabled = true
	g.blockchainPeers = []types.NodeEndpoint{}

	// first block from BDN (no blockchain node)
	heightFromNode := 0
	expectNoFeedNotification(t, bridge, g, true, heightFromNode, heightFromNode, 0)

	g.feedManager.Subscribe(types.BDNBlocksFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManager.Subscribe(types.TxReceiptsFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	heightFromNode = 10
	expectFeedNotification(t, bridge, g, true, heightFromNode, heightFromNode, 0)
}

func TestGateway_ValidateHeightBDNBlocksWithoutNode(t *testing.T) {
	bridge, g := setup(t, 1)
	g.feedManager.Subscribe(types.BDNBlocksFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManager.Subscribe(types.TxReceiptsFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManagerChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	g.BxConfig.WebsocketEnabled = true
	g.blockchainPeers = []types.NodeEndpoint{}

	// first block from BDN (no blockchain node)
	heightFromNode := 10
	expectFeedNotification(t, bridge, g, true, heightFromNode, heightFromNode, 0)

	// skip 1st too far block from BDN
	tooFarHeight := heightFromNode + bxgateway.BDNBlocksMaxBlocksAway + 1
	expectNoFeedNotification(t, bridge, g, true, tooFarHeight, heightFromNode, 1)

	// skip 2nd too far block from BDN
	expectNoFeedNotification(t, bridge, g, true, tooFarHeight+5, heightFromNode, 2)

	// skip 3rd too far block from BDN
	expectNoFeedNotification(t, bridge, g, true, tooFarHeight+6, heightFromNode, 3)

	// should publish block from BDN after 3 skipped, reset best height, clear skip count
	expectFeedNotification(t, bridge, g, true, tooFarHeight+7, tooFarHeight+7, 0)
}

func TestGateway_TestNoTxReceiptsWithoutSubscription(t *testing.T) {
	// The test checks that there is no TxReceipts feed notification when there is no corresponding subscription
	bridge, g := setup(t, 1)
	g.feedManager.Subscribe(types.BDNBlocksFeed, types.WebSocketFeed, nil, types.ClientInfo{Tier: string(sdnmessage.ATierEnterprise)}, types.ReqOptions{}, false)
	g.feedManagerChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	g.BxConfig.WebsocketEnabled = true
	g.blockchainPeers = []types.NodeEndpoint{}

	ethBlock := bxmock.NewEthBlock(uint64(10), common.Hash{})
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	err := g.publishBlock(bxBlock, nil, nil, false)
	assert.Nil(t, err)

	expectedNotifications := map[types.FeedType]struct{}{
		types.BDNBlocksFeed: {},
		types.OnBlockFeed:   {},
	}

	// TxReceipts and OnBlock runs in goroutine
	ticker := time.NewTicker(1 * time.Second)

	// It is neccessary to have FailNow to not wait until timeout when something goes wrong
	for len(expectedNotifications) > 0 {
		select {
		case notification := <-g.feedManagerChan:
			if _, ok := expectedNotifications[notification.NotificationType()]; ok {
				delete(expectedNotifications, notification.NotificationType())
				continue
			}
			assert.FailNowf(t, "not expected feed notification", string(notification.NotificationType()))
		case <-ticker.C:
			assert.FailNowf(t, "did not receive expected feed notifications", fmt.Sprintf("%v", expectedNotifications))
		}
	}
}

func expectNoFeedNotification(t *testing.T, bridge blockchain.Bridge, g *gateway, isBDNBlock bool, blockHeight int, expectedBestBlockHeight int, expectedSkipBlockCount int) {
	ethBlock := bxmock.NewEthBlock(uint64(blockHeight), common.Hash{})
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	err := g.publishBlock(bxBlock, nil, nil, !isBDNBlock)
	assert.Nil(t, err)

	select {
	case <-g.feedManagerChan:
		assert.Fail(t, "received unexpected feed notification")
	default:
	}
	assert.Equal(t, expectedBestBlockHeight, g.bestBlockHeight)
	assert.Equal(t, expectedSkipBlockCount, g.bdnBlocksSkipCount)
}

func expectFeedNotification(t *testing.T, bridge blockchain.Bridge, g *gateway, isBDNBlock bool, blockHeight int, expectedBestBlockHeight int, expectedSkipBlockCount int) {
	ethBlock := bxmock.NewEthBlock(uint64(blockHeight), common.Hash{})
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	err := g.publishBlock(bxBlock, nil, nil, !isBDNBlock)
	assert.Nil(t, err)

	expectedNotifications := map[types.FeedType]struct{}{
		types.BDNBlocksFeed:  {},
		types.TxReceiptsFeed: {},
		types.OnBlockFeed:    {},
	}

	if !isBDNBlock {
		expectedNotifications[types.NewBlocksFeed] = struct{}{}
	}

	// TxReceipts and OnBlock runs in goroutine
	ticker := time.NewTicker(1 * time.Second)

	// It is neccessary to have FailNow to not wait until timeout when something goes wrong
	for len(expectedNotifications) > 0 {
		select {
		case notification := <-g.feedManagerChan:
			if _, ok := expectedNotifications[notification.NotificationType()]; ok {
				delete(expectedNotifications, notification.NotificationType())
				continue
			}
			assert.FailNowf(t, "not expected feed notification", string(notification.NotificationType()))
		case <-ticker.C:
			assert.FailNowf(t, "did not receive expected feed notifications", fmt.Sprintf("%v", expectedNotifications))
		}
	}

	assert.Equal(t, expectedBestBlockHeight, g.bestBlockHeight)
	assert.Equal(t, expectedSkipBlockCount, g.bdnBlocksSkipCount)
}

func TestGateway_Status(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages(context.Background())
		assert.Nil(t, err)
	}()

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	bxBlock, err := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	assert.Nil(t, err)

	err = bridge.SendBlockToBDN(bxBlock, blockchainIPEndpoint)
	assert.Nil(t, err)
	assert.Equal(t, int(ethBlock.Size()), bxBlock.Size())

	msgBytes, err := mockTLS.MockAdvanceSent()
	assert.Nil(t, err)

	var sentBroadcast bxmessage.Broadcast
	err = sentBroadcast.Unpack(msgBytes, relayConn.Protocol())
	assert.Nil(t, err)

	assert.Equal(t, ethBlock.Hash().Bytes(), sentBroadcast.Hash().Bytes())
	assert.Equal(t, networkNum, sentBroadcast.GetNetworkNum())
	assert.Equal(t, 0, len(sentBroadcast.ShortIDs()))

	g.timeStarted = time.Now()

	timeNodeConnected := time.Now().Format(time.RFC3339)
	endpoints, stats := createPeerData(timeNodeConnected)
	for _, endpoint := range endpoints {
		g.bdnStats.NodeStats()[endpoint.IPPort()] = stats[endpoint.IPPort()]
	}
	err = g.bridge.SendBlockchainStatusResponse(endpoints)
	assert.Nil(t, err)

	rsp, err := g.Status(context.Background(), &pb.StatusRequest{})
	require.NoError(t, err)
	require.NotNil(t, rsp)
	require.Equal(t, rsp.GatewayInfo.IpAddress, "172.0.0.1")
	require.NotEmpty(t, rsp.GatewayInfo.StartupParams)
	require.Equal(t, g.timeStarted.Format(time.RFC3339), rsp.GatewayInfo.TimeStarted)

	for _, endpoint := range endpoints {
		var (
			expected = stats[endpoint.IPPort()]
			nodeInfo = rsp.Nodes[ipport(endpoint.IP, endpoint.Port)]
		)

		require.NotNil(t, expected)
		require.NotNil(t, nodeInfo)
		require.NotNil(t, nodeInfo.NodePerformance)
		require.Equal(t, nodeInfo.ConnectedAt, timeNodeConnected)

		actual := nodeInfo.NodePerformance

		require.Equal(t, uint32(expected.NewBlocksReceivedFromBlockchainNode), actual.NewBlocksReceivedFromBlockchainNode)
		require.Equal(t, uint32(expected.NewBlocksReceivedFromBdn), actual.NewBlocksReceivedFromBdn)
		require.Equal(t, expected.NewBlocksSeen, actual.NewBlocksSeen)
		require.Equal(t, expected.NewBlockMessagesFromBlockchainNode, actual.NewBlockMessagesFromBlockchainNode)
		require.Equal(t, expected.NewBlockAnnouncementsFromBlockchainNode, actual.NewBlockAnnouncementsFromBlockchainNode)
		require.Equal(t, expected.NewTxReceivedFromBlockchainNode, actual.NewTxReceivedFromBlockchainNode)
		require.Equal(t, expected.NewTxReceivedFromBdn, actual.NewTxReceivedFromBdn)
		require.Equal(t, expected.TxSentToNode, actual.TxSentToNode)
		require.Equal(t, expected.DuplicateTxFromNode, actual.DuplicateTxFromNode)
		require.Equal(t, expected.Dynamic, nodeInfo.Dynamic)
		require.Equal(t, expected.IsConnected, nodeInfo.IsConnected)
	}
}

func TestGateway_ConnectionStatus(t *testing.T) {
	_, g := setup(t, 1)

	var wg sync.WaitGroup
	wg.Add(1)
	go func() {
		for blockchainConnectionStatus := range g.bridge.ReceiveBlockchainConnectionStatus() {
			g.bdnStats.SetBlockchainConnectionStatus(blockchainConnectionStatus)
			wg.Done()
		}
	}()

	g.timeStarted = time.Now()

	err := g.bridge.SendBlockchainConnectionStatus(blockchain.ConnectionStatus{IsConnected: true, PeerEndpoint: types.NodeEndpoint{IP: "123.45.6.78", Port: 1234}})
	assert.Nil(t, err)
	wg.Wait()
	require.True(t, g.bdnStats.NodeStats()["123.45.6.78 1234"].IsConnected)
}

func createPeerData(timeNodeConnected string) ([]*types.NodeEndpoint, map[string]*bxmessage.BdnPerformanceStatsData) {
	endpoints := []*types.NodeEndpoint{
		{
			IP:          "192.168.10.20",
			Port:        30303,
			PublicKey:   "pubkey1",
			Dynamic:     true,
			ConnectedAt: timeNodeConnected,
		},
		{
			IP:          "192.168.10.21",
			Port:        30304,
			PublicKey:   "pubkey2",
			Dynamic:     false,
			ConnectedAt: timeNodeConnected,
		},
		{
			IP:          "192.168.10.22",
			Port:        30305,
			PublicKey:   "pubkey3",
			Dynamic:     true,
			ConnectedAt: timeNodeConnected,
		},
	}

	stats := map[string]*bxmessage.BdnPerformanceStatsData{
		endpoints[0].IPPort(): {
			NewBlocksReceivedFromBlockchainNode:     20,
			NewBlocksReceivedFromBdn:                30,
			NewBlocksSeen:                           10,
			NewBlockMessagesFromBlockchainNode:      10,
			NewBlockAnnouncementsFromBlockchainNode: 20,
			NewTxReceivedFromBlockchainNode:         40,
			NewTxReceivedFromBdn:                    50,
			TxSentToNode:                            100,
			DuplicateTxFromNode:                     50,
			Dynamic:                                 true,
		},
		endpoints[1].IPPort(): {
			NewBlocksReceivedFromBlockchainNode:     21,
			NewBlocksReceivedFromBdn:                31,
			NewBlocksSeen:                           11,
			NewBlockMessagesFromBlockchainNode:      11,
			NewBlockAnnouncementsFromBlockchainNode: 21,
			NewTxReceivedFromBlockchainNode:         41,
			NewTxReceivedFromBdn:                    51,
			TxSentToNode:                            101,
			DuplicateTxFromNode:                     51,
			Dynamic:                                 false,
		},
		endpoints[2].IPPort(): {
			NewBlocksReceivedFromBlockchainNode:     22,
			NewBlocksReceivedFromBdn:                32,
			NewBlocksSeen:                           12,
			NewBlockMessagesFromBlockchainNode:      12,
			NewBlockAnnouncementsFromBlockchainNode: 22,
			NewTxReceivedFromBlockchainNode:         42,
			NewTxReceivedFromBdn:                    52,
			TxSentToNode:                            102,
			DuplicateTxFromNode:                     52,
			Dynamic:                                 true,
		},
	}

	return endpoints, stats
}

// Mocking the context metadata for testing.
func createMockContextWithMetadata(accountIDHeaderVal string) context.Context {
	md := metadata.Pairs(types.OriginalSenderAccountIDHeaderKey, accountIDHeaderVal)
	return metadata.NewIncomingContext(context.Background(), md)
}

func TestretrieveOriginalSenderAccountID_BloxrouteAccountID(t *testing.T) {
	accountModel := &sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.AccountID(types.BloxrouteAccountID),
		},
	}
	ctx := createMockContextWithMetadata(testGatewayAccountID)

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)

	require.Nil(t, err)

	expectedAccountID := types.AccountID(testGatewayAccountID)

	require.Equal(t, expectedAccountID, *accountID)
}

func TestretrieveOriginalSenderAccountID_NonBloxrouteAccountID(t *testing.T) {
	accountModel := &sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.AccountID(testGatewayAccountID),
		},
	}
	ctx := createMockContextWithMetadata(testDifferentAccountID)

	accountID, err := retrieveOriginalSenderAccountID(ctx, accountModel)

	require.Nil(t, err)

	expectedAccountID := types.AccountID(testGatewayAccountID)

	require.Equal(t, expectedAccountID, *accountID)
}

func TestretrieveOriginalSenderAccountID_NoMetadata(t *testing.T) {
	accountModel := &sdnmessage.Account{
		AccountInfo: sdnmessage.AccountInfo{
			AccountID: types.BloxrouteAccountID,
		},
	}
	ctx := context.Background()

	_, err := retrieveOriginalSenderAccountID(ctx, accountModel)

	require.NotNil(t, err)
}
