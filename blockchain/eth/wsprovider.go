package eth

import (
	"context"
	"fmt"
	"github.com/bloXroute-Labs/gateway/blockchain"
	log "github.com/bloXroute-Labs/gateway/logger"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/ethereum/go-ethereum/rpc"
	"strings"
	"time"
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
	ws.log.Debugf("dialing %v...", ws.addr)
	for {
		client, err := rpc.Dial(ws.addr)
		if err == nil {
			ws.client = client
			ws.open = true
			ws.log.Infof("connection was successfully established with %v", ws.addr)
			return
		}

		time.Sleep(5 * time.Second)
		ws.log.Warnf("Failed to dial %v, retrying..", ws.addr)
		continue
	}
}

// SetBlockchainPeer sets the blockchain peer that corresponds to the ws client
func (ws *WSProvider) SetBlockchainPeer(peer interface{}) {
	ws.peer = peer.(*Peer)
	ws.Log().Debugf("set blockchain peer %v corresponding to ws client", peer.(*Peer).endpoint.IPPort())
}

// UnsetBlockchainPeer unsets the blockchain peer that corresponds to the ws client
func (ws *WSProvider) UnsetBlockchainPeer() {
	ws.peer = nil
	ws.Log().Debug("unset blockchain peer corresponding to ws client")
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

// Addr returns web-socket connection address
func (ws *WSProvider) Addr() string { return ws.addr }

// IsOpen returns true if web-socket connection is active
func (ws *WSProvider) IsOpen() bool { return ws.open }

// Close - unsubscribes all active subscriptions and closes client connection
func (ws *WSProvider) Close() {
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

// FetchTransactionReceipt fetches transaction receipt via CallRPC
func (ws *WSProvider) FetchTransactionReceipt(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	return ws.CallRPC("eth_getTransactionReceipt", payload, options)
}

// Log - returns WSProvider log entry
func (ws *WSProvider) Log() *log.Entry {
	return ws.log
}

// UpdateSyncStatus - updates sync status of ws client node
func (ws *WSProvider) UpdateSyncStatus(status blockchain.NodeSyncStatus) {
	ws.syncStatus = status
	ws.Log().Debugf("updated sync status to %v", status)
}

// SyncStatus - returns sync status of ws client node
func (ws *WSProvider) SyncStatus() blockchain.NodeSyncStatus {
	return ws.syncStatus
}
