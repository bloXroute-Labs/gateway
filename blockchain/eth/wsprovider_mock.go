package eth

import (
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/rpc"
)

var testTxReceiptMap = map[string]interface{}{
	"to":                "0x18cf158e1766ca6bdbe2719dace440121b4603b3",
	"transactionHash":   "0x4df870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b6",
	"blockHash":         "0x5df870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b7",
	"blockNumber":       "0xd1d827",
	"contractAddress":   "0x28cf158e1766ca6bdbe2719dace440121b4603b2",
	"cumulativeGasUsed": "0xf9389e",
	"effectiveGasPrice": "0x1c298e1cb9",
	"from":              "0x13cf158e1766ca6bdbe2719dace440121b4603b1",
	"gasUsed":           "0x5208",
	"logs":              []interface{}{"0x7cf870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b5"},
	"logsBloom":         "0x3df870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b4",
	"status":            "0x1",
	"transactionIndex":  "0x64",
	"type":              "0x2",
}

// MockWSProvider is a dummy struct that implements blockchain.WSProvider
type MockWSProvider struct {
	ethWSURI           string
	endpoint           types.NodeEndpoint
	syncStatus         blockchain.NodeSyncStatus
	NumReceiptsFetched int
	NumRPCCalls        int
	TxSent             []string
}

// NewMockWSProvider returns a MockWSProvider
func NewMockWSProvider(ethWSUri string, peerEndpoint types.NodeEndpoint, timeout time.Duration) blockchain.WSProvider {
	return &MockWSProvider{
		ethWSURI:   ethWSUri,
		endpoint:   peerEndpoint,
		syncStatus: blockchain.Unsynced,
	}
}

// SetBlockchainPeer sets the blockchain peer that corresponds to the ws client
func (m *MockWSProvider) SetBlockchainPeer(peer interface{}) {
	return
}

// UnsetBlockchainPeer unsets the blockchain peer that corresponds to the ws client
func (m *MockWSProvider) UnsetBlockchainPeer() {
	return
}

// BlockchainPeerEndpoint returns the blockchain peer that corresponds to the ws client
func (m *MockWSProvider) BlockchainPeerEndpoint() types.NodeEndpoint {
	return m.endpoint
}

// BlockchainPeer returns the blockchain peer that corresponds to the ws client
func (m *MockWSProvider) BlockchainPeer() interface{} {
	return &Peer{}
}

// Subscribe returns a dummy subscription
func (m *MockWSProvider) Subscribe(responseChannel interface{}, feedName string) (*blockchain.Subscription, error) {
	return &blockchain.Subscription{&rpc.ClientSubscription{}}, nil
}

// CallRPC returns a fake response with no error
func (m *MockWSProvider) CallRPC(method string, payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	m.NumRPCCalls++
	return "response", nil
}

// SendTransaction returns fake response with no error
func (m *MockWSProvider) SendTransaction(rawTx string, options blockchain.RPCOptions) (interface{}, error) {
	m.TxSent = append(m.TxSent, rawTx)
	return nil, nil
}

// FetchTransactionReceipt returns a fake response with no error
func (m *MockWSProvider) FetchTransactionReceipt(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	m.NumReceiptsFetched++
	return testTxReceiptMap, nil
}

// FetchTransaction returns a fake response with no error
func (m *MockWSProvider) FetchTransaction(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	return nil, nil
}

// FetchBlock query a block given height via CallRPC
func (m *MockWSProvider) FetchBlock(_ []interface{}, _ blockchain.RPCOptions) (interface{}, error) {
	return nil, nil
}

// Dial is a no-op
func (m *MockWSProvider) Dial() {
	return
}

// Close is a no-op
func (m *MockWSProvider) Close() {
}

// Addr noop
func (m *MockWSProvider) Addr() string { return m.ethWSURI }

// IsOpen noop
func (m *MockWSProvider) IsOpen() bool { return true }

// Log returns a fake log entry
func (m *MockWSProvider) Log() *log.Entry {
	return log.WithFields(log.Fields{
		"connType":   "WS",
		"remoteAddr": "123.45.67.8",
	})
}

// ValidRPCCallMethods returns an empty list
func (m *MockWSProvider) ValidRPCCallMethods() []string {
	return []string{}
}

// ValidRPCCallPayloadFields returns an empty list
func (m *MockWSProvider) ValidRPCCallPayloadFields() []string {
	return []string{}
}

// RequiredPayloadFieldsForRPCMethod returns an empty list with no error
func (m *MockWSProvider) RequiredPayloadFieldsForRPCMethod(method string) ([]string, bool) {
	return []string{}, true
}

// ConstructRPCCallPayload is a no-op
func (m *MockWSProvider) ConstructRPCCallPayload(method string, callParams map[string]string, tag string) ([]interface{}, error) {
	return nil, nil
}

// UpdateSyncStatus - updates sync status of ws client node
func (m *MockWSProvider) UpdateSyncStatus(status blockchain.NodeSyncStatus) {
	m.syncStatus = status
}

// SyncStatus - returns sync status of ws client node
func (m *MockWSProvider) SyncStatus() blockchain.NodeSyncStatus {
	return m.syncStatus
}
