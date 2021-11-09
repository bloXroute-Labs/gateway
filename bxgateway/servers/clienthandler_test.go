package servers

import (
	"encoding/json"
	"fmt"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/sdnmessage"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/test/bxmock"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/test/fixtures"
	"github.com/gorilla/websocket"
	"github.com/stretchr/testify/assert"
	"golang.org/x/sync/errgroup"
	"net/http"
	"sync"
	"testing"
	"time"
)

func getMockCustomerAccountModel(types.AccountID) (sdnmessage.Account, error) {
	return sdnmessage.Account{AccountInfo: sdnmessage.AccountInfo{TierName: sdnmessage.ATierEnterprise}, SecretHash: "a61681a9456ca319a90036e8361dbc73"}, nil
}

func TestClientHandler(t *testing.T) {
	g := bxmock.MockBxListener{}
	feedChan := make(chan types.Notification)
	var wg sync.WaitGroup
	url := "127.0.0.1:28333"
	wsURL := fmt.Sprintf("ws://%s/ws", url)
	fm := NewFeedManager(g, feedChan, &wg, url, types.NetworkNum(1), sdnmessage.Account{}, getMockCustomerAccountModel)
	var group errgroup.Group
	group.Go(fm.Start)
	time.Sleep(10 * time.Millisecond)

	dialer := websocket.DefaultDialer
	headers := make(http.Header)
	dummyAuthHeader := "ZDJhYjkzYmEtMWE4Yi00MTg3LTk5NGUtYzYzODk2YzkzNmUzOmE2MTY4MWE5NDU2Y2EzMTlhOTAwMzZlODM2MWRiYzcz"
	headers.Set("Authorization", dummyAuthHeader)
	ws, _, err := dialer.Dial(wsURL, headers)
	assert.Nil(t, err)

	t.Run("wsClient", func(t *testing.T) {
		handlePingRequest(t, ws)
		handleBlxrTxRequestLegacyTx(t, ws)
		handleBlxrTxRequestAccessListTx(t, ws)
		handleBlxrTxRequestDynamicFeeTx(t, ws)
	})
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
