package eth

import (
	"context"
	"fmt"
	"strings"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/rpc"
)

// WSProvider implements the blockchain.WSProvider interface for Ethereum
type WSProvider struct {
	addr          string
	open          bool
	peerEndpoint  types.NodeEndpoint
	peer          *Peer
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

// NewWSProvider - returns a new instance of WSProvider
func NewWSProvider(ethWSUri string, peerEndpoint types.NodeEndpoint, timeout time.Duration) blockchain.WSProvider {
	var ws WSProvider

	ws.log = log.WithFields(log.Fields{
		"component":  "wsProvider",
		"connType":   "WS",
		"remoteAddr": ethWSUri,
	})
	ws.timeout = timeout
	ws.syncStatus = blockchain.Unsynced
	ws.addr = ethWSUri
	ws.peerEndpoint = peerEndpoint

	return &ws
}

// Dial - dials websocket address and sets ws client with established connection
func (ws *WSProvider) Dial() {
	// gateway should retry connecting to the ws url until it's successfully connected
	ws.log.Debugf("dialing %v... process %v", ws.addr, utils.GetGID())
	for {
		ctx, cancel := context.WithTimeout(context.Background(), ws.timeout)
		client, err := rpc.DialContext(ctx, ws.addr)
		if err == nil {
			cancel()
			ws.client = client
			ws.open = true
			ws.log.Info("connection was successfully established")
			return
		}

		ws.log.Warnf("failed to dial: err %v, retrying...", err)
		continue
	}
}

// SetBlockchainPeer sets the blockchain peer that corresponds to the ws client
func (ws *WSProvider) SetBlockchainPeer(peer interface{}) {
	ws.peer = peer.(*Peer)
	ws.log.Debugf("set blockchain peer %v corresponding to ws client  process %v", peer.(*Peer).endpoint.IPPort(), utils.GetGID())
}

// UnsetBlockchainPeer unsets the blockchain peer that corresponds to the ws client
func (ws *WSProvider) UnsetBlockchainPeer() {
	ws.peer = nil
	ws.log.Debugf("unset blockchain peer corresponding to ws client  process %v", utils.GetGID())
}

// BlockchainPeer returns the blockchain peer that corresponds to the ws client
func (ws *WSProvider) BlockchainPeer() interface{} {
	return ws.peer
}

// BlockchainPeerEndpoint returns the endpoint of the corresponding node or of eth ws uri if there is no corresponding node
func (ws *WSProvider) BlockchainPeerEndpoint() types.NodeEndpoint {
	return ws.peerEndpoint
}

// Subscribe - subscribes to Ethereum feeds and returns subscription
func (ws *WSProvider) Subscribe(responseChannel interface{}, feedName string, args ...interface{}) (*blockchain.Subscription, error) {
	ctx, cancel := context.WithTimeout(context.Background(), ws.timeout)
	defer cancel()

	var sub *rpc.ClientSubscription
	var err error
	if len(args) < 1 {
		sub, err = ws.client.EthSubscribe(ctx, responseChannel, feedName)
	} else {
		sub, err = ws.client.EthSubscribe(ctx, responseChannel, feedName, args[0])
	}

	if err != nil {
		return nil, fmt.Errorf("failed to subscribe to feed %v: %v", feedName, err)
	}
	ws.subscriptions = append(ws.subscriptions, blockchain.Subscription{Sub: sub})
	return &blockchain.Subscription{Sub: sub}, nil
}

// Addr returns web-socket connection address
func (ws *WSProvider) Addr() string { return ws.addr }

// IsOpen returns true if web-socket connection is active
func (ws *WSProvider) IsOpen() bool { return ws.open }

// Close - unsubscribes all active subscriptions and closes client connection
func (ws *WSProvider) Close() {
	ws.log.Debugf("closing websocket to node, open subscriptions %v  process %v", len(ws.subscriptions), utils.GetGID())
	for _, s := range ws.subscriptions {
		s.Sub.(*rpc.ClientSubscription).Unsubscribe()
	}
	ws.subscriptions = ws.subscriptions[:0]
	ws.client.Close()
	ws.open = false
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

// SendTransaction sends signed transaction in payload to node via CallRPC
func (ws *WSProvider) SendTransaction(rawTx string, options blockchain.RPCOptions) (interface{}, error) {
	return ws.CallRPC("eth_sendRawTransaction", []interface{}{rawTx}, options)
}

// FetchTransactionReceipt fetches transaction receipt via CallRPC
func (ws *WSProvider) FetchTransactionReceipt(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	return ws.CallRPC("eth_getTransactionReceipt", payload, options)
}

// FetchTransaction check status of a transaction via CallRPC
func (ws *WSProvider) FetchTransaction(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	return ws.CallRPC("eth_getTransactionByHash", payload, options)
}

// FetchBlock query a block given height via CallRPC
func (ws *WSProvider) FetchBlock(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	return ws.CallRPC("eth_getBlockByNumber", payload, options)
}

// Log - returns WSProvider log entry
func (ws *WSProvider) Log() *log.Entry {
	return ws.log
}

// UpdateSyncStatus - updates sync status of ws client node
func (ws *WSProvider) UpdateSyncStatus(status blockchain.NodeSyncStatus) {
	ws.syncStatus = status
	ws.log.Debugf("updated sync status to %v process %v", status, utils.GetGID())
}

// SyncStatus - returns sync status of ws client node
func (ws *WSProvider) SyncStatus() blockchain.NodeSyncStatus {
	return ws.syncStatus
}
