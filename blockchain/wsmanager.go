package blockchain

import "github.com/bloXroute-Labs/gateway/types"

// WSManager provides an interface to manage websocket providers
type WSManager interface {
	Synced() bool
	UpdateNodeSyncStatus(types.NodeEndpoint, NodeSyncStatus)
	ReceiveNodeSyncStatusUpdate() chan NodeSyncStatus
	SyncedProvider() (WSProvider, bool)
	Provider(nodeEndpoint *types.NodeEndpoint) (WSProvider, bool)
	Providers() map[string]WSProvider
	SetBlockchainPeer(peer interface{}) bool
	UnsetBlockchainPeer(peerEndpoint types.NodeEndpoint) bool
	ValidRPCCallMethods() []string
	ValidRPCCallPayloadFields() []string
	RequiredPayloadFieldsForRPCMethod(method string) ([]string, bool)
	ConstructRPCCallPayload(method string, callParams map[string]string, tag string) ([]interface{}, error)
}
