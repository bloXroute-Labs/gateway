package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/config"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
	"math/big"
	"net/http"
	"testing"
	"time"
)

var accountIDToAccountModel = map[types.AccountID]sdnmessage.Account{
	"a": {AccountInfo: sdnmessage.AccountInfo{AccountID: "a", TierName: sdnmessage.ATierElite}, SecretHash: "123456"},
	"b": {AccountInfo: sdnmessage.AccountInfo{AccountID: "b", TierName: sdnmessage.ATierDeveloper}, SecretHash: "7891011"},
	"c": {AccountInfo: sdnmessage.AccountInfo{AccountID: "c", TierName: sdnmessage.ATierElite}},
	"gw": {
		AccountInfo: sdnmessage.AccountInfo{
			AccountID:  "gw",
			ExpireDate: "2999-12-31",
			TierName:   sdnmessage.ATierEnterprise,
		},
		SecretHash: "secret",
		NewTransactionStreaming: sdnmessage.BDNFeedService{
			ExpireDate: "2999-12-31",
			Feed: sdnmessage.FeedProperties{
				AllowFiltering:  true,
				AvailableFields: []string{"all"},
			},
		},
		TransactionReceiptFeed: sdnmessage.BDNFeedService{
			ExpireDate: "2999-12-31",
			Feed: sdnmessage.FeedProperties{
				AllowFiltering:  true,
				AvailableFields: []string{"all"},
			},
		},
		OnBlockFeed: sdnmessage.BDNFeedService{
			ExpireDate: "2999-12-31",
			Feed: sdnmessage.FeedProperties{
				AllowFiltering:  true,
				AvailableFields: []string{"all"},
			},
		},
	},
}

func getMockCustomerAccountModel(accountID types.AccountID) (sdnmessage.Account, error) {
	var err error
	if accountID == "d" {
		err = fmt.Errorf("Timeout error")
	}
	return accountIDToAccountModel[accountID], err
}

func reset(fm *FeedManager, wsURL string, blockchainPeers []types.NodeEndpoint) *websocket.Conn {
	fm.CloseAllClientConnections()
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
	clearWSProviderStats(fm, blockchainPeers)
	return newWSConn(wsURL)
}

func clearWSProviderStats(fm *FeedManager, blockchainPeers []types.NodeEndpoint) {
	for _, wsProvider := range fm.nodeWSManager.Providers() {
		wsProvider.(*eth.MockWSProvider).NumReceiptsFetched = 0
		wsProvider.(*eth.MockWSProvider).NumRPCCalls = 0
	}
}

func markAllPeersWithSyncStatus(fm *FeedManager, blockchainPeers []types.NodeEndpoint, status blockchain.NodeSyncStatus) {
	for _, peer := range blockchainPeers {
		fm.nodeWSManager.UpdateNodeSyncStatus(peer, status)
	}
}

func newWSConn(wsURL string) *websocket.Conn {
	dialer := websocket.DefaultDialer
	headers := make(http.Header)
	dummyAuthHeader := "Z3c6c2VjcmV0"
	headers.Set("Authorization", dummyAuthHeader)
	ws, _, err := dialer.Dial(wsURL, headers)
	if err != nil {
		panic(err)
	}
	return ws
}

func TestClientHandler(t *testing.T) {
	// set a shorted delay for tests
	ErrWSConnDelay = 10 * time.Millisecond
	g := bxmock.MockBxListener{}
	stats := statistics.NoStats{}
	feedChan := make(chan types.Notification)
	url := "127.0.0.1:28332"
	wsURLs := []string{fmt.Sprintf("ws://%s/ws", url), fmt.Sprintf("ws://%s/", url)}

	gwAccount, _ := getMockCustomerAccountModel("gw")
	cfg := config.Bx{WebsocketPort: 28332, ManageWSServer: true, WebsocketTLSEnabled: false}

	blockchainPeers, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)
	fm := NewFeedManager(context.Background(), g, feedChan, types.NetworkNum(1), 1, eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout), gwAccount, getMockCustomerAccountModel, "", "", cfg, stats)
	providers := fm.nodeWSManager.Providers()
	p1 := providers[blockchainPeers[0].IPPort()]
	assert.NotNil(t, p1)
	p2 := providers[blockchainPeers[1].IPPort()]
	assert.NotNil(t, p2)
	p3 := providers[blockchainPeers[2].IPPort()]
	assert.NotNil(t, p3)

	var group errgroup.Group
	clientHandler := NewClientHandler(fm, nil, NewHTTPServer(fm, cfg.HTTPPort), log.WithFields(log.Fields{
		"component": "gatewayClientHandler",
	}))
	go clientHandler.ManageWSServer(cfg.ManageWSServer)
	go clientHandler.ManageHTTPServer(context.Background())
	group.Go(fm.Start)
	time.Sleep(10 * time.Millisecond)

	dialer := websocket.DefaultDialer
	headers := make(http.Header)

	// check both /ws and / endpoints
	for _, wsURL := range wsURLs {
		clearWSProviderStats(fm, blockchainPeers)
		markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Unsynced)
		time.Sleep(time.Millisecond)
		markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
		time.Sleep(5 * time.Millisecond)

		// pass - different account for server and client (simulate internal GW)
		dummyAuthHeader := "YToxMjM0NTY=" // a:123456
		headers.Set("Authorization", dummyAuthHeader)
		ws, _, err := dialer.Dial(wsURL, headers)
		handleError(t, ws, nil)

		// fail for tier type
		dummyAuthHeader = "Yjo3ODkxMDEx" // b:7891011
		headers.Set("Authorization", dummyAuthHeader)
		ws, _, err = dialer.Dial(wsURL, headers)
		handleError(t, ws, &websocket.CloseError{Code: 1008, Text: "account must be enterprise / enterprise elite / ultra"})

		// fail for secret hash
		dummyAuthHeader = "Yzo3ODkxMDEx" //c:7891011
		headers.Set("Authorization", dummyAuthHeader)
		ws, _, err = dialer.Dial(wsURL, headers)
		handleError(t, ws, &websocket.CloseError{Code: 1008, Text: "wrong value in the authorization header"})

		// pass for timeout - account should set to elite
		dummyAuthHeader = "ZDo3ODkxMDEx" // d:7891011
		headers.Set("Authorization", dummyAuthHeader)
		ws, _, err = dialer.Dial(wsURL, headers)
		handleError(t, ws, nil)

		// pass - same account for server and client
		dummyAuthHeader = "Z3c6c2VjcmV0" //gw:secret
		headers.Set("Authorization", dummyAuthHeader)
		ws, _, err = dialer.Dial(wsURL, headers)
		assert.Nil(t, err)

		t.Run(fmt.Sprintf("wsClient-%s", wsURL), func(t *testing.T) {
			handlePingRequest(t, ws)
			handleBlxrTxRequestLegacyTx(t, ws)
			handleBlxrTxsRequestLegacyTx(t, ws)
			handleBlxrTxRequestAccessListTx(t, ws)
			handleBlxrTxRequestDynamicFeeTx(t, ws)
			handleBlxrTxRequestTxWithPrefix(t, ws)
			handleBlxrTxRequestRLPTx(t, ws)
			handleBlxrTxWithWrongChainID(t, ws)
			handleSubscribe(t, fm, ws)
			handleTxReceiptsSubscribe(t, fm, ws)
			handleInvalidSubscribe(t, ws)
			testWSShutdown(t, fm, ws, blockchainPeers)
		})
		// restart bc last test shut down ws server
		fm = NewFeedManager(context.Background(), g, feedChan, types.NetworkNum(1), 1, eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout), gwAccount, getMockCustomerAccountModel, "", "", cfg, stats)
		clientHandler = NewClientHandler(fm, nil, NewHTTPServer(fm, cfg.HTTPPort), log.WithFields(log.Fields{
			"component": "gatewayClientHandler",
		}))
		go clientHandler.ManageWSServer(cfg.ManageWSServer)
		go clientHandler.ManageHTTPServer(context.Background())
		group.Go(fm.Start)
		time.Sleep(10 * time.Millisecond)
	}

	// check disconnected subscription
	clearWSProviderStats(fm, blockchainPeers)
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
	time.Sleep(5 * time.Millisecond)
	authHeader := "Z3c6c2VjcmV0" //gw:secret
	headers.Set("Authorization", authHeader)
	ws, _, err := dialer.Dial(wsURLs[0], headers)
	assert.Nil(t, err)
	_, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`)
	time.Sleep(time.Millisecond)
	assert.True(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
	ws.Close()
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))

	// check subscribe and unsubscribe using different endpoints
	clearWSProviderStats(fm, blockchainPeers)
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
	time.Sleep(time.Millisecond)

	// pass - same account for server and client
	dummyAuthHeader := "Z3c6c2VjcmV0"
	headers.Set("Authorization", dummyAuthHeader)
	_, _, errWs0 := dialer.Dial(wsURLs[0], headers)
	assert.Nil(t, errWs0)

}

//TODO: Follow work to handle sync and unsync on another PR
//func TestHandleClient_Notification(t *testing.T) {
//	g := bxmock.MockBxListener{}
//	stats := statistics.NoStats{}
//	feedChan := make(chan types.Notification)
//	url := "127.0.0.1:28332"
//	wsURLs := []string{fmt.Sprintf("ws://%s/ws", url), fmt.Sprintf("ws://%s/", url)}
//
//	gwAccount, _ := getMockCustomerAccountModel("gw")
//	cfg := config.Bx{WebsocketPort: 28332, ManageWSServer: true, WebsocketTLSEnabled: false}
//
//	blockchainPeers, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)
//	fm := NewFeedManager(context.Background(), g, feedChan, types.NetworkNum(1), eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout), gwAccount, getMockCustomerAccountModel, "", "", cfg, stats)
//	providers := fm.nodeWSManager.Providers()
//	p1 := providers[blockchainPeers[0].IPPort()]
//	assert.NotNil(t, p1)
//	p2 := providers[blockchainPeers[1].IPPort()]
//	assert.NotNil(t, p2)
//	p3 := providers[blockchainPeers[2].IPPort()]
//	assert.NotNil(t, p3)
//
//	var group errgroup.Group
//	group.Go(fm.Start)
//	time.Sleep(10 * time.Millisecond)
//
//	dialer := websocket.DefaultDialer
//	headers := make(http.Header)
//
//	// check both /ws and / endpoints
//	for _, wsURL := range wsURLs {
//		clearWSProviderStats(fm, blockchainPeers)
//		markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Unsynced)
//		time.Sleep(time.Millisecond)
//		markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
//		time.Sleep(5 * time.Millisecond)
//
//		t.Run(fmt.Sprintf("wsClient-%s", wsURL), func(t *testing.T) {
//			//handleTxReceiptsNotification(t, fm, ws, blockchainPeers)
//			//handleOnBlockNotification(t, fm, ws, blockchainPeers)
//			//TODO: Follow work to handle sync and unsync
//			//handleTxReceiptsNotificationRequestedUnsynced(t, fm, ws, blockchainPeers)
//			//handleOnBlockNotificationRequestedUnsynced(t, fm, ws, blockchainPeers)
//			//handleTxReceiptsNotificationNoneSynced(t, fm, ws, blockchainPeers)
//			//ws = reset(fm, wsURL, blockchainPeers)
//			//handleOnBlockNotificationNoneSynced(t, fm, ws, blockchainPeers)
//			//ws = reset(fm, wsURL, blockchainPeers)
//			testWSShutdown(t, fm, ws, blockchainPeers)
//			{
//				// restart bc last test shut down ws server
//				fm = NewFeedManager(context.Background(), g, feedChan, types.NetworkNum(1), eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout), gwAccount, getMockCustomerAccountModel, "", "", cfg, stats)
//				group.Go(fm.Start)
//				time.Sleep(10 * time.Millisecond)
//			}
//		})
//	}
//
//	// check subscribe and unsubscribe using different endpoints
//	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Unsynced)
//	time.Sleep(time.Millisecond)
//	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
//	time.Sleep(time.Millisecond)
//
//	// pass - same account for server and client
//	dummyAuthHeader := "Z3c6c2VjcmV0"
//	headers.Set("Authorization", dummyAuthHeader)
//	_, _, errWs0 := dialer.Dial(wsURLs[0], headers)
//	assert.Nil(t, errWs0)
//}

func handleSubscribe(t *testing.T, fm *FeedManager, ws *websocket.Conn) {
	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`)
	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleInvalidSubscribe(t *testing.T, ws *websocket.Conn) {
	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "subscribe", "para": ["txReceipts", {"include": []}]}`), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	assert.Nil(t, clientRes.Result)
	time.Sleep(time.Millisecond)
	handlePingRequest(t, ws)
}

/* the below functions are not in use
func handleTxReceiptsSubscribeClientCloseConnection(t *testing.T, fm *FeedManager, ws *websocket.Conn) {
	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "subscribe", "params": ["txReceipts", {"include": []}]}`), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	subscriptionID, err := uuid.FromString(fmt.Sprintf("%v", clientRes.Result))
	assert.Nil(t, err)
	assert.True(t, fm.SubscriptionExists(subscriptionID))

	fm.feedChan <- mockBlockTransaction()

	err = ws.Close()
	assert.Nil(t, err)

	time.Sleep(5 * time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
}

func mockBlockTransaction() types.Notification {
	var notification types.Notification
	blockNotification := &types.BlockNotification{}
	blockNotification.Transactions = getEthTransactions()
	blockNotification.SetNotificationType(types.TxReceiptsFeed)
	notification = blockNotification
	return notification
}

func getEthTransactions() []map[string]interface{} {
	var ret []map[string]interface{}
	//var fromBytes common.Address
	//rand.Read(fromBytes[:])
	// todo: create ethTransaction with the new function
	tx := types.EthTransaction{
		GasTipCap: big.NewInt(100),
		GasFeeCap: big.NewInt(100),
	}.Fields(types.AllFields)
	txSame := types.EthTransaction{
		GasTipCap: big.NewInt(100),
		GasFeeCap: big.NewInt(100),
	}.Fields(types.AllFields)
	txLowerGas := types.EthTransaction{
		GasTipCap: big.NewInt(5),
		GasFeeCap: big.NewInt(5),
	}.Fields(types.AllFields)
	txSlightlyHigherGas := types.EthTransaction{
		GasTipCap: big.NewInt(101),
		GasFeeCap: big.NewInt(101),
	}.Fields(types.AllFields)
	txHigherGas := types.EthTransaction{
		GasTipCap: big.NewInt(111),
		GasFeeCap: big.NewInt(111),
	}.Fields(types.AllFields)
	ret = append(ret, tx, txSame, txLowerGas, txSlightlyHigherGas, txHigherGas)

	return ret
}
*/

func handleTxReceiptsSubscribe(t *testing.T, fm *FeedManager, ws *websocket.Conn) {
	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["txReceipts", {"include": []}]}`)
	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleTxReceiptsNotification(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	bridge := blockchain.NewBxBridge(eth.Converter{})
	wsProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, wsProvider.BlockchainPeerEndpoint(), blockchainPeers[0])

	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["txReceipts", {"include": []}]}`)

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, td))
	numTx := len(bxBlock.Txs)
	feedNotification, _ := bridge.BxBlockToCanonicFormat(bxBlock)
	feedNotification.SetNotificationType(types.TxReceiptsFeed)
	sourceEndpoint := types.NodeEndpoint{IP: blockchainPeers[0].IP, Port: blockchainPeers[0].Port}
	feedNotification.SetSource(&sourceEndpoint)
	assert.True(t, fm.nodeWSManager.Synced())

	fm.feedChan <- feedNotification
	time.Sleep(5 * time.Millisecond)
	assert.Equal(t, numTx, wsProvider.(*eth.MockWSProvider).NumReceiptsFetched)

	// expect receipt notifications
	for i := 0; i < numTx; i++ {
		_, message, err := ws.ReadMessage()
		assert.Nil(t, err)
		var m json.RawMessage
		err = json.Unmarshal(message, &m)
		assert.Nil(t, err)
	}

	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleOnBlockNotification(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	bridge := blockchain.NewBxBridge(eth.Converter{})
	wsProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, wsProvider.BlockchainPeerEndpoint(), blockchainPeers[0])

	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["ethOnBlock", {"include": [], "call-params":  [{"method": "eth_blockNumber", "name": "height"}] }]}`)

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, td))
	feedNotification, _ := bridge.BxBlockToCanonicFormat(bxBlock)
	feedNotification.SetNotificationType(types.OnBlockFeed)
	sourceEndpoint := types.NodeEndpoint{blockchainPeers[0].IP, blockchainPeers[0].Port, "", false}
	feedNotification.SetSource(&sourceEndpoint)
	assert.True(t, fm.nodeWSManager.Synced())

	fm.feedChan <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, 1, wsProvider.(*eth.MockWSProvider).NumRPCCalls)

	// expect onBlock notification and TaskCompletedEvent
	for i := 0; i < 2; i++ {
		_, message, err := ws.ReadMessage()
		assert.Nil(t, err)
		var m json.RawMessage
		err = json.Unmarshal(message, &m)
		assert.Nil(t, err)
	}

	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleTxReceiptsNotificationRequestedUnsynced(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	clearWSProviderStats(fm, blockchainPeers)
	bridge := blockchain.NewBxBridge(eth.Converter{})
	requestedUnsyncedWSProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, requestedUnsyncedWSProvider.BlockchainPeerEndpoint(), blockchainPeers[0])
	expectedSyncedWSProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[2])
	assert.True(t, ok)
	assert.Equal(t, expectedSyncedWSProvider.BlockchainPeerEndpoint(), blockchainPeers[2])

	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["txReceipts", {"include": []}]}`)

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, td))
	numTx := len(bxBlock.Txs)
	feedNotification, _ := bridge.BxBlockToCanonicFormat(bxBlock)
	feedNotification.SetNotificationType(types.TxReceiptsFeed)
	sourceEndpoint := types.NodeEndpoint{blockchainPeers[0].IP, blockchainPeers[0].Port, "", false}
	feedNotification.SetSource(&sourceEndpoint)

	fm.nodeWSManager.UpdateNodeSyncStatus(requestedUnsyncedWSProvider.BlockchainPeerEndpoint(), blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[1], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.True(t, fm.nodeWSManager.Synced())

	fm.feedChan <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, numTx, expectedSyncedWSProvider.(*eth.MockWSProvider).NumReceiptsFetched)
	assert.Equal(t, 0, requestedUnsyncedWSProvider.(*eth.MockWSProvider).NumReceiptsFetched)

	// expect receipt notifications
	for i := 0; i < numTx; i++ {
		_, message, err := ws.ReadMessage()
		assert.Nil(t, err)
		var m json.RawMessage
		err = json.Unmarshal(message, &m)
		assert.Nil(t, err)
	}

	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleOnBlockNotificationRequestedUnsynced(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	clearWSProviderStats(fm, blockchainPeers)
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
	time.Sleep(time.Millisecond)

	bridge := blockchain.NewBxBridge(eth.Converter{})

	requestedUnsyncedWSProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, requestedUnsyncedWSProvider.BlockchainPeerEndpoint(), blockchainPeers[0])
	expectedSyncedWSProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[2])
	assert.True(t, ok)
	assert.Equal(t, expectedSyncedWSProvider.BlockchainPeerEndpoint(), blockchainPeers[2])

	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["ethOnBlock", {"include": [], "call-params":  [{"method": "eth_blockNumber", "name": "height"}] }]}`)

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, td))
	feedNotification, _ := bridge.BxBlockToCanonicFormat(bxBlock)
	feedNotification.SetNotificationType(types.OnBlockFeed)
	sourceEndpoint := types.NodeEndpoint{blockchainPeers[0].IP, blockchainPeers[0].Port, "", false}
	feedNotification.SetSource(&sourceEndpoint)

	fm.nodeWSManager.UpdateNodeSyncStatus(requestedUnsyncedWSProvider.BlockchainPeerEndpoint(), blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[1], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.True(t, fm.nodeWSManager.Synced())

	fm.feedChan <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, 1, expectedSyncedWSProvider.(*eth.MockWSProvider).NumRPCCalls)
	assert.Equal(t, 0, requestedUnsyncedWSProvider.(*eth.MockWSProvider).NumRPCCalls)

	// expect onBlock notification and TaskCompletedEvent
	for i := 0; i < 2; i++ {
		_, message, err := ws.ReadMessage()
		assert.Nil(t, err)
		var m json.RawMessage
		err = json.Unmarshal(message, &m)
		assert.Nil(t, err)
	}

	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleTxReceiptsNotificationNoneSynced(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	clearWSProviderStats(fm, blockchainPeers)
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
	time.Sleep(time.Millisecond)

	bridge := blockchain.NewBxBridge(eth.Converter{})

	ws0, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, ws0.BlockchainPeerEndpoint(), blockchainPeers[0])
	ws1, ok := fm.nodeWSManager.Provider(&blockchainPeers[1])
	assert.True(t, ok)
	assert.Equal(t, ws0.BlockchainPeerEndpoint(), blockchainPeers[0])
	ws2, ok := fm.nodeWSManager.Provider(&blockchainPeers[2])
	assert.True(t, ok)
	assert.Equal(t, ws2.BlockchainPeerEndpoint(), blockchainPeers[2])

	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "subscribe", "params": ["txReceipts", {"include": []}]}`), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	subscriptionID, err := uuid.FromString(fmt.Sprintf("%v", clientRes.Result))
	assert.Nil(t, err)
	assert.True(t, fm.SubscriptionExists(subscriptionID))

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, td))
	feedNotification, err := bridge.BxBlockToCanonicFormat(bxBlock)
	feedNotification.SetNotificationType(types.TxReceiptsFeed)
	sourceEndpoint := types.NodeEndpoint{blockchainPeers[0].IP, blockchainPeers[0].Port, "", false}
	feedNotification.SetSource(&sourceEndpoint)

	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[0], blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[1], blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[2], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.nodeWSManager.Synced())
	assert.False(t, fm.SubscriptionExists(subscriptionID))

	fm.feedChan <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, 0, ws0.(*eth.MockWSProvider).NumReceiptsFetched)
	assert.Equal(t, 0, ws1.(*eth.MockWSProvider).NumReceiptsFetched)
	assert.Equal(t, 0, ws2.(*eth.MockWSProvider).NumReceiptsFetched)
}

func handleOnBlockNotificationNoneSynced(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	bridge := blockchain.NewBxBridge(eth.Converter{})

	ws0, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, ws0.BlockchainPeerEndpoint(), blockchainPeers[0])
	ws1, ok := fm.nodeWSManager.Provider(&blockchainPeers[1])
	assert.True(t, ok)
	assert.Equal(t, ws0.BlockchainPeerEndpoint(), blockchainPeers[0])
	ws2, ok := fm.nodeWSManager.Provider(&blockchainPeers[2])
	assert.True(t, ok)
	assert.Equal(t, ws2.BlockchainPeerEndpoint(), blockchainPeers[2])

	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "subscribe", "params": ["ethOnBlock", {"include": [], "call-params":  [{"method": "eth_blockNumber", "name": "height"}] }]}`), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	subscriptionID, err := uuid.FromString(fmt.Sprintf("%v", clientRes.Result))
	assert.Nil(t, err)
	assert.True(t, fm.SubscriptionExists(subscriptionID))

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, td))
	feedNotification, err := bridge.BxBlockToCanonicFormat(bxBlock)
	feedNotification.SetNotificationType(types.OnBlockFeed)
	sourceEndpoint := types.NodeEndpoint{blockchainPeers[0].IP, blockchainPeers[0].Port, "", false}
	feedNotification.SetSource(&sourceEndpoint)

	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[0], blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[1], blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[2], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.nodeWSManager.Synced())
	assert.False(t, fm.SubscriptionExists(subscriptionID))

	fm.feedChan <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, 0, ws0.(*eth.MockWSProvider).NumRPCCalls)
	assert.Equal(t, 0, ws1.(*eth.MockWSProvider).NumRPCCalls)
	assert.Equal(t, 0, ws2.(*eth.MockWSProvider).NumRPCCalls)
}

func testWSShutdown(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	subscriptionID, err := uuid.FromString(fmt.Sprintf("%v", clientRes.Result))
	assert.Nil(t, err)
	assert.True(t, fm.SubscriptionExists(subscriptionID))

	subscribeMsg2 := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "2", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash","tx_contents.gas_price"]}]}`), nil)
	clientRes2 := getClientResponse(t, subscribeMsg2)
	subscriptionID2, err := uuid.FromString(fmt.Sprintf("%v", clientRes2.Result))
	assert.Nil(t, err)
	assert.True(t, fm.SubscriptionExists(subscriptionID2))

	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[0], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.True(t, fm.SubscriptionExists(subscriptionID))
	assert.True(t, fm.SubscriptionExists(subscriptionID2))

	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[1], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.True(t, fm.SubscriptionExists(subscriptionID))
	assert.True(t, fm.SubscriptionExists(subscriptionID2))

	// ws server only shuts down once no synced nodes remain
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[2], blockchain.Unsynced)
	assert.False(t, fm.nodeWSManager.Synced())
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	assert.False(t, fm.SubscriptionExists(subscriptionID2))
}

func handlePingRequest(t *testing.T, ws *websocket.Conn) {
	timeClientSendsRequest := time.Now().UTC()
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "ping"}`), nil)
	timeClientReceivesResponse := time.Now().UTC()

	clientRes := getClientResponse(t, msg)
	res := parsePingResult(t, clientRes.Result)
	timeServerReceivesRequest, err := time.Parse(bxgateway.MicroSecTimeFormat, res.Pong)
	assert.Nil(t, err)
	assert.True(t, timeClientReceivesResponse.After(timeServerReceivesRequest))
	assert.True(t, timeServerReceivesRequest.After(timeClientSendsRequest))
}

func handleError(t *testing.T, ws *websocket.Conn, closeError *websocket.CloseError) {
	_ = writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "ping"}`), closeError)
}

func handleBlxrTxRequestLegacyTx(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.LegacyTransaction)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	res := parseBlxrTxResult(t, clientRes.Result)
	assert.Equal(t, fixtures.LegacyTransactionHash[2:], res.TxHash)
	assert.Nil(t, clientRes.Error)
}

func handleBlxrTxRequestRLPTx(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.RLPTransaction)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	res := parseBlxrTxResult(t, clientRes.Result)
	assert.Equal(t, fixtures.RLPTransactionHash[2:], res.TxHash)
	assert.Nil(t, clientRes.Error)
}

func handleBlxrTxWithWrongChainID(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.LegacyTransactionBSC)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	assert.NotNil(t, clientRes.Error)
}

func handleBlxrTxsRequestLegacyTx(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_batch_tx", "params": {"transactions": ["%s"]}}`, fixtures.LegacyTransaction)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	res := parseBlxrTxsResult(t, clientRes.Result)
	assert.Equal(t, fixtures.LegacyTransactionHash[2:], res.TxHashes[0])
}

func handleBlxrTxRequestAccessListTx(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.AccessListTransactionForRPCInterface)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	res := parseBlxrTxResult(t, clientRes.Result)
	assert.Equal(t, fixtures.AccessListTransactionHash[2:], res.TxHash)
}

func handleBlxrTxRequestDynamicFeeTx(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.DynamicFeeTransactionForRPCInterface)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	res := parseBlxrTxResult(t, clientRes.Result)
	assert.Equal(t, fixtures.DynamicFeeTransactionHash[2:], res.TxHash)
}

func handleBlxrTxRequestTxWithPrefix(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, "0x"+fixtures.DynamicFeeTransactionForRPCInterface)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	res := parseBlxrTxResult(t, clientRes.Result)
	assert.Equal(t, fixtures.DynamicFeeTransactionHash[2:], res.TxHash)
}

func TestSendBundleArgs_Validate(t *testing.T) {
	invalidTransactions, err := hexutil.Decode("0x")
	require.NoError(t, err)
	validTransactions, err := hexutil.Decode("0xf8708344ca68852cb417800083032918943b815bb2ee63fdddf3bb9e6cf7ccbf8311dea5968803d61ab6e77d90008026a0b1fbc7fc1a0a038485315c67f93556b2c7edd0c9ae4873992ab0a03ff71dac65a0185fe97853d910164b792becbc25c1e6c8cdae9f2f6dfbf2854d98a818765aaa")
	require.NoError(t, err)

	testCases := []struct {
		name    string
		payload sendBundleArgs
		error   error
	}{
		{
			name: "bundle without transactions",
			payload: sendBundleArgs{
				Txs: []hexutil.Bytes{},
			},
			error: errors.New("bundle missing txs"),
		},
		{
			name: "invalid bundle transactions",
			payload: sendBundleArgs{
				Txs:         []hexutil.Bytes{invalidTransactions},
				BlockNumber: "test",
			},
			error: errors.New("typed transaction too short"),
		},
		{
			name: "empty block number",
			payload: sendBundleArgs{
				Txs:         []hexutil.Bytes{validTransactions},
				BlockNumber: "",
			},
			error: errors.New("bundle missing blockNumber"),
		},
		{
			name: "invalid block number",
			payload: sendBundleArgs{
				Txs:         []hexutil.Bytes{validTransactions},
				BlockNumber: "A",
			},
			error: errors.New(`blockNumber must be hex, hex string without 0x prefix`),
		},
		{
			name: "valid payload with hex block number with 0x",
			payload: sendBundleArgs{
				Txs:         []hexutil.Bytes{validTransactions},
				BlockNumber: "0xcccccc",
			},
			error: nil,
		},
	}

	for _, tt := range testCases {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.payload.validate()
			assert.Equal(t, tt.error, err)
		})
	}
}

type clientResponse struct {
	Jsonrpc string      `json:"JSONRPC"`
	ID      string      `json:"id"`
	Result  interface{} `json:"result"`
	Error   interface{} `json:"error"`
}

func getClientResponse(t *testing.T, msg []byte) (cr clientResponse) {
	res := clientResponse{}
	err := json.Unmarshal(msg, &res)
	assert.Nil(t, err)
	return res
}

func parsePingResult(t *testing.T, rpcResponse interface{}) (pr rpcPingResponse) {
	res := rpcPingResponse{}
	b, err := json.Marshal(rpcResponse)
	assert.Nil(t, err)
	err = json.Unmarshal(b, &res)
	assert.Nil(t, err)
	return res
}

func parseBlxrTxResult(t *testing.T, rpcResponse interface{}) (tr rpcTxResponse) {
	res := rpcTxResponse{}
	b, err := json.Marshal(rpcResponse)
	assert.Nil(t, err)
	err = json.Unmarshal(b, &res)
	assert.Nil(t, err)
	return res
}

func parseBlxrTxsResult(t *testing.T, rpcResponse interface{}) (tr rpcBatchTxResponse) {
	res := rpcBatchTxResponse{}
	b, err := json.Marshal(rpcResponse)
	assert.Nil(t, err)
	err = json.Unmarshal(b, &res)
	assert.Nil(t, err)
	return res
}

func writeMsgToWsAndReadResponse(t *testing.T, conn *websocket.Conn, msg []byte, expectedErr *websocket.CloseError) (response []byte) {
	err := conn.WriteMessage(websocket.TextMessage, msg)
	assert.Nil(t, err)
	_, response, err = conn.ReadMessage()
	assert.True(t, (expectedErr == nil && err == nil) || (expectedErr != nil && err != nil))
	return response
}

func assertSubscribe(t *testing.T, ws *websocket.Conn, fm *FeedManager, filter string) (string, uuid.UUID) {
	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(filter), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	subscriptionID, err := uuid.FromString(fmt.Sprintf("%v", clientRes.Result))
	assert.Nil(t, err)
	assert.True(t, fm.SubscriptionExists(subscriptionID))
	return fmt.Sprintf(
		`{"id": 1, "method": "unsubscribe", "params": ["%v"]}`,
		subscriptionID,
	), subscriptionID
}

func TestFeedsLimit(t *testing.T) {
	// set a shorted delay for tests
	//ErrWSConnDelay = 10 * time.Millisecond
	g := bxmock.MockBxListener{}
	stats := statistics.NoStats{}
	feedChan := make(chan types.Notification)

	bloxrouteAccount, _ := getMockCustomerAccountModel("bloxroute Labs")
	gwAccount, _ := getMockCustomerAccountModel("gw")

	cfg := config.Bx{WebsocketPort: 28332, ManageWSServer: true, WebsocketTLSEnabled: false, NodeType: utils.InternalGateway}

	_, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)
	fm := NewFeedManager(context.Background(), g, feedChan, types.NetworkNum(1), 1, eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout), bloxrouteAccount, getMockCustomerAccountModel, "", "", cfg, stats)
	fm.accountModel.NewTransactionStreaming.Feed.Limit = 10

	for i := 0; i < 10; i++ {
		_, _, err := fm.Subscribe(types.BDNBlocksFeed, nil, sdnmessage.AccountTier(sdnmessage.ATierEnterprise), types.AccountID("gw"), "", "", "", "")
		assert.Nil(t, err)
	}
	_, _, err := fm.Subscribe(types.BDNBlocksFeed, nil, sdnmessage.AccountTier(sdnmessage.ATierEnterprise), types.AccountID("gw"), "", "", "", "")
	assert.NotNil(t, err)

	// go it with external gw - should not block
	cfg = config.Bx{WebsocketPort: 28332, ManageWSServer: true, WebsocketTLSEnabled: false}

	_, blockchainPeersInfo = test.GenerateBlockchainPeersInfo(3)
	fm = NewFeedManager(context.Background(), g, feedChan, types.NetworkNum(1), 1, eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout), gwAccount, getMockCustomerAccountModel, "", "", cfg, stats)

	for i := 0; i < 10; i++ {
		_, _, err := fm.Subscribe(types.BDNBlocksFeed, nil, sdnmessage.AccountTier(sdnmessage.ATierEnterprise), types.AccountID("gw"), "", "", "", "")
		assert.Nil(t, err)
	}
	_, _, err = fm.Subscribe(types.BDNBlocksFeed, nil, sdnmessage.AccountTier(sdnmessage.ATierEnterprise), types.AccountID("gw"), "", "", "", "")
	assert.Nil(t, err)

}
