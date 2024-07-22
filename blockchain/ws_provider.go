package blockchain

import (
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// NodeSyncStatus indicates if blockchain node is synced or unsynced
type NodeSyncStatus string

// NodeSyncStatus types enumeration
const (
	Synced   NodeSyncStatus = "SYNCED"
	Unsynced NodeSyncStatus = "UNSYNCED"
)

// RPCOptions provides options to customize RPC call using WSProvider.CallRPC
type RPCOptions struct {
	RetryAttempts int
	RetryInterval time.Duration
}

// DefaultRPCOptions - provides default options for CallRPC
var DefaultRPCOptions = RPCOptions{RetryAttempts: 5, RetryInterval: 10 * time.Millisecond}

// Subscription represents a client RPC subscription
type Subscription struct {
	Sub interface{}
}

// WSProvider provides an interface to interact with blockchain client via websocket RPC
type WSProvider interface {
	Dial()
	Close()
	Addr() string
	IsOpen() bool
	SetBlockchainPeer(peer interface{})
	UnsetBlockchainPeer()
	BlockchainPeer() interface{}
	BlockchainPeerEndpoint() types.NodeEndpoint
	UpdateSyncStatus(status NodeSyncStatus)
	SyncStatus() NodeSyncStatus
	Subscribe(responseChannel interface{}, feedName string, args ...interface{}) (*Subscription, error)
	CallRPC(method string, payload []interface{}, options RPCOptions) (interface{}, error)
	FetchTransaction(payload []interface{}, options RPCOptions) (interface{}, error)
	FetchBlock(payload []interface{}, options RPCOptions) (interface{}, error)
	FetchTransactionReceipt(payload []interface{}, options RPCOptions) (interface{}, error)
	SendTransaction(rawTx string, options RPCOptions) (interface{}, error)
	TraceTransaction(payload []interface{}, options RPCOptions) (TraceTransactionResponse, error)
	Log() *log.Entry
}

// TraceTransactionResponse is response from debug_traceTransaction call
type TraceTransactionResponse struct {
	Post map[string]struct {
		Balance string `json:"balance"`
	} `json:"post"`
	Pre map[string]struct {
		Balance string `json:"balance"`
	} `json:"pre"`
}
