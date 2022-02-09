package servers

import (
	"context"
	"encoding/json"
	"errors"
	"fmt"
	"github.com/bloXroute-Labs/gateway"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/bloXroute-Labs/gateway/config"
	"github.com/bloXroute-Labs/gateway/sdnmessage"
	"github.com/bloXroute-Labs/gateway/test/bxmock"
	"github.com/bloXroute-Labs/gateway/test/fixtures"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/gorilla/websocket"
	uuid "github.com/satori/go.uuid"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"golang.org/x/sync/errgroup"
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
	},
}

func getMockCustomerAccountModel(accountID types.AccountID) (sdnmessage.Account, error) {
	var err error
	if accountID == "d" {
		err = fmt.Errorf("Timeout error")
	}
	return accountIDToAccountModel[accountID], err
}

func TestClientHandler(t *testing.T) {
	g := bxmock.MockBxListener{}
	feedChan := make(chan types.Notification)
	url := "127.0.0.1:28332"
	wsURL := fmt.Sprintf("ws://%s/ws", url)
	gwAccount, _ := getMockCustomerAccountModel("gw")
	cfg := config.Bx{WebsocketPort: 28332, ManageWSServer: true, WebsocketTLSEnabled: false}

	fm := NewFeedManager(context.Background(), g, feedChan, types.NetworkNum(1), bxmock.NewMockWSProvider(), gwAccount, getMockCustomerAccountModel, "", "", cfg)
	var group errgroup.Group
	group.Go(fm.Start)
	time.Sleep(10 * time.Millisecond)

	dialer := websocket.DefaultDialer
	headers := make(http.Header)

	// pass - different account for server and client
	dummyAuthHeader := "YToxMjM0NTY="
	headers.Set("Authorization", dummyAuthHeader)
	ws, _, err := dialer.Dial(wsURL, headers)
	assert.Nil(t, err)

	t.Run("wsClient", func(t *testing.T) {
		handlePingRequest(t, ws)
	})

	// fail for tier type
	dummyAuthHeader = "Yjo3ODkxMDEx"
	headers.Set("Authorization", dummyAuthHeader)
	ws, _, err = dialer.Dial(wsURL, headers)
	assert.NotNil(t, err)

	// fail for secret hash
	dummyAuthHeader = "Yzo3ODkxMDEx"
	headers.Set("Authorization", dummyAuthHeader)
	ws, _, err = dialer.Dial(wsURL, headers)
	assert.NotNil(t, err)

	// fail for timeout - account should set to enterprise
	dummyAuthHeader = "ZDo3ODkxMDEx"
	headers.Set("Authorization", dummyAuthHeader)
	ws, _, err = dialer.Dial(wsURL, headers)
	assert.Nil(t, err)

	t.Run("wsClient", func(t *testing.T) {
		handlePingRequest(t, ws)
	})

	// pass - same account for server and client
	dummyAuthHeader = "Z3c6c2VjcmV0"
	headers.Set("Authorization", dummyAuthHeader)
	ws, _, err = dialer.Dial(wsURL, headers)
	assert.Nil(t, err)

	t.Run("wsClient", func(t *testing.T) {
		handlePingRequest(t, ws)
		handleBlxrTxRequestLegacyTx(t, ws)
		handleBlxrTxRequestAccessListTx(t, ws)
		handleBlxrTxRequestDynamicFeeTx(t, ws)
		subscriptionID := handleSubscribe(t, fm, ws)
		handleUnsubscribe(t, fm, subscriptionID)
		testWSShutdown(t, fm, ws)
	})
}

func handleSubscribe(t *testing.T, fm *FeedManager, ws *websocket.Conn) uuid.UUID {
	subscribeMsg := writeMsgToWsAndReadResponse(ws, []byte(`{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`))
	clientRes := getClientResponse(subscribeMsg)
	subscriptionID, err := uuid.FromString(fmt.Sprintf("%v", clientRes.Result))
	assert.Nil(t, err)
	_, exists := fm.idToClientSubscription[subscriptionID]
	assert.True(t, exists)

	return subscriptionID
}

func handleUnsubscribe(t *testing.T, fm *FeedManager, subscriptionID uuid.UUID) {
	err := fm.Unsubscribe(subscriptionID)
	assert.Nil(t, err)
	_, exists := fm.idToClientSubscription[subscriptionID]
	assert.False(t, exists)
}

func testWSShutdown(t *testing.T, fm *FeedManager, ws *websocket.Conn) {
	subscribeMsg := writeMsgToWsAndReadResponse(ws, []byte(`{"id": "1", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`))
	clientRes := getClientResponse(subscribeMsg)
	subscriptionID, err := uuid.FromString(fmt.Sprintf("%v", clientRes.Result))
	assert.Nil(t, err)
	_, exists := fm.idToClientSubscription[subscriptionID]
	assert.True(t, exists)

	subscribeMsg2 := writeMsgToWsAndReadResponse(ws, []byte(`{"id": "2", "method": "subscribe", "params": ["newTxs", {"include": ["tx_hash"]}]}`))
	clientRes2 := getClientResponse(subscribeMsg2)
	subscriptionID2, err := uuid.FromString(fmt.Sprintf("%v", clientRes2.Result))
	assert.Nil(t, err)
	_, exists = fm.idToClientSubscription[subscriptionID2]
	assert.True(t, exists)

	fm.blockchainWS.UpdateNodeSyncStatus(blockchain.Unsynced)
	time.Sleep(time.Millisecond)
	_, exists = fm.idToClientSubscription[subscriptionID]
	assert.False(t, exists)
	_, exists = fm.idToClientSubscription[subscriptionID2]
	assert.False(t, exists)
}

func handlePingRequest(t *testing.T, ws *websocket.Conn) {
	timeClientSendsRequest := time.Now().UTC()
	msg := writeMsgToWsAndReadResponse(ws, []byte(`{"id": "1", "method": "ping"}`))
	timeClientReceivesResponse := time.Now().UTC()

	clientRes := getClientResponse(msg)
	res := parsePingResult(clientRes.Result)
	timeServerReceivesRequest, err := time.Parse(bxgateway.MicroSecTimeFormat, res.Pong)
	assert.Nil(t, err)
	assert.True(t, timeClientReceivesResponse.After(timeServerReceivesRequest))
	assert.True(t, timeServerReceivesRequest.After(timeClientSendsRequest))
}

func handleBlxrTxRequestLegacyTx(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.LegacyTransaction)
	msg := writeMsgToWsAndReadResponse(ws, []byte(reqPayload))
	clientRes := getClientResponse(msg)
	res := parseBlxrTxResult(clientRes.Result)
	assert.Equal(t, fixtures.LegacyTransactionHash[2:], res.TxHash)
}

func handleBlxrTxRequestAccessListTx(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.AccessListTransactionForRPCInterface)
	msg := writeMsgToWsAndReadResponse(ws, []byte(reqPayload))
	clientRes := getClientResponse(msg)
	res := parseBlxrTxResult(clientRes.Result)
	assert.Equal(t, fixtures.AccessListTransactionHash[2:], res.TxHash)
}

func handleBlxrTxRequestDynamicFeeTx(t *testing.T, ws *websocket.Conn) {
	reqPayload := fmt.Sprintf(`{"id": "1", "method": "blxr_tx", "params": {"transaction": "%s"}}`, fixtures.DynamicFeeTransactionForRPCInterface)
	msg := writeMsgToWsAndReadResponse(ws, []byte(reqPayload))
	clientRes := getClientResponse(msg)
	res := parseBlxrTxResult(clientRes.Result)
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
			error: errors.New("empty typed transaction bytes"),
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
}

func getClientResponse(msg []byte) (cr clientResponse) {
	res := clientResponse{}
	err := json.Unmarshal(msg, &res)
	if err != nil {
		panic(err)
	}
	return res
}

func parsePingResult(rpcResponse interface{}) (pr rpcPingResponse) {
	res := rpcPingResponse{}
	b, err := json.Marshal(rpcResponse)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, &res)
	if err != nil {
		panic(err)
	}
	return res
}

func parseBlxrTxResult(rpcResponse interface{}) (tr rpcTxResponse) {
	res := rpcTxResponse{}
	b, err := json.Marshal(rpcResponse)
	if err != nil {
		panic(err)
	}
	err = json.Unmarshal(b, &res)
	if err != nil {
		panic(err)
	}
	return res
}

func writeMsgToWsAndReadResponse(conn *websocket.Conn, msg []byte) (response []byte) {
	err := conn.WriteMessage(websocket.TextMessage, msg)
	if err != nil {
		panic(err)
	}
	_, response, err = conn.ReadMessage()
	if err != nil {
		panic(err)
	}
	return response
}
