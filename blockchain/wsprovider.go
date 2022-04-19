package blockchain

import (
	log "github.com/bloXroute-Labs/gateway/logger"
	"github.com/bloXroute-Labs/gateway/types"
	"time"
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
	Subscribe(responseChannel interface{}, feedName string) (*Subscription, error)
	CallRPC(method string, payload []interface{}, options RPCOptions) (interface{}, error)
	FetchTransactionReceipt(payload []interface{}, options RPCOptions) (interface{}, error)
	Log() *log.Entry
}
