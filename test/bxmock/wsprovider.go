package bxmock

import (
	"github.com/bloXroute-Labs/gateway/blockchain"
	"github.com/ethereum/go-ethereum/rpc"
	log "github.com/sirupsen/logrus"
)

// MockWSProvider is a dummy struct that implements blockchain.WSProvider
type MockWSProvider struct {
	syncStatusCh chan blockchain.NodeSyncStatus
}

// NewMockWSProvider returns a new MockWSProvider
func NewMockWSProvider() blockchain.WSProvider {
	return &MockWSProvider{
		syncStatusCh: make(chan blockchain.NodeSyncStatus, 1),
	}
}

// Subscribe returns a dummy subscription
func (m *MockWSProvider) Subscribe(responseChannel interface{}, feedName string) (*blockchain.Subscription, error) {
	return &blockchain.Subscription{&rpc.ClientSubscription{}}, nil
}

// CallRPC returns a fake response with no error
func (m *MockWSProvider) CallRPC(method string, payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	return "response", nil
}

// FetchTransactionReceipt returns a fake response with no error
func (m *MockWSProvider) FetchTransactionReceipt(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	return "response", nil
}

// Connect is a no-op
func (m *MockWSProvider) Connect() {
	return
}

// Close is a no-op
func (m *MockWSProvider) Close() {
}

// Log returns a fake log entry
func (m *MockWSProvider) Log() *log.Entry {
	return log.WithFields(log.Fields{
		"connType":   "WS",
		"remoteAddr": "123.45.67.8",
	})
}

// GetValidRPCCallMethods returns an empty list
func (m *MockWSProvider) GetValidRPCCallMethods() []string {
	return []string{}
}

// GetValidRPCCallPayloadFields returns an empty list
func (m *MockWSProvider) GetValidRPCCallPayloadFields() []string {
	return []string{}
}

// GetRequiredPayloadFieldsForRPCMethod returns an empty list with no error
func (m *MockWSProvider) GetRequiredPayloadFieldsForRPCMethod(method string) ([]string, bool) {
	return []string{}, true
}

// ConstructRPCCallPayload is a no-op
func (m *MockWSProvider) ConstructRPCCallPayload(method string, callParams map[string]string, tag string) ([]interface{}, error) {
	return nil, nil
}

// UpdateNodeSyncStatus pushes sync update to syncStatusCh
func (m *MockWSProvider) UpdateNodeSyncStatus(syncStatus blockchain.NodeSyncStatus) {
	m.syncStatusCh <- syncStatus
}

// ReceiveNodeSyncStatusUpdate returns the syncStatusCh
func (m *MockWSProvider) ReceiveNodeSyncStatusUpdate() chan blockchain.NodeSyncStatus {
	return m.syncStatusCh
}
