package blockchain

import (
	log "github.com/sirupsen/logrus"
	"time"
)

// RPCOptions provides options to customize RPC call using WSProvider.CallRPC
type RPCOptions struct {
	RetryAttempts int
	RetryInterval time.Duration
}

// NodeSyncStatus indicates if blockchain node is synced or unsynced
type NodeSyncStatus int

// enumeration for NodeSyncStatus
const (
	Synced NodeSyncStatus = iota
	Unsynced
)

// Subscription represents a client RPC subscription
type Subscription struct {
	Sub interface{}
}

// WSProvider provides an interface to interact with blockchain client via websocket RPC
type WSProvider interface {
	Connect()
	Close()
	Subscribe(responseChannel interface{}, feedName string) (*Subscription, error)
	CallRPC(method string, payload []interface{}, options RPCOptions) (interface{}, error)
	FetchTransactionReceipt(payload []interface{}, options RPCOptions) (interface{}, error)
	Log() *log.Entry
	GetValidRPCCallMethods() []string
	GetValidRPCCallPayloadFields() []string
	GetRequiredPayloadFieldsForRPCMethod(method string) ([]string, bool)
	ConstructRPCCallPayload(method string, callParams map[string]string, tag string) ([]interface{}, error)
	UpdateNodeSyncStatus(NodeSyncStatus)
	ReceiveNodeSyncStatusUpdate() chan NodeSyncStatus
}
