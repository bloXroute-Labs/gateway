// Code generated by MockGen. DO NOT EDIT.
// Source: ./servers/grpc/server.go
//
// Generated by this command:
//
//	mockgen -destination ./test/mock/connector_mock.go -package mock -source ./servers/grpc/server.go Connector
//

// Package mock is a generated GoMock package.
package mock

import (
	reflect "reflect"

	bxmessage "github.com/bloXroute-Labs/gateway/v2/bxmessage"
	connections "github.com/bloXroute-Labs/gateway/v2/connections"
	feed "github.com/bloXroute-Labs/gateway/v2/services/feed"
	types "github.com/bloXroute-Labs/gateway/v2/types"
	utils "github.com/bloXroute-Labs/gateway/v2/utils"
	jsonrpc2 "github.com/sourcegraph/jsonrpc2"
	gomock "go.uber.org/mock/gomock"
)

// MockConnector is a mock of Connector interface.
type MockConnector struct {
	ctrl     *gomock.Controller
	recorder *MockConnectorMockRecorder
}

// MockConnectorMockRecorder is the mock recorder for MockConnector.
type MockConnectorMockRecorder struct {
	mock *MockConnector
}

// NewMockConnector creates a new mock instance.
func NewMockConnector(ctrl *gomock.Controller) *MockConnector {
	mock := &MockConnector{ctrl: ctrl}
	mock.recorder = &MockConnectorMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockConnector) EXPECT() *MockConnectorMockRecorder {
	return m.recorder
}

// Broadcast mocks base method.
func (m *MockConnector) Broadcast(msg bxmessage.Message, source connections.Conn, to utils.NodeType) types.BroadcastResults {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Broadcast", msg, source, to)
	ret0, _ := ret[0].(types.BroadcastResults)
	return ret0
}

// Broadcast indicates an expected call of Broadcast.
func (mr *MockConnectorMockRecorder) Broadcast(msg, source, to any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Broadcast", reflect.TypeOf((*MockConnector)(nil).Broadcast), msg, source, to)
}

// Peers mocks base method.
func (m *MockConnector) Peers(peerType string) []bxmessage.PeerInfo {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Peers", peerType)
	ret0, _ := ret[0].([]bxmessage.PeerInfo)
	return ret0
}

// Peers indicates an expected call of Peers.
func (mr *MockConnectorMockRecorder) Peers(peerType any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Peers", reflect.TypeOf((*MockConnector)(nil).Peers), peerType)
}

// Relays mocks base method.
func (m *MockConnector) Relays() map[string]bxmessage.RelayConnectionInfo {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Relays")
	ret0, _ := ret[0].(map[string]bxmessage.RelayConnectionInfo)
	return ret0
}

// Relays indicates an expected call of Relays.
func (mr *MockConnectorMockRecorder) Relays() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Relays", reflect.TypeOf((*MockConnector)(nil).Relays))
}

// MockfeedManager is a mock of feedManager interface.
type MockfeedManager struct {
	ctrl     *gomock.Controller
	recorder *MockfeedManagerMockRecorder
}

// MockfeedManagerMockRecorder is the mock recorder for MockfeedManager.
type MockfeedManagerMockRecorder struct {
	mock *MockfeedManager
}

// NewMockfeedManager creates a new mock instance.
func NewMockfeedManager(ctrl *gomock.Controller) *MockfeedManager {
	mock := &MockfeedManager{ctrl: ctrl}
	mock.recorder = &MockfeedManagerMockRecorder{mock}
	return mock
}

// EXPECT returns an object that allows the caller to indicate expected use.
func (m *MockfeedManager) EXPECT() *MockfeedManagerMockRecorder {
	return m.recorder
}

// GetGrpcSubscriptionReply mocks base method.
func (m *MockfeedManager) GetGrpcSubscriptionReply() []feed.ClientSubscriptionFullInfo {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "GetGrpcSubscriptionReply")
	ret0, _ := ret[0].([]feed.ClientSubscriptionFullInfo)
	return ret0
}

// GetGrpcSubscriptionReply indicates an expected call of GetGrpcSubscriptionReply.
func (mr *MockfeedManagerMockRecorder) GetGrpcSubscriptionReply() *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "GetGrpcSubscriptionReply", reflect.TypeOf((*MockfeedManager)(nil).GetGrpcSubscriptionReply))
}

// Notify mocks base method.
func (m *MockfeedManager) Notify(notification types.Notification) {
	m.ctrl.T.Helper()
	m.ctrl.Call(m, "Notify", notification)
}

// Notify indicates an expected call of Notify.
func (mr *MockfeedManagerMockRecorder) Notify(notification any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Notify", reflect.TypeOf((*MockfeedManager)(nil).Notify), notification)
}

// Subscribe mocks base method.
func (m *MockfeedManager) Subscribe(feedName types.FeedType, feedConnectionType types.FeedConnectionType, conn *jsonrpc2.Conn, ci types.ClientInfo, ro types.ReqOptions, ethSubscribe bool) (*feed.ClientSubscriptionHandlingInfo, error) {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Subscribe", feedName, feedConnectionType, conn, ci, ro, ethSubscribe)
	ret0, _ := ret[0].(*feed.ClientSubscriptionHandlingInfo)
	ret1, _ := ret[1].(error)
	return ret0, ret1
}

// Subscribe indicates an expected call of Subscribe.
func (mr *MockfeedManagerMockRecorder) Subscribe(feedName, feedConnectionType, conn, ci, ro, ethSubscribe any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Subscribe", reflect.TypeOf((*MockfeedManager)(nil).Subscribe), feedName, feedConnectionType, conn, ci, ro, ethSubscribe)
}

// Unsubscribe mocks base method.
func (m *MockfeedManager) Unsubscribe(subscriptionID string, closeClientConnection bool, errMsg string) error {
	m.ctrl.T.Helper()
	ret := m.ctrl.Call(m, "Unsubscribe", subscriptionID, closeClientConnection, errMsg)
	ret0, _ := ret[0].(error)
	return ret0
}

// Unsubscribe indicates an expected call of Unsubscribe.
func (mr *MockfeedManagerMockRecorder) Unsubscribe(subscriptionID, closeClientConnection, errMsg any) *gomock.Call {
	mr.mock.ctrl.T.Helper()
	return mr.mock.ctrl.RecordCallWithMethodType(mr.mock, "Unsubscribe", reflect.TypeOf((*MockfeedManager)(nil).Unsubscribe), subscriptionID, closeClientConnection, errMsg)
}