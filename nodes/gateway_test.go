package nodes

import (
	"context"
	"math"
	"math/big"
	"sync"
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
)

var (
	networkNum           types.NetworkNum = 5
	chainID              int64            = 1
	blockchainIPEndpoint                  = types.NodeEndpoint{IP: "127.0.0.1", Port: 8001}
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
	}

	bridge := blockchain.NewBxBridge(eth.Converter{})
	blockchainPeers, blockchainPeersInfo := ethtest.GenerateBlockchainPeersInfo(numPeers)
	node, _ := NewGateway(context.Background(), bxConfig, bridge, eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout), blockchainPeers, blockchainPeersInfo, "", 0)

	g := node.(*gateway)
	g.sdn = sdn
	g.setupTxStore()
	g.txTrace = loggers.NewTxTrace(nil)
	g.setSyncWithRelay()
	g.feedManager = servers.NewFeedManager(g.context, g, g.feedChan, networkNum, types.NetworkID(chainID),
		g.wsManager, g.sdn.AccountModel(), nil,
		"", "", *g.BxConfig, g.stats)
	return bridge, g
}

func newBP() (*services.BxTxStore, services.BlockProcessor) {
	txStore := services.NewBxTxStore(time.Minute, time.Minute, time.Minute, services.NewEmptyShortIDAssigner(), services.NewHashHistory("seenTxs", time.Minute), nil, 30*time.Minute)
	bp := services.NewRLPBlockProcessor(&txStore)
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
	assert.Equal(t, ethTxBytes, sentTx.Content())
}

func assertNoTransactionSentToRelay(t *testing.T, relayTLS bxmock.MockTLS) {
	_, err := relayTLS.MockAdvanceSent()
	assert.NotNil(t, err)
}

func assertNoBlockSentToRelay(t *testing.T, relayTLS bxmock.MockTLS) {
	_, err := relayTLS.MockAdvanceSent()
	assert.NotNil(t, err)
}

func TestGateway_PushBlockchainConfig(t *testing.T) {
	networkID := int64(1)
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

func TestGateway_HandleTransactionFromBlockchain(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil)
	processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])
	assertTransactionSentToRelay(t, ethTx, ethTxBytes, mockTLS, relayConn)

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
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil)
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
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, 1, nil)
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
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	for i := uint64(1); i <= uint64(limit); i++ {
		ethTx, ethTxBytes := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, i, nil)
		processEthTxOnBridge(t, bridge, ethTx, g.blockchainPeers[0])
		assertTransactionSentToRelay(t, ethTx, ethTxBytes, mockTLS, relayConn)
	}

	ethTx, _ := bxmock.NewSignedEthTxBytes(ethtypes.DynamicFeeTxType, uint64(limit+1), nil)
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
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	for i := uint64(1); i <= uint64(limit); i++ {
		_, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, i, nil, networkNum, 0)
		txMessage.SetFlags(types.TFPaidTx)
		err := g.HandleMsg(txMessage, connections.NewRPCConn(account.AccountID, "", networkNum, utils.Websocket), connections.RunForeground)
		assert.Nil(t, err)
	}

	_, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, uint64(limit+1), nil, networkNum, 0)
	txMessage.SetFlags(types.TFPaidTx)
	err := g.HandleMsg(txMessage, connections.NewRPCConn(account.AccountID, "", networkNum, utils.Websocket), connections.RunForeground)
	assert.Nil(t, err)

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
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
	}()

	for i := uint64(1); i <= uint64(limit); i++ {
		_, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, i, nil, networkNum, 0)
		txMessage.SetFlags(types.TFDeliverToNode)
		err := g.HandleMsg(txMessage, connections.NewRPCConn(account.AccountID, "", networkNum, utils.Websocket), connections.RunForeground)
		assert.Nil(t, err)
	}

	for i := uint64(1); i <= uint64(5); i++ {
		_, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, uint64(limit)+i, nil, networkNum, 0)
		txMessage.SetFlags(types.TFDeliverToNode)
		err := g.HandleMsg(txMessage, connections.NewRPCConn(account.AccountID, "", networkNum, utils.Websocket), connections.RunForeground)
		assert.Nil(t, err)
	}

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
		err := g.handleBridgeMessages()
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
	g.TxStore.Add(hashes[0], types.TxContent{}, 1, networkNum, false, 0, time.Now(), 0, types.EmptySender)
	g.TxStore.Add(hashes[1], types.TxContent{1, 2, 3}, types.ShortIDEmpty, networkNum, false, 0, time.Now(), 0, types.EmptySender)

	err := bridge.AnnounceTransactionHashes(peerID, hashes)
	assert.Nil(t, err)

	request := <-bridge.ReceiveTransactionHashesRequest()

	assert.Equal(t, len(hashes)-2, len(request.Hashes))
	assert.Equal(t, hashes[2], request.Hashes[0])
	assert.Equal(t, hashes[3], request.Hashes[1])

	assert.Equal(t, peerID, request.PeerID)
}

func TestGateway_HandleTransactionFromRPC(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	ethTx, txMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0)
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

	deliveredEthTx, deliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0)
	deliveredTxMessage.SetFlags(types.TFDeliverToNode)

	err := g.HandleMsg(deliveredTxMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)
	assertNoTransactionSentToRelay(t, mockTLS2)

	bdnTxs := <-bridge.ReceiveBDNTransactions()
	assert.Equal(t, 1, len(bdnTxs.Transactions))
	assert.Equal(t, types.NodeEndpoint{IP: relayConn1.Info().PeerIP, Port: int(relayConn1.Info().PeerPort)}, bdnTxs.PeerEndpoint)

	bdnTx := bdnTxs.Transactions[0]
	assert.Equal(t, deliveredEthTx.Hash().Bytes(), bdnTx.Hash().Bytes())

	_, undeliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0)

	err = g.HandleMsg(undeliveredTxMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)

	select {
	case <-bridge.ReceiveBDNTransactions():
		assert.Fail(t, "unexpectedly received txs when TFDeliverToNode not set")
	default:
	}
}

func TestGateway_HandleTransactionFromRelayValidatorOnly(t *testing.T) {
	bridge, g := setup(t, 1)
	_, relayConn1 := addRelayConn(g)
	mockTLS2, _ := addRelayConn(g)

	deliveredEthTx, deliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0)
	deliveredTxMessage.SetFlags(types.TFValidatorsOnly)

	err := g.HandleMsg(deliveredTxMessage, relayConn1, connections.RunForeground)
	assert.Nil(t, err)
	assertNoTransactionSentToRelay(t, mockTLS2)

	bdnTxs := <-bridge.ReceiveBDNTransactions()
	assert.Equal(t, 1, len(bdnTxs.Transactions))
	assert.Equal(t, types.NodeEndpoint{IP: relayConn1.Info().PeerIP, Port: int(relayConn1.Info().PeerPort)}, bdnTxs.PeerEndpoint)

	bdnTx := bdnTxs.Transactions[0]
	assert.Equal(t, deliveredEthTx.Hash().Bytes(), bdnTx.Hash().Bytes())

	_, undeliveredTxMessage := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0)

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
	ethTx, ethTxMsg := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0)
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
	assert.Equal(t, types.NodeEndpoint{IP: relayConn1.Info().PeerIP, Port: int(relayConn1.Info().PeerPort)}, bdnTxs.PeerEndpoint)

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

	_, freeTx := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 1, nil, networkNum, 0)
	freeTx.SetFlags(types.TFDeliverToNode)

	_, paidTx := bxmock.NewSignedEthTxMessage(ethtypes.LegacyTxType, 2, nil, networkNum, 0)
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
		err := g.handleBridgeMessages()
		assert.Nil(t, err)
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
}

func TestGateway_HandleBlockFromBlockchain_TwoRelays(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS1, relayConn1 := addRelayConn(g)
	mockTLS2, relayConn2 := addRelayConn(g)

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
	g.feedManager.Subscribe(types.BDNBlocksFeed, nil, sdnmessage.AccountTier(sdnmessage.ATierEnterprise), types.AccountID(""), "", "", "", "")
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

func TestGateway_ValidateHeightBDNBlocksWithNode(t *testing.T) {
	bridge, g := setup(t, 1)
	g.feedManager.Subscribe(types.BDNBlocksFeed, nil, sdnmessage.AccountTier(sdnmessage.ATierEnterprise), types.AccountID(""), "", "", "", "")
	g.feedChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	g.BxConfig.WebsocketEnabled = true

	// block from node
	heightFromNode := 10
	expectFeedNotification(t, bridge, g, types.NewBlocksFeed, heightFromNode, heightFromNode, 0)

	// skip 1st too far ahead block from BDN
	tooFarAheadHeight := heightFromNode + bxgateway.BDNBlocksMaxBlocksAway + 1
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarAheadHeight, heightFromNode, 1)

	// skip 2nd too far ahead block from BDN
	offset := 5
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarAheadHeight+offset, heightFromNode, 2)

	// skip 3rd too far ahead block from BDN
	offset++
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarAheadHeight+offset, heightFromNode, 3)

	// should publish block from BDN after 3 skipped, clear best height, clear skip count
	offset++
	expectFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarAheadHeight+offset, 0, 0)

	// block from node - set bestBlockHeight
	offset++
	bestBlockHeight := tooFarAheadHeight + offset
	expectFeedNotification(t, bridge, g, types.NewBlocksFeed, tooFarAheadHeight+offset, bestBlockHeight, 0)

	// skip 1st too far ahead block from BDN
	tooFarAheadHeight = tooFarAheadHeight + offset + bxgateway.BDNBlocksMaxBlocksAway + 1
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarAheadHeight, bestBlockHeight, 1)

	// skip 2nd too far ahead block from BDN
	offset = 1
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarAheadHeight+offset, bestBlockHeight, 2)

	// skip 3rd too far block from BDN
	offset++
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarAheadHeight+offset, bestBlockHeight, 3)

	// old block from BDN - skip
	tooOldHeight := bestBlockHeight - bxgateway.BDNBlocksMaxBlocksAway - 1
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooOldHeight, bestBlockHeight, 3)

	// publish 4th too far ahead block, clear best height and skipped block count
	offset++
	expectFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarAheadHeight+offset, 0, 0)

	// block from node - set bestBlockHeight
	expectFeedNotification(t, bridge, g, types.NewBlocksFeed, bestBlockHeight+1, bestBlockHeight+1, 0)

	// skip 1st too far ahead block from BDN
	offset++
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarAheadHeight+offset, bestBlockHeight+1, 1)

	// block from node - set bestBlockHeight and clear skipped block count
	expectFeedNotification(t, bridge, g, types.NewBlocksFeed, bestBlockHeight+2, bestBlockHeight+2, 0)
}

func TestGateway_BlockFeedIfSubscribeOnly(t *testing.T) {
	bridge, g := setup(t, 1)
	g.feedChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	g.BxConfig.WebsocketEnabled = true
	g.blockchainPeers = []types.NodeEndpoint{}

	// first block from BDN (no blockchain node)
	heightFromNode := 0
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, heightFromNode, heightFromNode, 0)

	g.feedManager.Subscribe(types.BDNBlocksFeed, nil, sdnmessage.AccountTier(sdnmessage.ATierEnterprise), types.AccountID(""), "", "", "", "")
	heightFromNode = 10
	expectFeedNotification(t, bridge, g, types.BDNBlocksFeed, heightFromNode, heightFromNode, 0)

}

func TestGateway_ValidateHeightBDNBlocksWithoutNode(t *testing.T) {
	bridge, g := setup(t, 1)
	g.feedManager.Subscribe(types.BDNBlocksFeed, nil, sdnmessage.AccountTier(sdnmessage.ATierEnterprise), types.AccountID(""), "", "", "", "")
	g.feedChan = make(chan types.Notification, bxgateway.BxNotificationChannelSize)
	g.BxConfig.WebsocketEnabled = true
	g.blockchainPeers = []types.NodeEndpoint{}

	// first block from BDN (no blockchain node)
	heightFromNode := 10
	expectFeedNotification(t, bridge, g, types.BDNBlocksFeed, heightFromNode, heightFromNode, 0)

	// skip 1st too far block from BDN
	tooFarHeight := heightFromNode + bxgateway.BDNBlocksMaxBlocksAway + 1
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarHeight, heightFromNode, 1)

	// skip 2nd too far block from BDN
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarHeight+5, heightFromNode, 2)

	// skip 3rd too far block from BDN
	expectNoFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarHeight+6, heightFromNode, 3)

	// should publish block from BDN after 3 skipped, reset best height, clear skip count
	expectFeedNotification(t, bridge, g, types.BDNBlocksFeed, tooFarHeight+7, tooFarHeight+7, 0)
}

func expectNoFeedNotification(t *testing.T, bridge blockchain.Bridge, g *gateway, feedName types.FeedType, blockHeight int, expectedBestBlockHeight int, expectedSkipBlockCount int) {
	ethBlock := bxmock.NewEthBlock(uint64(blockHeight), common.Hash{})
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	err := g.publishBlock(bxBlock, feedName, nil)
	assert.Nil(t, err)
	select {
	case <-g.feedChan:
		assert.Fail(t, "received unexpected feed notification")
	default:
	}
	assert.Equal(t, expectedBestBlockHeight, g.bestBlockHeight)
	assert.Equal(t, expectedSkipBlockCount, g.bdnBlocksSkipCount)
}

func expectFeedNotification(t *testing.T, bridge blockchain.Bridge, g *gateway, feedName types.FeedType, blockHeight int, expectedBestBlockHeight int, expectedSkipBlockCount int) {
	ethBlock := bxmock.NewEthBlock(uint64(blockHeight), common.Hash{})
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, nil))
	err := g.publishBlock(bxBlock, feedName, nil)
	assert.Nil(t, err)
	select {
	case <-g.feedChan:
	default:
		assert.Fail(t, "did not receive expected feed notification")
	}
	if feedName == types.NewBlocksFeed {
		// onBlock notification
		select {
		case <-g.feedChan:
		default:
			assert.Fail(t, "did not receive expected feed notification")
		}
		// txReceipt Notification
		select {
		case <-g.feedChan:
		default:
			assert.Fail(t, "did not receive expected feed notification")
		}
	}
	assert.Equal(t, expectedBestBlockHeight, g.bestBlockHeight)
	assert.Equal(t, expectedSkipBlockCount, g.bdnBlocksSkipCount)
}

func TestGateway_Status(t *testing.T) {
	bridge, g := setup(t, 1)
	mockTLS, relayConn := addRelayConn(g)

	go func() {
		err := g.handleBridgeMessages()
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
	rsp, err := g.Status(context.Background(), &pb.StatusRequest{})
	require.NoError(t, err)
	require.NotNil(t, rsp)
	require.Equal(t, rsp.GatewayInfo.IpAddress, "172.0.0.1")
	require.NotEmpty(t, rsp.GatewayInfo.StartupParams)
	require.Equal(t, g.timeStarted.Format(time.RFC3339), rsp.GatewayInfo.TimeStarted)
}
