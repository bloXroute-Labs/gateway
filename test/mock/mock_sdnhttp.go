// Code generated by MockGen. DO NOT EDIT.
// Source: github.com/bloXroute-Labs/gateway/v2/connections (interfaces: SDNHTTP)
//
// Generated by this command:
//
//	mockgen -destination ../../bxgateway/test/mock/mock_sdnhttp.go -package mock . SDNHTTP

// Package mock is a generated GoMock package.
package mock

import (
	context "context"
	reflect "reflect"
	time "time"

	connections "github.com/bloXroute-Labs/gateway/v2/connections"
	sdnmessage "github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	types "github.com/bloXroute-Labs/gateway/v2/types"
	gomock "go.uber.org/mock/gomock"
)

// MockSDNHTTP is a mock of SDNHTTP interface.
type MockSDNHTTP struct {
	ctrl     *gomock.Controller
	recorder *MockSDNHTTPMockRecorder
}

// MockSDNHTTPMockRecorder is the mock recorder for MockSDNHTTP.
type MockSDNHTTPMockRecorder struct {
	mock *MockSDNHTTP
}

// NewMockSDNHTTP creates a new mock instance.
func NewMockSDNHTTP(ctrl *gomock.Controller) *MockSDNHTTP {
	mock := &MockSDNHTTP{ctrl: ctrl}
	mock.recorder = &MockSDNHTTPMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockSDNHTTP) EXPECT() *MockSDNHTTPMockRecorder {
	return m.recorder
}

// AccountModel mocks base method.
func (m *MockSDNHTTP) AccountModel() sdnmessage.Account {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AccountModel")
	ret0, _ := ret[0].(sdnmessage.Account)
	return ret0
}

// AccountModel indicates an expected call of AccountModel.
func (mr *MockSDNHTTPMockRecorder) AccountModel() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AccountModel", reflect.TypeOf((*MockSDNHTTP)(nil).AccountModel))
}

// AccountTier mocks base method.
func (m *MockSDNHTTP) AccountTier() sdnmessage.AccountTier {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "AccountTier")
	ret0, _ := ret[0].(sdnmessage.AccountTier)
	return ret0
}

// AccountTier indicates an expected call of AccountTier.
func (mr *MockSDNHTTPMockRecorder) AccountTier() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "AccountTier", reflect.TypeOf((*MockSDNHTTP)(nil).AccountTier))
}

// DirectRelayConnections mocks base method.
func (m *MockSDNHTTP) DirectRelayConnections(relayHosts string, relayLimit uint64, relayInstructions chan<- connections.RelayInstruction) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "DirectRelayConnections", relayHosts, relayLimit, relayInstructions)
	ret0, _ := ret[0].(error)
	return ret0
}

// DirectRelayConnections indicates an expected call of DirectRelayConnections.
func (mr *MockSDNHTTPMockRecorder) DirectRelayConnections(relayHosts, relayLimit, relayInstructions interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "DirectRelayConnections", reflect.TypeOf((*MockSDNHTTP)(nil).DirectRelayConnections), relayHosts, relayLimit, relayInstructions)
}

// FetchAllBlockchainNetworks mocks base method.
func (m *MockSDNHTTP) FetchAllBlockchainNetworks() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FetchAllBlockchainNetworks")
	ret0, _ := ret[0].(error)
	return ret0
}

// FetchAllBlockchainNetworks indicates an expected call of FetchAllBlockchainNetworks.
func (mr *MockSDNHTTPMockRecorder) FetchAllBlockchainNetworks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FetchAllBlockchainNetworks", reflect.TypeOf((*MockSDNHTTP)(nil).FetchAllBlockchainNetworks))
}

// FetchBlockchainNetwork mocks base method.
func (m *MockSDNHTTP) FetchBlockchainNetwork() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FetchBlockchainNetwork")
	ret0, _ := ret[0].(error)
	return ret0
}

// FetchBlockchainNetwork indicates an expected call of FetchBlockchainNetwork.
func (mr *MockSDNHTTPMockRecorder) FetchBlockchainNetwork() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FetchBlockchainNetwork", reflect.TypeOf((*MockSDNHTTP)(nil).FetchBlockchainNetwork))
}

// FetchCustomerAccountModel mocks base method.
func (m *MockSDNHTTP) FetchCustomerAccountModel(accountID types.AccountID) (sdnmessage.Account, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FetchCustomerAccountModel", accountID)
	ret0, _ := ret[0].(sdnmessage.Account)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FetchCustomerAccountModel indicates an expected call of FetchCustomerAccountModel.
func (mr *MockSDNHTTPMockRecorder) FetchCustomerAccountModel(accountID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FetchCustomerAccountModel", reflect.TypeOf((*MockSDNHTTP)(nil).FetchCustomerAccountModel), accountID)
}

// FindNetwork mocks base method.
func (m *MockSDNHTTP) FindNetwork(networkNum types.NetworkNum) (*sdnmessage.BlockchainNetwork, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "FindNetwork", networkNum)
	ret0, _ := ret[0].(*sdnmessage.BlockchainNetwork)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// FindNetwork indicates an expected call of FindNetwork.
func (mr *MockSDNHTTPMockRecorder) FindNetwork(networkNum interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindNetwork", reflect.TypeOf((*MockSDNHTTP)(nil).FindNetwork), networkNum)
}

// FindNewRelay mocks base method.
func (m *MockSDNHTTP) FindNewRelay(gwContext context.Context, oldRelayIP string, relayInstructions chan connections.RelayInstruction) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "FindNewRelay", gwContext, oldRelayIP, relayInstructions)
}

// FindNewRelay indicates an expected call of FindNewRelay.
func (mr *MockSDNHTTPMockRecorder) FindNewRelay(gwContext, oldRelayIP, relayInstructions interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "FindNewRelay", reflect.TypeOf((*MockSDNHTTP)(nil).FindNewRelay), gwContext, oldRelayIP, relayInstructions)
}

// Get mocks base method.
func (m *MockSDNHTTP) Get(endpoint string, requestBody []byte) ([]byte, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Get", endpoint, requestBody)
	ret0, _ := ret[0].([]byte)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Get indicates an expected call of Get.
func (mr *MockSDNHTTPMockRecorder) Get(endpoint, requestBody interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Get", reflect.TypeOf((*MockSDNHTTP)(nil).Get), endpoint, requestBody)
}

// GetQuotaUsage mocks base method.
func (m *MockSDNHTTP) GetQuotaUsage(accountID string) (*connections.QuotaResponseBody, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetQuotaUsage", accountID)
	ret0, _ := ret[0].(*connections.QuotaResponseBody)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// GetQuotaUsage indicates an expected call of GetQuotaUsage.
func (mr *MockSDNHTTPMockRecorder) GetQuotaUsage(accountID interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetQuotaUsage", reflect.TypeOf((*MockSDNHTTP)(nil).GetQuotaUsage), accountID)
}

// InitGateway mocks base method.
func (m *MockSDNHTTP) InitGateway(protocol, network string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "InitGateway", protocol, network)
	ret0, _ := ret[0].(error)
	return ret0
}

// InitGateway indicates an expected call of InitGateway.
func (mr *MockSDNHTTPMockRecorder) InitGateway(protocol, network interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "InitGateway", reflect.TypeOf((*MockSDNHTTP)(nil).InitGateway), protocol, network)
}

// MinTxAge mocks base method.
func (m *MockSDNHTTP) MinTxAge() time.Duration {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "MinTxAge")
	ret0, _ := ret[0].(time.Duration)
	return ret0
}

// MinTxAge indicates an expected call of MinTxAge.
func (mr *MockSDNHTTPMockRecorder) MinTxAge() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "MinTxAge", reflect.TypeOf((*MockSDNHTTP)(nil).MinTxAge))
}

// NeedsRegistration mocks base method.
func (m *MockSDNHTTP) NeedsRegistration() bool {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NeedsRegistration")
	ret0, _ := ret[0].(bool)
	return ret0
}

// NeedsRegistration indicates an expected call of NeedsRegistration.
func (mr *MockSDNHTTPMockRecorder) NeedsRegistration() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NeedsRegistration", reflect.TypeOf((*MockSDNHTTP)(nil).NeedsRegistration))
}

// NetworkNum mocks base method.
func (m *MockSDNHTTP) NetworkNum() types.NetworkNum {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NetworkNum")
	ret0, _ := ret[0].(types.NetworkNum)
	return ret0
}

// NetworkNum indicates an expected call of NetworkNum.
func (mr *MockSDNHTTPMockRecorder) NetworkNum() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NetworkNum", reflect.TypeOf((*MockSDNHTTP)(nil).NetworkNum))
}

// Networks mocks base method.
func (m *MockSDNHTTP) Networks() *sdnmessage.BlockchainNetworks {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Networks")
	ret0, _ := ret[0].(*sdnmessage.BlockchainNetworks)
	return ret0
}

// Networks indicates an expected call of Networks.
func (mr *MockSDNHTTPMockRecorder) Networks() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Networks", reflect.TypeOf((*MockSDNHTTP)(nil).Networks))
}

// NodeID mocks base method.
func (m *MockSDNHTTP) NodeID() types.NodeID {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NodeID")
	ret0, _ := ret[0].(types.NodeID)
	return ret0
}

// NodeID indicates an expected call of NodeID.
func (mr *MockSDNHTTPMockRecorder) NodeID() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NodeID", reflect.TypeOf((*MockSDNHTTP)(nil).NodeID))
}

// NodeModel mocks base method.
func (m *MockSDNHTTP) NodeModel() *sdnmessage.NodeModel {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "NodeModel")
	ret0, _ := ret[0].(*sdnmessage.NodeModel)
	return ret0
}

// NodeModel indicates an expected call of NodeModel.
func (mr *MockSDNHTTPMockRecorder) NodeModel() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "NodeModel", reflect.TypeOf((*MockSDNHTTP)(nil).NodeModel))
}

// Register mocks base method.
func (m *MockSDNHTTP) Register() error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Register")
	ret0, _ := ret[0].(error)
	return ret0
}

// Register indicates an expected call of Register.
func (mr *MockSDNHTTPMockRecorder) Register() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Register", reflect.TypeOf((*MockSDNHTTP)(nil).Register))
}

// SDNURL mocks base method.
func (m *MockSDNHTTP) SDNURL() string {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "SDNURL")
	ret0, _ := ret[0].(string)
	return ret0
}

// SDNURL indicates an expected call of SDNURL.
func (mr *MockSDNHTTPMockRecorder) SDNURL() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SDNURL", reflect.TypeOf((*MockSDNHTTP)(nil).SDNURL))
}

// SendNodeEvent mocks base method.
func (m *MockSDNHTTP) SendNodeEvent(event sdnmessage.NodeEvent, id types.NodeID) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SendNodeEvent", event, id)
}

// SendNodeEvent indicates an expected call of SendNodeEvent.
func (mr *MockSDNHTTPMockRecorder) SendNodeEvent(event, id interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SendNodeEvent", reflect.TypeOf((*MockSDNHTTP)(nil).SendNodeEvent), event, id)
}

// SetNetworks mocks base method.
func (m *MockSDNHTTP) SetNetworks(networks sdnmessage.BlockchainNetworks) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "SetNetworks", networks)
}

// SetNetworks indicates an expected call of SetNetworks.
func (mr *MockSDNHTTPMockRecorder) SetNetworks(networks interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "SetNetworks", reflect.TypeOf((*MockSDNHTTP)(nil).SetNetworks), networks)
}

// MockIgnoredRelaysMap is a mock of IgnoredRelaysMap interface.
type MockIgnoredRelaysMap struct {
	ctrl     *gomock.Controller
	recorder *MockIgnoredRelaysMapMockRecorder
}

// MockIgnoredRelaysMapMockRecorder is the mock recorder for MockIgnoredRelaysMap.
type MockIgnoredRelaysMapMockRecorder struct {
	mock *MockIgnoredRelaysMap
}

// NewMockIgnoredRelaysMap creates a new mock instance.
func NewMockIgnoredRelaysMap(ctrl *gomock.Controller) *MockIgnoredRelaysMap {
	mock := &MockIgnoredRelaysMap{ctrl: ctrl}
	mock.recorder = &MockIgnoredRelaysMapMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockIgnoredRelaysMap) EXPECT() *MockIgnoredRelaysMapMockRecorder {
	return m.recorder
}

// Delete mocks base method.
func (m *MockIgnoredRelaysMap) Delete(key string) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Delete", key)
}

// Delete indicates an expected call of Delete.
func (mr *MockIgnoredRelaysMapMockRecorder) Delete(key interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Delete", reflect.TypeOf((*MockIgnoredRelaysMap)(nil).Delete), key)
}

// Load mocks base method.
func (m *MockIgnoredRelaysMap) Load(key string) (connections.RelyInfo, bool) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Load", key)
	ret0, _ := ret[0].(connections.RelyInfo)
	ret1, _ := ret[1].(bool)
	return ret0, ret1
}

// Load indicates an expected call of Load.
func (mr *MockIgnoredRelaysMapMockRecorder) Load(key interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Load", reflect.TypeOf((*MockIgnoredRelaysMap)(nil).Load), key)
}

// Store mocks base method.
func (m *MockIgnoredRelaysMap) Store(key string, value connections.RelyInfo) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Store", key, value)
}

// Store indicates an expected call of Store.
func (mr *MockIgnoredRelaysMapMockRecorder) Store(key, value interface{}) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Store", reflect.TypeOf((*MockIgnoredRelaysMap)(nil).Store), key, value)
}
