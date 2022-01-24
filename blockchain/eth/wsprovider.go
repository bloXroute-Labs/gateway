package eth

import (
	"context"
	"fmt"
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/ethereum/go-ethereum/rpc"
	log "github.com/sirupsen/logrus"
	"strings"
	"time"
)

// WSProvider implements the blockchain.WSProvider interface for Ethereum
type WSProvider struct {
	addr          string
	client        *rpc.Client
	log           *log.Entry
	ctx           context.Context
	timeout       time.Duration
	subscriptions []blockchain.Subscription
	syncStatus    blockchain.NodeSyncStatus
	syncStatusCh  chan blockchain.NodeSyncStatus
}

// RPCResponse represents the Ethereum RPC response
type RPCResponse struct {
	JSONRPC string      `json:"jsonrpc"`
	ID      interface{} `json:"id"`
	Method  string      `json:"method"`
	Params  interface{} `json:"params"`
}

var validRPCCallPayloadFields = []string{"data", "from", "to", "gasPrice", "gas", "address", "pos"}

var validRPCCallMethods = []string{"eth_call", "eth_getBalance", "eth_getTransactionCount", "eth_getCode", "eth_getStorageAt", "eth_blockNumber"}

var commandMethodsToRequiredPayloadFields = map[string][]string{
	"eth_call":                {"data"},
	"eth_getBalance":          {"address"},
	"eth_getTransactionCount": {"address"},
	"eth_getCode":             {"address"},
	"eth_getStorageAt":        {"address", "pos"},
	"eth_blockNumber":         {},
}

// NewEthWSProvider - returns a new instance of WSProvider
func NewEthWSProvider(ethWSUri string, timeout time.Duration) blockchain.WSProvider {
	var ws WSProvider

	ws.log = log.WithFields(log.Fields{
		"connType":   "WS",
		"remoteAddr": ethWSUri,
	})
	ws.timeout = timeout
	ws.syncStatus = blockchain.Synced
	ws.syncStatusCh = make(chan blockchain.NodeSyncStatus, 1)
	ws.addr = ethWSUri
	ws.Connect()

	ws.log.Infof("connection was successfully established with %v", ethWSUri)
	return &ws
}

// Connect - dials websocket address and returns client with established connection
func (ws *WSProvider) Connect() {
	// gateway should retry connecting to the ws url until it's successfully connected
	ws.log.Debugf("dialing %v...", ws.addr)
	for {
		client, err := rpc.Dial(ws.addr)
		if err == nil {
			ws.client = client
			return
		}

		time.Sleep(5 * time.Second)
		ws.log.Warnf("Failed to dial %v, retrying..", ws.addr)
		continue
	}
}

// Subscribe - subscribes to Ethereum feeds and returns subscription
func (ws *WSProvider) Subscribe(responseChannel interface{}, feedName string) (*blockchain.Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ws.timeout)
	defer cancel()

	sub, err := ws.client.EthSubscribe(ctx, responseChannel, feedName)
	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to feed %v: %v", feedName, err)
	}
	ws.subscriptions = append(ws.subscriptions, blockchain.Subscription{Sub: sub})
	return &blockchain.Subscription{Sub: sub}, nil
}

// Close - unsubscribes all active subscriptions and closes client connection
func (ws *WSProvider) Close() {
	for _, s := range ws.subscriptions {
		s.Sub.(*rpc.ClientSubscription).Unsubscribe()
	}
	ws.subscriptions = ws.subscriptions[:0]
	ws.client.Close()
}

// CallRPC - executes Ethereum RPC calls
func (ws *WSProvider) CallRPC(method string, payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	var (
		response interface{}
		err      error
	)
	for retries := 0; retries < options.RetryAttempts; retries++ {
		err = ws.client.Call(&response, method, payload...)
		if (err != nil && strings.Contains(err.Error(), "header not found")) || response == nil {
			time.Sleep(options.RetryInterval)
			continue
		}
		break
	}
	return response, err
}

// FetchTransactionReceipt fetches transaction receipt via CallRPC
func (ws *WSProvider) FetchTransactionReceipt(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	return ws.CallRPC("eth_getTransactionReceipt", payload, options)
}

// Log - returns WSProvider log entry
func (ws *WSProvider) Log() *log.Entry {
	return ws.log
}

//GetValidRPCCallMethods returns valid Ethereum RPC command methods
func (ws *WSProvider) GetValidRPCCallMethods() []string {
	return validRPCCallMethods
}

// GetValidRPCCallPayloadFields returns valid Ethereum RPC method payload fields
func (ws *WSProvider) GetValidRPCCallPayloadFields() []string {
	return validRPCCallPayloadFields
}

// GetRequiredPayloadFieldsForRPCMethod returns the valid payload fields for the provided Ethereum RPC command method
func (ws *WSProvider) GetRequiredPayloadFieldsForRPCMethod(method string) ([]string, bool) {
	requiredFields, ok := commandMethodsToRequiredPayloadFields[method]
	return requiredFields, ok
}

// ConstructRPCCallPayload returns payload used in RPC call
func (ws *WSProvider) ConstructRPCCallPayload(method string, callParams map[string]string, tag string) ([]interface{}, error) {
	switch method {
	case "eth_call":
		payload := []interface{}{callParams, tag}
		return payload, nil
	case "eth_blockNumber":
		return []interface{}{}, nil
	case "eth_getStorageAt":
		payload := []interface{}{callParams["address"], callParams["pos"], tag}
		return payload, nil
	case "eth_getBalance":
		fallthrough
	case "eth_getCode":
		fallthrough
	case "eth_getTransactionCount":
		payload := []interface{}{callParams["address"], tag}
		return payload, nil
	default:
		return nil, fmt.Errorf("unexpectedly failed to match method %v", method)
	}
}

// UpdateNodeSyncStatus sends update on NodeSyncStatus channel if status has changed
func (ws *WSProvider) UpdateNodeSyncStatus(syncStatus blockchain.NodeSyncStatus) {
	if syncStatus == ws.syncStatus {
		return
	}
	select {
	case ws.syncStatusCh <- syncStatus:
		ws.syncStatus = syncStatus
	default:
		// enables non-blocking
	}
}

// ReceiveNodeSyncStatusUpdate provides a channel to receive NodeSyncStatus updates
func (ws *WSProvider) ReceiveNodeSyncStatusUpdate() chan blockchain.NodeSyncStatus {
	return ws.syncStatusCh
}
