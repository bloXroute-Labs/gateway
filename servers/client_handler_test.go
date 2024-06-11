package servers

import (
	"context"
	"encoding/json"
	"fmt"
	"math/big"
	"net/http"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/jsonrpc"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/google/uuid"
	"github.com/sourcegraph/jsonrpc2"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth/test"
	"github.com/bloXroute-Labs/gateway/v2/config"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/services/statistics"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
)

// newHeadsResponseParams - response of the jsonrpc params
type newHeadsResponseParams struct {
	Subscription string              `json:"subscription"`
	Result       types.NewHeadsBlock `json:"result"`
}

var accountIDToAccountModel = map[types.AccountID]sdnmessage.Account{
	"a": {AccountInfo: sdnmessage.AccountInfo{AccountID: "a", TierName: sdnmessage.ATierElite}, SecretHash: "123456"},
	"b": {AccountInfo: sdnmessage.AccountInfo{AccountID: "b", TierName: sdnmessage.ATierDeveloper}, SecretHash: "7891011"},
	"c": {AccountInfo: sdnmessage.AccountInfo{AccountID: "c", TierName: sdnmessage.ATierElite}},
	"i": {AccountInfo: sdnmessage.AccountInfo{AccountID: "i", TierName: sdnmessage.ATierIntroductory}, SecretHash: "654321"},
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

func mockAuthorize(accountID types.AccountID, _ string, _, _ bool, _ string) (sdnmessage.Account, error) {
	return getMockCustomerAccountModel(accountID)
}

func getMockCustomerAccountModel(accountID types.AccountID) (sdnmessage.Account, error) {
	var err error
	if accountID == "d" {
		err = fmt.Errorf("Timeout error")
	}
	return accountIDToAccountModel[accountID], err
}

func getMockQuotaUsage(accountID string) (*connections.QuotaResponseBody, error) {
	res := connections.QuotaResponseBody{
		AccountID:   accountID,
		QuotaFilled: 1,
		QuotaLimit:  2,
	}

	return &res, nil
}

func reset(fm *FeedManager, wsURL string, blockchainPeers []types.NodeEndpoint) *websocket.Conn {
	fm.CloseAllClientConnections()
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
	clearWSProviderStats(fm, blockchainPeers)
	return newWSConn(wsURL)
}

func clearWSProviderStats(fm *FeedManager, blockchainPeers []types.NodeEndpoint) {
	for _, wsProvider := range fm.nodeWSManager.Providers() {
		wsProvider.(*eth.MockWSProvider).ResetCounters()
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

	fm := NewFeedManager(context.Background(), g, feedChan, nil, services.NewNoOpSubscriptionServices(),
		types.NetworkNum(1), 1, types.NodeID("nodeID"),
		eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout, false),
		gwAccount, getMockCustomerAccountModel, "", "", cfg, stats, nil, nil)
	providers := fm.nodeWSManager.Providers()
	p1 := providers[blockchainPeers[0].IPPort()]
	assert.NotNil(t, p1)
	p2 := providers[blockchainPeers[1].IPPort()]
	assert.NotNil(t, p2)
	p3 := providers[blockchainPeers[2].IPPort()]
	assert.NotNil(t, p3)

	var group errgroup.Group
	sourceFromNode := false
	im := services.NewIntentsManager()
	wsServer := NewWSServer(fm, im, getMockQuotaUsage, true, &sourceFromNode, mockAuthorize, true, false)
	clientHandler := NewClientHandler(fm, im, wsServer, NewHTTPServer(fm, cfg.HTTPPort), nil, true, nil, &sourceFromNode, mockAuthorize, true, false)
	go clientHandler.ManageServers(context.Background(), cfg.ManageWSServer)
	group.Go(func() error {
		return fm.Start(context.Background())
	})

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
		handleError(t, ws, nil)

		// fail for secret hash
		dummyAuthHeader = "Yzo3ODkxMDEx" //c:7891011
		headers.Set("Authorization", dummyAuthHeader)
		ws, _, err = dialer.Dial(wsURL, headers)
		handleError(t, ws, nil)

		// pass for timeout - account should set to elite
		dummyAuthHeader = "ZDo3ODkxMDEx" // d:7891011
		headers.Set("Authorization", dummyAuthHeader)
		ws, _, err = dialer.Dial(wsURL, headers)
		handleError(t, ws, nil)

		// pass - same account for server and client
		dummyAuthHeader = "Z3c6c2VjcmV0" //gw:secret
		headers.Set("Authorization", dummyAuthHeader)
		ws, _, err = dialer.Dial(wsURL, headers)
		assert.NoError(t, err)

		t.Run(fmt.Sprintf("wsClient-%s", wsURL), func(t *testing.T) {
			handlePingRequest(t, ws)
			handleBlxrTxEnsureNodeValidation(t, fm, ws)
			handleBlxrTxRequestLegacyTx(t, ws)
			handleBlxrTxsRequestLegacyTx(t, ws)
			handleBlxrTxRequestAccessListTx(t, ws)
			handleBlxrTxRequestDynamicFeeTx(t, ws)
			handleBlxrTxRequestTxWithPrefix(t, ws)
			handleBlxrTxRequestWithNextValidator(t, ws)
			handleBlxrTxRequestRLPTx(t, ws)
			handleBlxrTxWithWrongChainID(t, ws)
			handleNonBloxrouteRPCMethods(t, fm, ws, blockchainPeers)
			handleNonBloxrouteSendTxMethod(t, fm, ws, blockchainPeers)
			handleSubmitIntent(t, fm, ws)
			handleSubscribe(t, fm, ws)
			handleIntentsSubscribe(t, fm, ws)
			handleEthSubscribe(t, fm, ws, blockchainPeers)
			handleTxReceiptsSubscribe(t, fm, ws)
			handleInvalidSubscribe(t, ws)
			testWSShutdown(t, fm, ws, blockchainPeers)
		})
		// restart bc last test shut down ws server
		im := services.NewIntentsManager()
		fm = NewFeedManager(context.Background(), g, make(chan types.Notification), nil, services.NewNoOpSubscriptionServices(), types.NetworkNum(1), 1, types.NodeID("nodeID"), eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout, false), gwAccount, getMockCustomerAccountModel, "", "", cfg, stats, nil, nil)
		clientHandler := NewClientHandler(fm, im, wsServer, NewHTTPServer(fm, cfg.HTTPPort), nil, true, getMockQuotaUsage, &sourceFromNode, mockAuthorize, true, false)
		go clientHandler.ManageServers(context.Background(), cfg.ManageWSServer)
		group.Go(func() error {
			return fm.Start(context.Background())
		})
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
	assert.NoError(t, err)
	assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": [], "multiTxs": true}]}`)
	_, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`)

	ticker := time.NewTicker(time.Millisecond)
	timer := time.NewTimer(time.Millisecond * 10)
loop1:
	for {
		select {
		case <-ticker.C:
			if fm.SubscriptionExists(subscriptionID) {
				break loop1
			}
		case <-timer.C:
			assert.True(t, fm.SubscriptionExists(subscriptionID))
		}
	}

	ticker.Stop()
	timer.Stop()

	handlePingRequest(t, ws)

	ws.Close()
	time.Sleep(3 * time.Millisecond)

	ticker = time.NewTicker(time.Millisecond)
	timer = time.NewTimer(time.Millisecond * 10)
loop2:
	for {
		select {
		case <-ticker.C:
			if !fm.SubscriptionExists(subscriptionID) {
				break loop2
			}
		case <-timer.C:
			assert.False(t, fm.SubscriptionExists(subscriptionID))
		}
	}

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

func TestClientHandler_BSC(t *testing.T) {
	g := bxmock.MockBxListener{}
	stats := statistics.NoStats{}
	feedChan := make(chan types.Notification)

	gwAccount, _ := getMockCustomerAccountModel("gw")

	// BSC configuration
	urlBSC := "127.0.0.1:28333"
	cfgBSC := config.Bx{WebsocketPort: 28333, ManageWSServer: true, WebsocketTLSEnabled: false}
	BscWsURLs := fmt.Sprintf("ws://%s/ws", urlBSC)
	blockchainPeersBSC, blockchainPeersInfoBSC := test.GenerateBlockchainPeersInfo(1)

	var group errgroup.Group
	sourceFromNode := false
	im := services.NewIntentsManager()
	fmBSC := NewFeedManager(context.Background(), g, feedChan, nil, services.NewNoOpSubscriptionServices(), types.NetworkNum(1), 56, types.NodeID("nodeID"), eth.NewEthWSManager(blockchainPeersInfoBSC, eth.NewMockWSProvider, bxgateway.WSProviderTimeout, false), gwAccount, getMockCustomerAccountModel, "", "", cfgBSC, stats, nil, nil)
	wsServer := NewWSServer(fmBSC, im, getMockQuotaUsage, true, &sourceFromNode, mockAuthorize, true, false)
	clientHandlerBSC := NewClientHandler(fmBSC, im, wsServer, NewHTTPServer(fmBSC, cfgBSC.HTTPPort), nil, false, getMockQuotaUsage, &sourceFromNode, mockAuthorize, true, false)
	go clientHandlerBSC.ManageServers(context.Background(), false)
	group.Go(func() error {
		return fmBSC.Start(context.Background())
	})

	time.Sleep(10 * time.Millisecond)

	dialer := websocket.DefaultDialer
	headers := make(http.Header)

	markAllPeersWithSyncStatus(fmBSC, blockchainPeersBSC, blockchain.Synced)
	dummyAuthHeader := "Z3c6c2VjcmV0" //gw:secret
	headers.Set("Authorization", dummyAuthHeader)
	wsBSC, _, err := dialer.Dial(BscWsURLs, headers)
	assert.NoError(t, err)

	t.Run(fmt.Sprintf("wsClient-%s", urlBSC), func(t *testing.T) {
		handleBscBlxrTxRequestWithNextValidator(t, wsBSC)
		// handleNonBloxrouteRPCMethodsDisabled(t, fmBSC, wsBSC, blockchainPeersBSC)
	})
}

func TestClientHandlerAuth(t *testing.T) {
	for _, allowIntroductoryTierAccess := range []bool{true, false} {
		t.Run(fmt.Sprintf("allowIntroductoryTierAccess-%t", allowIntroductoryTierAccess), func(t *testing.T) {
			ctx, cancel := context.WithCancel(context.Background())

			g := bxmock.MockBxListener{}
			stats := statistics.NoStats{}
			feedChan := make(chan types.Notification)
			url := "ws://127.0.0.1:28334/ws"
			gwAccount, _ := getMockCustomerAccountModel("gw")
			cfg := config.Bx{WebsocketPort: 28334, ManageWSServer: true, WebsocketTLSEnabled: false}
			blockchainPeers, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)

			fm := NewFeedManager(context.Background(), g, feedChan, nil, services.NewNoOpSubscriptionServices(),
				types.NetworkNum(1), 1, types.NodeID("nodeID"),
				eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout, false),
				gwAccount, getMockCustomerAccountModel, "", "", cfg, stats, nil, nil)

			var group errgroup.Group
			sourceFromNode := false
			im := services.NewIntentsManager()
			wsServer := NewWSServer(fm, im, getMockQuotaUsage, false, &sourceFromNode, mockAuthorize, true, allowIntroductoryTierAccess)
			clientHandler := NewClientHandler(fm, im, wsServer, NewHTTPServer(fm, cfg.HTTPPort), nil, false, nil, &sourceFromNode, mockAuthorize, true, allowIntroductoryTierAccess)

			group.Go(func() error {
				return clientHandler.ManageServers(ctx, cfg.ManageWSServer)
			})
			group.Go(func() error {
				return fm.Start(ctx)
			})

			markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
			time.Sleep(10 * time.Millisecond)

			dialer := websocket.DefaultDialer
			headers := make(http.Header)

			dummyAuthHeader := "aTo2NTQzMjE=" // i:654321
			headers.Set("Authorization", dummyAuthHeader)
			ws, _, err := dialer.Dial(url, headers)
			require.NoError(t, err)
			handleError(t, ws, nil)

			reqPayload := fmt.Sprintf(`{"jsonrpc": "2.0", "id": "1", "method": "eth_sendRawTransaction", "params": ["0x%v"]}`, fixtures.DynamicFeeTransaction)

			var msg []byte
			if allowIntroductoryTierAccess {
				msg = writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
				assert.Equal(t, `{"id":"1","error":{"code":-32001,"message":"Insufficient quota","data":"account must be enterprise / enterprise elite / ultra"},"jsonrpc":"2.0"}
`, string(msg))
				err = ws.WriteMessage(websocket.TextMessage, msg)
				assert.NoError(t, err)
				_, _, err = ws.ReadMessage()
				assert.Error(t, err, "connection should be closed by the server")
			} else {
				msg = writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
				assert.Equal(t, `{"id":"1","error":{"code":-32601,"message":"Invalid method","data":"got unsupported method name: eth_sendRawTransaction"},"jsonrpc":"2.0"}
`, string(msg))
				// client should be still connected
				msg = writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
				assert.Equal(t, `{"id":"1","error":{"code":-32601,"message":"Invalid method","data":"got unsupported method name: eth_sendRawTransaction"},"jsonrpc":"2.0"}
`, string(msg))
			}

			cancel()

			err = group.Wait()
			require.NoError(t, err)
		})
	}
}

// TODO: Follow work to handle sync and unsync on another PR
// func TestHandleClient_Notification(t *testing.T) {
//	g := bxmock.MockBxListener{}
//	stats := statistics.NoStats{}
//	wsFeed := make(chan types.Notification)
//	url := "127.0.0.1:28332"
//	wsURLs := []string{fmt.Sprintf("ws://%s/ws", url), fmt.Sprintf("ws://%s/", url)}
//
//	gwAccount, _ := getMockCustomerAccountModel("gw")
//	cfg := config.Bx{WebsocketPort: 28332, ManageWSServer: true, WebsocketTLSEnabled: false}
//
//	blockchainPeers, blockchainPeersInfo := test.GenerateBlockchainPeersInfo(3)
//	fm := NewFeedManager(context.Background(), g, wsFeed, types.NetworkNum(1), eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout), gwAccount, getMockCustomerAccountModel, "", "", cfg, stats)
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
//				fm = NewFeedManager(context.Background(), g, wsFeed, types.NetworkNum(1), eth.NewEthWSManager(blockchainPeersInfo, eth.NewMockWSProvider, bxgateway.WSProviderTimeout), gwAccount, getMockCustomerAccountModel, "", "", cfg, stats)
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
// }

func handleSubscribe(t *testing.T, fm *FeedManager, ws *websocket.Conn) {
	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`)
	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleEthSubscribe(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	wsProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, wsProvider.BlockchainPeerEndpoint(), blockchainPeers[0])

	unsubscribeFilter, subscriptionID := assertEthSubscribe(t, ws, fm, `{"id": "1", "method": "eth_subscribe", "params": ["newHeads"]}`)
	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	feedNotification, _ := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil, false)
	feedNotification.SetNotificationType(types.NewBlocksFeed)
	sourceEndpoint := types.NodeEndpoint{IP: blockchainPeers[0].IP, Port: blockchainPeers[0].Port, BlockchainNetwork: bxgateway.Mainnet}
	feedNotification.SetSource(&sourceEndpoint)
	assert.True(t, fm.nodeWSManager.Synced())

	fm.feed <- feedNotification
	time.Sleep(time.Millisecond)

	for i := 0; i < 1; i++ {
		_, message, err := ws.ReadMessage()
		assert.NoError(t, err)
		var req jsonrpc2.Request
		err = json.Unmarshal(message, &req)
		var m newHeadsResponseParams
		err = json.Unmarshal(*req.Params, &m)
		assert.NoError(t, err)
		assert.Equal(t, m.Result.BlockHash.String(), ethBlock.Hash().String())
	}

	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleIntentsSubscribe(t *testing.T, fm *FeedManager, ws *websocket.Conn) {
	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{ "id": "2", "method": "subscribe", "params": [ "userIntentSolutionsFeed", { "dapp_address": "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642", "hash": [183,2,62,89,100,47,176,195,131,57,253,105,100,82,169,64,29,188,156,99,54,36,235,232,173,157,89,149,61,184,76,66], "signature": [219,205,179,107,60,59,38,125,46,45,156,10,134,243,162,72,64,73,94,242,204,254,129,50,70,146,3,160,193,121,120,157,15,160,110,100,156,30,241,77,145,65,209,160,191,14,68,103,79,163,72,155,116,2,11,23,11,167,240,4,119,184,40,176,0]}]}`)

	solution := &types.UserIntentSolution{
		ID:            uuid.New().String(),
		SolverAddress: "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642",
		DappAddress:   "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642",
		IntentID:      uuid.New().String(),
		Solution:      []byte{71, 108, 111, 114, 121, 32, 116, 111, 32, 85, 107, 114, 97, 105, 110, 101, 33},
		Hash:          []byte{183, 2, 62, 89, 100, 47, 176, 195, 131, 57, 253, 105, 100, 82, 169, 64, 29, 188, 156, 99, 54, 36, 235, 232, 173, 157, 89, 149, 61, 184, 76, 66},
		Signature:     []byte{219, 205, 179, 107, 60, 59, 38, 125, 46, 45, 156, 10, 134, 243, 162, 72, 64, 73, 94, 242, 204, 254, 129, 50, 70, 146, 3, 160, 193, 121, 120, 157, 15, 160, 110, 100, 156, 30, 241, 77, 145, 65, 209, 160, 191, 14, 68, 103, 79, 163, 72, 155, 116, 2, 11, 23, 11, 167, 240, 4, 119, 184, 40, 176, 0},
		Timestamp:     time.Now(),
	}

	fm.feed <- types.NewUserIntentSolutionNotification(solution)
	time.Sleep(time.Millisecond)

	_, message, err := ws.ReadMessage()
	require.NoError(t, err)
	var req jsonrpc2.Request
	err = json.Unmarshal(message, &req)
	require.NoError(t, err)

	var m userIntentSolutionResponse
	err = json.Unmarshal(*req.Params, &m)
	assert.NoError(t, err)
	assert.Equal(t, m.Subscription, subscriptionID)
	assert.Equal(t, m.Result.IntentID, solution.ID)
	assert.Equal(t, m.Result.IntentSolution, solution.Solution)

	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleSubmitIntent(t *testing.T, fm *FeedManager, ws *websocket.Conn) {
	reqParams := &jsonrpc.RPCSubmitIntentPayload{
		DappAddress:   "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642",
		SenderAddress: "0x097399a35cfC20efE5FcD2e9b1d892884DAAd642",
		Intent:        []byte{71, 108, 111, 114, 121, 32, 116, 111, 32, 85, 107, 114, 97, 105, 110, 101, 33},
		Hash:          []byte{183, 2, 62, 89, 100, 47, 176, 195, 131, 57, 253, 105, 100, 82, 169, 64, 29, 188, 156, 99, 54, 36, 235, 232, 173, 157, 89, 149, 61, 184, 76, 66},
		Signature:     []byte{219, 205, 179, 107, 60, 59, 38, 125, 46, 45, 156, 10, 134, 243, 162, 72, 64, 73, 94, 242, 204, 254, 129, 50, 70, 146, 3, 160, 193, 121, 120, 157, 15, 160, 110, 100, 156, 30, 241, 77, 145, 65, 209, 160, 191, 14, 68, 103, 79, 163, 72, 155, 116, 2, 11, 23, 11, 167, 240, 4, 119, 184, 40, 176, 0},
	}
	intent, err := json.Marshal(reqParams)
	require.NoError(t, err)
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_submit_intent", "params": %s}`, string(intent))
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	assert.Nil(t, clientRes.Error)
	res := parseSubmitInternalTxResult(t, clientRes.Result)
	_, err = uuid.Parse(res.IntentID)
	require.NoError(t, err)
}

func handleInvalidSubscribe(t *testing.T, ws *websocket.Conn) {
	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "subscribe", "para": ["txReceipts", {"include": []}]}`), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	assert.Nil(t, clientRes.Result)
	time.Sleep(time.Millisecond)
	handlePingRequest(t, ws)
}

func handleTxReceiptsSubscribe(t *testing.T, fm *FeedManager, ws *websocket.Conn) {
	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["txReceipts", {"include": []}]}`)
	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleTxReceiptsNotification(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	bridge := blockchain.NewBxBridge(eth.Converter{}, false)
	wsProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, wsProvider.BlockchainPeerEndpoint(), blockchainPeers[0])

	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["txReceipts", {"include": []}]}`)

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(10000)
	bxBlock, _ := bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, td))
	numTx := len(bxBlock.Txs)
	feedNotification, _ := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil, false)
	feedNotification.SetNotificationType(types.TxReceiptsFeed)
	sourceEndpoint := types.NodeEndpoint{IP: blockchainPeers[0].IP, Port: blockchainPeers[0].Port}
	feedNotification.SetSource(&sourceEndpoint)
	assert.True(t, fm.nodeWSManager.Synced())

	fm.feed <- feedNotification
	time.Sleep(5 * time.Millisecond)
	assert.Equal(t, numTx, wsProvider.(*eth.MockWSProvider).NumReceiptsFetched)

	// expect receipt notifications
	for i := 0; i < numTx; i++ {
		_, message, err := ws.ReadMessage()
		assert.NoError(t, err)
		var m json.RawMessage
		err = json.Unmarshal(message, &m)
		assert.NoError(t, err)
	}

	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleNonBloxrouteRPCMethodsDisabled(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	clearWSProviderStats(fm, blockchainPeers)
	assert.True(t, fm.nodeWSManager.Synced())
	ws1, _ := fm.nodeWSManager.Provider(&blockchainPeers[0])
	ws2, _ := fm.nodeWSManager.Provider(&blockchainPeers[1])
	ws3, _ := fm.nodeWSManager.Provider(&blockchainPeers[2])
	assert.Equal(t, blockchain.Synced, ws1.SyncStatus())
	ws2.UpdateSyncStatus(blockchain.Unsynced)
	ws3.UpdateSyncStatus(blockchain.Unsynced)

	request := `{"jsonrpc": "2.0", "id": "1", "method": "eth_getBalance", "params": ["0xAABCf4f110F06aFd82A7696f4fb79AE4a41D0f81", "latest"]}`
	response := writeMsgToWsAndReadResponse(t, ws, []byte(request), nil)
	clientRes := getClientResponse(t, response)
	assert.Equal(t, 0, ws1.(*eth.MockWSProvider).NumRPCCalls())
	assert.NotNil(t, clientRes.Error)
	assert.Equal(t, clientRes.Error.(map[string]interface{})["data"], "got unsupported method name: eth_getBalance")
	assert.Equal(t, clientRes.Error.(map[string]interface{})["code"], float64(jsonrpc.MethodNotFound))
	assert.Equal(t, clientRes.Error.(map[string]interface{})["message"], "Invalid method")
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
}

func handleNonBloxrouteRPCMethods(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	clearWSProviderStats(fm, blockchainPeers)
	assert.True(t, fm.nodeWSManager.Synced())
	ws1, _ := fm.nodeWSManager.Provider(&blockchainPeers[0])
	ws2, _ := fm.nodeWSManager.Provider(&blockchainPeers[1])
	ws3, _ := fm.nodeWSManager.Provider(&blockchainPeers[2])
	assert.Equal(t, blockchain.Synced, ws1.SyncStatus())
	ws2.UpdateSyncStatus(blockchain.Unsynced)
	ws3.UpdateSyncStatus(blockchain.Unsynced)

	request := `{"jsonrpc": "2.0", "id": "1", "method": "eth_getBalance", "params": ["0xAABCf4f110F06aFd82A7696f4fb79AE4a41D0f81", "latest"]}`
	response := writeMsgToWsAndReadResponse(t, ws, []byte(request), nil)
	clientRes := getClientResponse(t, response)
	assert.Equal(t, 1, ws1.(*eth.MockWSProvider).NumRPCCalls())
	assert.Equal(t, "response", clientRes.Result)
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
}

func handleNonBloxrouteSendTxMethod(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	assert.True(t, fm.nodeWSManager.Synced())
	ws1, _ := fm.nodeWSManager.Provider(&blockchainPeers[0])
	ws2, _ := fm.nodeWSManager.Provider(&blockchainPeers[1])
	ws3, _ := fm.nodeWSManager.Provider(&blockchainPeers[2])
	assert.Equal(t, blockchain.Synced, ws1.SyncStatus())
	ws2.UpdateSyncStatus(blockchain.Unsynced)
	ws3.UpdateSyncStatus(blockchain.Unsynced)

	request := fmt.Sprintf(`{"jsonrpc": "2.0", "id": "1", "method": "eth_sendRawTransaction", "params": ["0x%v"]}`, fixtures.DynamicFeeTransaction)
	response := writeMsgToWsAndReadResponse(t, ws, []byte(request), nil)
	clientRes := getClientResponse(t, response)
	hashRes := clientRes.Result.(string)
	assert.Equal(t, "0x"+fixtures.DynamicFeeTransactionHash[2:], hashRes)
	assert.Nil(t, clientRes.Error)
	markAllPeersWithSyncStatus(fm, blockchainPeers, blockchain.Synced)
}

func handleOnBlockNotification(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	wsProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, wsProvider.BlockchainPeerEndpoint(), blockchainPeers[0])

	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["ethOnBlock", {"include": [], "call-params":  [{"method": "eth_blockNumber", "name": "height"}] }]}`)

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	feedNotification, _ := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil, false)
	feedNotification.SetNotificationType(types.OnBlockFeed)
	sourceEndpoint := types.NodeEndpoint{IP: blockchainPeers[0].IP, Port: blockchainPeers[0].Port, BlockchainNetwork: bxgateway.Mainnet}
	feedNotification.SetSource(&sourceEndpoint)
	assert.True(t, fm.nodeWSManager.Synced())

	fm.feed <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, 1, wsProvider.(*eth.MockWSProvider).NumRPCCalls)

	// expect onBlock notification and TaskCompletedEvent
	for i := 0; i < 2; i++ {
		_, message, err := ws.ReadMessage()
		assert.NoError(t, err)
		var m json.RawMessage
		err = json.Unmarshal(message, &m)
		assert.NoError(t, err)
	}

	writeMsgToWsAndReadResponse(t, ws, []byte(unsubscribeFilter), nil)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.SubscriptionExists(subscriptionID))
	handlePingRequest(t, ws)
}

func handleTxReceiptsNotificationRequestedUnsynced(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	clearWSProviderStats(fm, blockchainPeers)
	bridge := blockchain.NewBxBridge(eth.Converter{}, false)
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
	feedNotification, _ := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil, false)
	feedNotification.SetNotificationType(types.TxReceiptsFeed)
	sourceEndpoint := types.NodeEndpoint{IP: blockchainPeers[0].IP, Port: blockchainPeers[0].Port, BlockchainNetwork: bxgateway.Mainnet}
	feedNotification.SetSource(&sourceEndpoint)

	fm.nodeWSManager.UpdateNodeSyncStatus(requestedUnsyncedWSProvider.BlockchainPeerEndpoint(), blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[1], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.True(t, fm.nodeWSManager.Synced())

	fm.feed <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, numTx, expectedSyncedWSProvider.(*eth.MockWSProvider).NumReceiptsFetched)
	assert.Equal(t, 0, requestedUnsyncedWSProvider.(*eth.MockWSProvider).NumReceiptsFetched)

	// expect receipt notifications
	for i := 0; i < numTx; i++ {
		_, message, err := ws.ReadMessage()
		assert.NoError(t, err)
		var m json.RawMessage
		err = json.Unmarshal(message, &m)
		assert.NoError(t, err)
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

	requestedUnsyncedWSProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[0])
	assert.True(t, ok)
	assert.Equal(t, requestedUnsyncedWSProvider.BlockchainPeerEndpoint(), blockchainPeers[0])
	expectedSyncedWSProvider, ok := fm.nodeWSManager.Provider(&blockchainPeers[2])
	assert.True(t, ok)
	assert.Equal(t, expectedSyncedWSProvider.BlockchainPeerEndpoint(), blockchainPeers[2])

	unsubscribeFilter, subscriptionID := assertSubscribe(t, ws, fm, `{"id": "1", "method": "subscribe", "params": ["ethOnBlock", {"include": [], "call-params":  [{"method": "eth_blockNumber", "name": "height"}] }]}`)

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	feedNotification, _ := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil, false)
	feedNotification.SetNotificationType(types.OnBlockFeed)
	sourceEndpoint := types.NodeEndpoint{IP: blockchainPeers[0].IP, Port: blockchainPeers[0].Port, BlockchainNetwork: bxgateway.Mainnet}
	feedNotification.SetSource(&sourceEndpoint)

	fm.nodeWSManager.UpdateNodeSyncStatus(requestedUnsyncedWSProvider.BlockchainPeerEndpoint(), blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[1], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.True(t, fm.nodeWSManager.Synced())

	fm.feed <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, 1, expectedSyncedWSProvider.(*eth.MockWSProvider).NumRPCCalls())
	assert.Equal(t, 0, requestedUnsyncedWSProvider.(*eth.MockWSProvider).NumRPCCalls())

	// expect onBlock notification and TaskCompletedEvent
	for i := 0; i < 2; i++ {
		_, message, err := ws.ReadMessage()
		assert.NoError(t, err)
		var m json.RawMessage
		err = json.Unmarshal(message, &m)
		assert.NoError(t, err)
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
	subscriptionID := fmt.Sprintf("%v", clientRes.Result)
	assert.True(t, fm.SubscriptionExists(subscriptionID))

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	feedNotification, err := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil, false)
	assert.NoError(t, err)
	feedNotification.SetNotificationType(types.TxReceiptsFeed)
	sourceEndpoint := types.NodeEndpoint{IP: blockchainPeers[0].IP, Port: blockchainPeers[0].Port, BlockchainNetwork: bxgateway.Mainnet}
	feedNotification.SetSource(&sourceEndpoint)

	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[0], blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[1], blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[2], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.nodeWSManager.Synced())
	assert.False(t, fm.SubscriptionExists(subscriptionID))

	fm.feed <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, 0, ws0.(*eth.MockWSProvider).NumReceiptsFetched)
	assert.Equal(t, 0, ws1.(*eth.MockWSProvider).NumReceiptsFetched)
	assert.Equal(t, 0, ws2.(*eth.MockWSProvider).NumReceiptsFetched)
}

func handleOnBlockNotificationNoneSynced(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
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
	subscriptionID := fmt.Sprintf("%v", clientRes.Result)
	assert.True(t, fm.SubscriptionExists(subscriptionID))

	ethBlock := bxmock.NewEthBlock(10, common.Hash{})
	feedNotification, err := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil, false)
	assert.NoError(t, err)
	feedNotification.SetNotificationType(types.OnBlockFeed)
	sourceEndpoint := types.NodeEndpoint{IP: blockchainPeers[0].IP, Port: blockchainPeers[0].Port, BlockchainNetwork: bxgateway.Mainnet}
	feedNotification.SetSource(&sourceEndpoint)

	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[0], blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[1], blockchain.Unsynced)
	fm.nodeWSManager.UpdateNodeSyncStatus(blockchainPeers[2], blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	assert.False(t, fm.nodeWSManager.Synced())
	assert.False(t, fm.SubscriptionExists(subscriptionID))

	fm.feed <- feedNotification
	time.Sleep(time.Millisecond)
	assert.Equal(t, 0, ws0.(*eth.MockWSProvider).NumRPCCalls)
	assert.Equal(t, 0, ws1.(*eth.MockWSProvider).NumRPCCalls)
	assert.Equal(t, 0, ws2.(*eth.MockWSProvider).NumRPCCalls)
}

func testWSShutdown(t *testing.T, fm *FeedManager, ws *websocket.Conn, blockchainPeers []types.NodeEndpoint) {
	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	subscriptionID := fmt.Sprintf("%v", clientRes.Result)
	assert.True(t, fm.SubscriptionExists(subscriptionID))

	subscribeMsg2 := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "2", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash","tx_contents.gas_price"]}]}`), nil)
	clientRes2 := getClientResponse(t, subscribeMsg2)
	subscriptionID2 := fmt.Sprintf("%v", clientRes2.Result)
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
	assert.NoError(t, err)
	assert.True(t, timeClientReceivesResponse.After(timeServerReceivesRequest))
	assert.True(t, timeServerReceivesRequest.After(timeClientSendsRequest))
}

func handleQuotaUsageRequest(t *testing.T, ws *websocket.Conn) {
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "quota_usage"}`), nil)
	clientRes := getClientResponse(t, msg)
	res := parseQuotaUsage(t, clientRes.Result)
	assert.Nil(t, "gw", res.AccountID)
	assert.Nil(t, 1, res.QuotaFilled)
	assert.Nil(t, 2, res.QuotaLimit)
}

func handleError(t *testing.T, ws *websocket.Conn, closeError *websocket.CloseError) {
	_ = writeMsgToWsAndReadResponse(t, ws, []byte(`{"id": "1", "method": "ping"}`), closeError)
}

func handleBlxrTxEnsureNodeValidation(t *testing.T, fm *FeedManager, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s", "node_validation": true}}`, fixtures.LegacyTransaction)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	res := parseBlxrTxResult(t, clientRes.Result)
	assert.Equal(t, fixtures.LegacyTransactionHash[2:], res.TxHash)
	assert.Nil(t, clientRes.Error)

	var txSent []string
	for _, wsProvider := range fm.nodeWSManager.Providers() {
		for _, tx := range wsProvider.(*eth.MockWSProvider).TxSent {
			txSent = append(txSent, tx)
		}
		wsProvider.(*eth.MockWSProvider).TxSent = []string{}
	}
	assert.Equal(t, 1, len(txSent))
	assert.Equal(t, "0x"+fixtures.LegacyTransaction, txSent[0])
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

func handleBlxrTxRequestWithNextValidator(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s", "next_validator":true}}}`, "0x"+fixtures.LegacyTransaction)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	assert.NotNil(t, clientRes.Error)
}

func handleBscBlxrTxRequestWithNextValidator(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s", "next_validator":true}}}`, "0x"+fixtures.LegacyTransactionBSC)
	msg := writeMsgToWsAndReadResponse(t, ws, []byte(reqPayload), nil)
	clientRes := getClientResponse(t, msg)
	assert.NotNil(t, clientRes.Error)
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
	if err != nil {
		assert.NoError(t, err)
	}
	return res
}

func parsePingResult(t *testing.T, rpcResponse interface{}) (pr rpcPingResponse) {
	res := rpcPingResponse{}
	b, err := json.Marshal(rpcResponse)
	assert.NoError(t, err)
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	return res
}

func parseQuotaUsage(t *testing.T, rpcResponse interface{}) (qr connections.QuotaResponseBody) {
	res := connections.QuotaResponseBody{
		AccountID:   "account-id",
		QuotaFilled: 1,
		QuotaLimit:  2,
	}
	b, err := json.Marshal(rpcResponse)
	assert.NoError(t, err)
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	return res
}

func parseBlxrTxResult(t *testing.T, rpcResponse interface{}) (tr rpcTxResponse) {
	res := rpcTxResponse{}
	b, err := json.Marshal(rpcResponse)
	assert.NoError(t, err)
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	return res
}

func parseBlxrTxsResult(t *testing.T, rpcResponse interface{}) (tr rpcBatchTxResponse) {
	res := rpcBatchTxResponse{}
	b, err := json.Marshal(rpcResponse)
	assert.NoError(t, err)
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	return res
}

func parseSubmitInternalTxResult(t *testing.T, rpcResponse interface{}) (tr rpcIntentResponse) {
	res := rpcIntentResponse{}
	b, err := json.Marshal(rpcResponse)
	assert.NoError(t, err)
	err = json.Unmarshal(b, &res)
	assert.NoError(t, err)
	return res
}

func writeMsgToWsAndReadResponse(t *testing.T, conn *websocket.Conn, msg []byte, expectedErr *websocket.CloseError) (response []byte) {
	err := conn.WriteMessage(websocket.TextMessage, msg)
	assert.NoError(t, err)
	_, response, err = conn.ReadMessage()
	assert.True(t, (expectedErr == nil && err == nil) || (expectedErr != nil && err != nil))
	return response
}

func assertSubscribe(t *testing.T, ws *websocket.Conn, fm *FeedManager, filter string) (string, string) {
	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(filter), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	subscriptionID := fmt.Sprintf("%v", clientRes.Result)
	assert.True(t, fm.SubscriptionExists(subscriptionID))
	return fmt.Sprintf(
		`{"id": 1, "method": "unsubscribe", "params": ["%v"]}`,
		subscriptionID,
	), subscriptionID
}

func assertEthSubscribe(t *testing.T, ws *websocket.Conn, fm *FeedManager, filter string) (string, string) {
	subscribeMsg := writeMsgToWsAndReadResponse(t, ws, []byte(filter), nil)
	clientRes := getClientResponse(t, subscribeMsg)
	subscriptionID := fmt.Sprintf("%v", clientRes.Result)
	assert.True(t, fm.SubscriptionExists(subscriptionID))
	return fmt.Sprintf(
		`{"id": 1, "method": "eth_unsubscribe", "params": ["%v"]}`,
		subscriptionID,
	), subscriptionID
}

func TestIsFiltersSupportedByTxType(t *testing.T) {
	tests := []struct {
		name     string
		txType   uint8
		filters  []string
		expected bool
	}{
		{
			name:     "DynamicFeeTxType with gas_price filter",
			txType:   ethtypes.DynamicFeeTxType,
			filters:  []string{"gas_price"},
			expected: false,
		},
		{
			name:     "DynamicFeeTxType with gas_price and max_fee_per_gas filters",
			txType:   ethtypes.DynamicFeeTxType,
			filters:  []string{"gas_price", "max_fee_per_gas"},
			expected: true,
		},
		{
			name:     "DynamicFeeTxType with gas_price and max_priority_fee_per_gas filters",
			txType:   ethtypes.DynamicFeeTxType,
			filters:  []string{"gas_price", "max_priority_fee_per_gas"},
			expected: true,
		},
		{
			name:     "DynamicFeeTxType with gas_price and max_priority_fee_per_gas filters",
			txType:   ethtypes.DynamicFeeTxType,
			filters:  []string{"gas_price", "max_fee_per_gas", "max_priority_fee_per_gas"},
			expected: true,
		},
		{
			name:     "BlobTxType with gas_price filter",
			txType:   ethtypes.BlobTxType,
			filters:  []string{"gas_price"},
			expected: false,
		},
		{
			name:     "BlobTxType with gas_price and max_fee_per_gas filters",
			txType:   ethtypes.BlobTxType,
			filters:  []string{"gas_price", "max_fee_per_gas"},
			expected: true,
		},
		{
			name:     "BlobTxType with gas_price and max_priority_fee_per_gas filters",
			txType:   ethtypes.BlobTxType,
			filters:  []string{"gas_price", "max_priority_fee_per_gas"},
			expected: true,
		},
		{
			name:     "BlobTxType with gas_price and max_priority_fee_per_gas filters",
			txType:   ethtypes.BlobTxType,
			filters:  []string{"gas_price", "max_fee_per_gas", "max_priority_fee_per_gas"},
			expected: true,
		},
		{
			name:     "Non-DynamicFeeTxType with max_fee_per_gas filter",
			txType:   ethtypes.LegacyTxType,
			filters:  []string{"max_fee_per_gas"},
			expected: false,
		},
		{
			name:     "Non-DynamicFeeTxType with max_priority_fee_per_gas filter",
			txType:   ethtypes.LegacyTxType,
			filters:  []string{"max_priority_fee_per_gas"},
			expected: false,
		},
		{
			name:     "Non-DynamicFeeTxType with no filters",
			txType:   ethtypes.LegacyTxType,
			filters:  []string{},
			expected: true,
		},
		{
			name:     "Non-DynamicFeeTxType with gasPrice",
			txType:   ethtypes.LegacyTxType,
			filters:  []string{"gas_price"},
			expected: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			if got := IsFiltersSupportedByTxType(tt.txType, tt.filters); got != tt.expected {
				t.Errorf("IsFiltersSupportedByTxType() = %v, expected %v", got, tt.expected)
			}
		})
	}
}
