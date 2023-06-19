package bxmock

import (
	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// MockWSManager is a dummy struct that implements blockchain.WSManager
type MockWSManager struct {
	syncStatusCh chan blockchain.NodeSyncStatus
}

// NewMockWSManager returns a new MockWSManager
func NewMockWSManager() blockchain.WSManager {
	return &MockWSManager{
		syncStatusCh: make(chan blockchain.NodeSyncStatus, 1),
	}
}

// Synced always indicates synced
func (m *MockWSManager) Synced() bool {
	return true
}

// Provider is a no-op
func (m *MockWSManager) Provider(nodeEndpoint *types.NodeEndpoint) (blockchain.WSProvider, bool) {
	return nil, true
}

// SyncedProvider is a no-op
func (m *MockWSManager) SyncedProvider() (blockchain.WSProvider, bool) {
	return nil, true
}

// ProviderWithBlock is a no-op
func (m *MockWSManager) ProviderWithBlock(nodeEndpoint *types.NodeEndpoint, blockNumber uint64) (blockchain.WSProvider, bool) {
	return nil, true
}

// Providers is a no-op
func (m *MockWSManager) Providers() map[string]blockchain.WSProvider {
	return nil
}

// SetBlockchainPeer is a no-op
func (m *MockWSManager) SetBlockchainPeer(peer interface{}) bool {
	return true
}

// UnsetBlockchainPeer is a no-op
func (m *MockWSManager) UnsetBlockchainPeer(peerEndpoint types.NodeEndpoint) bool {
	return true
}

// ValidRPCCallMethods returns an empty list
func (m *MockWSManager) ValidRPCCallMethods() []string {
	return []string{}
}

// ValidRPCCallPayloadFields returns an empty list
func (m *MockWSManager) ValidRPCCallPayloadFields() []string {
	return []string{}
}

// RequiredPayloadFieldsForRPCMethod returns an empty list with no error
func (m *MockWSManager) RequiredPayloadFieldsForRPCMethod(method string) ([]string, bool) {
	return []string{}, true
}

// ConstructRPCCallPayload is a no-op
func (m *MockWSManager) ConstructRPCCallPayload(method string, callParams map[string]string, tag string) ([]interface{}, error) {
	return nil, nil
}

// UpdateNodeSyncStatus naively pushes sync update to syncStatusCh
func (m *MockWSManager) UpdateNodeSyncStatus(nodeEndpoint types.NodeEndpoint, syncStatus blockchain.NodeSyncStatus) {
	m.syncStatusCh <- syncStatus
}

// ReceiveNodeSyncStatusUpdate provides a channel to receive NodeSyncStatus updates
func (m *MockWSManager) ReceiveNodeSyncStatusUpdate() chan blockchain.NodeSyncStatus {
	return m.syncStatusCh
}
