package bxmock

import (
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
)

// MockBxListener is a flexible struct that implements connections.BxListener
type MockBxListener struct{}

// NodeStatus returns an empty status
func (m MockBxListener) NodeStatus() connections.NodeStatus {
	return connections.NodeStatus{}
}

// HandleMsg does nothing
func (m MockBxListener) HandleMsg(msg bxmessage.Message, conn connections.Conn, background connections.MsgHandlingOptions) error {
	return nil
}

// OnConnEstablished does nothing
func (m MockBxListener) OnConnEstablished(conn connections.Conn) error {
	return nil
}

// OnConnClosed does nothing
func (m MockBxListener) OnConnClosed(conn connections.Conn) error {
	return nil
}

// ValidateConnection does nothing
func (m MockBxListener) ValidateConnection(conn connections.Conn) error {
	return nil
}
