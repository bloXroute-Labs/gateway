package eth

import (
	"sync/atomic"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/ethereum/go-ethereum/rpc"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

var textBlockMap = map[string]interface{}{
	"baseFeePerGas":   "0x7ed4d4dd0",
	"difficulty":      "0x0",
	"extraData":       "0x496c6c756d696e61746520446d6f63726174697a6520447374726962757465",
	"gasLimit":        "0x1c9c380",
	"gasUsed":         "0xa19801",
	"hash":            "0xa3a4e0f6561d18f1188e714afe13e3e56b7fadd10504193fcf08c76cb651a73c",
	"logsBloom":       "0xc2212318c10003f9c4ab519c81413c2102417ac32410584274a9b20006168534114712bca6a81820f82c34263c8281c112a1d902ba023d22da700281906d21a0004ef11832025a3cef0044c9bf2498a7af018c0f4562ad8e14871b49c0600aa15a207c71862b0e0734991390812c5b6803d04460212c5c20a42e439ce0b8008559d1cd7b938a088d94ec44e077818e86202417a5a92902ec164429c2a7370800aa4babcd97837919150362b853e7fc43870c05bcbd0694ba0534b4b3843c1767825a06df9c99407b013244f2aa78b784e466646f82bd24707430c33f082224639150385ccd029aa99955041e41700a4c9903953233dc0542096a6e1b8a638764",
	"miner":           "0xdafea492d9c6733ae3d56b7ed1adb60692c98bc5",
	"mixHash":         "0x776bd31b55e90d06dd75513af678f6434511a40be7e8bdedaa5f8c5f9b08da22",
	"nonce":           "0x0000000000000000",
	"number":          "0x107e107",
	"parentHash":      "0xf1ae1f4d524a977ef2bc95844dd10aa1ce91ebe5fe8f8f89398d9996479d7250",
	"receiptsRoot":    "0x4944041b5c0f2bedd593017ce3c55c47aeebd5741e9c17a75244f2cc551869c9",
	"sha3Uncles":      "0x1dcc4de8dec75d7aab85b567b6ccd41ad312451b948a7413f0a142fd40d49347",
	"size":            "0xb863",
	"stateRoot":       "0x6c03b3023115152825e6fb21f95522c3982e48d798a4bd188d5f0e74a0248cab",
	"timestamp":       "0x64676d47",
	"totalDifficulty": "0xc70d815d562d3cfa955",
	"transactions": []interface{}{
		"0x4df870e552898df04761d6ea87ac848e3c60bfa35a9036b2b4d53ac64730a5b6",
	},
	"transactionsRoot": "0x1e1b41dfa3e2549bb76e976dc8296e18aaba570e633fe786c51087dfc19274df",
	"withdrawalsRoot":  "0x0",
}

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
	open               atomic.Bool
	numReceiptsFetched atomic.Int64
	numRPCCalls        atomic.Int64
	TxSent             []string
}

// NewMockWSProvider returns a MockWSProvider
func NewMockWSProvider(ethWSUri string, peerEndpoint types.NodeEndpoint, timeout time.Duration) blockchain.WSProvider {
	return &MockWSProvider{
		ethWSURI:   ethWSUri,
		endpoint:   peerEndpoint,
		syncStatus: blockchain.Unsynced,
		open:       atomic.Bool{},
	}
}

// ResetCounters resets the counters
func (m *MockWSProvider) ResetCounters() {
	m.numReceiptsFetched.Store(0)
	m.numRPCCalls.Store(0)
}

// NumReceiptsFetched returns the number of receipts fetched
func (m *MockWSProvider) NumReceiptsFetched() int {
	return int(m.numReceiptsFetched.Load())
}

// NumRPCCalls returns the number of RPC calls made
func (m *MockWSProvider) NumRPCCalls() int {
	return int(m.numRPCCalls.Load())
}

// SetBlockchainPeer sets the blockchain peer that corresponds to the ws client
func (m *MockWSProvider) SetBlockchainPeer(peer interface{}) {
}

// UnsetBlockchainPeer unsets the blockchain peer that corresponds to the ws client
func (m *MockWSProvider) UnsetBlockchainPeer() {
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
func (m *MockWSProvider) Subscribe(responseChannel interface{}, feedName string, args ...interface{}) (*blockchain.Subscription, error) {
	return &blockchain.Subscription{Sub: &rpc.ClientSubscription{}}, nil
}

// CallRPC returns a fake response with no error
func (m *MockWSProvider) CallRPC(method string, payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	m.numRPCCalls.Add(1)
	return "response", nil
}

// SendTransaction returns fake response with no error
func (m *MockWSProvider) SendTransaction(rawTx string, options blockchain.RPCOptions) (interface{}, error) {
	m.TxSent = append(m.TxSent, rawTx)
	return nil, nil
}

// FetchTransactionReceipt returns a fake response with no error
func (m *MockWSProvider) FetchTransactionReceipt(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	m.numReceiptsFetched.Add(1)
	return testTxReceiptMap, nil
}

// FetchTransaction returns a fake response with no error
func (m *MockWSProvider) FetchTransaction(payload []interface{}, options blockchain.RPCOptions) (interface{}, error) {
	return nil, nil
}

// FetchBlock query a block given height via CallRPC
func (m *MockWSProvider) FetchBlock(_ []interface{}, _ blockchain.RPCOptions) (interface{}, error) {
	return textBlockMap, nil
}

// TraceTransaction returns a fake response with no error
func (m *MockWSProvider) TraceTransaction(payload []interface{}, options blockchain.RPCOptions) (blockchain.TraceTransactionResponse, error) {
	return blockchain.TraceTransactionResponse{}, nil
}

// Dial is a no-op
func (m *MockWSProvider) Dial() {
	m.open.Store(true)
}

// Close is a no-op
func (m *MockWSProvider) Close() {
	m.open.Store(false)
}

// Addr noop
func (m *MockWSProvider) Addr() string { return m.ethWSURI }

// IsOpen noop
func (m *MockWSProvider) IsOpen() bool { return m.open.Load() }

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
