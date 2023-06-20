package blockchain

import "github.com/bloXroute-Labs/gateway/v2/types"

// TODO: remove SetBlockchainPeer and UnsetBlockchainPeer from interface and implementation

// WSManager provides an interface to manage websocket providers
type WSManager interface {
	Synced() bool
	UpdateNodeSyncStatus(types.NodeEndpoint, NodeSyncStatus)
	ReceiveNodeSyncStatusUpdate() chan NodeSyncStatus
	SyncedProvider() (WSProvider, bool)
	Provider(nodeEndpoint *types.NodeEndpoint) (WSProvider, bool)
	Providers() map[string]WSProvider
	ProviderWithBlock(nodeEndpoint *types.NodeEndpoint, blockNumber uint64) (WSProvider, bool)
	SetBlockchainPeer(peer interface{}) bool
	UnsetBlockchainPeer(peerEndpoint types.NodeEndpoint) bool
	ValidRPCCallMethods() []string
	ValidRPCCallPayloadFields() []string
	RequiredPayloadFieldsForRPCMethod(method string) ([]string, bool)
	ConstructRPCCallPayload(method string, callParams map[string]string, tag string) ([]interface{}, error)
}
