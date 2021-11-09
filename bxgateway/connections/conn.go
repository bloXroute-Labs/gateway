package connections

import (
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/bxmessage"
	log "github.com/sirupsen/logrus"
	"time"
)

// ConnHandler defines the methods needed to handle bloxroute connections
type ConnHandler interface {
	ProcessMessage(msg bxmessage.MessageBytes)
}

// Conn defines a network interface that sends and receives messages
type Conn interface {
	ID() Socket
	Info() Info
	IsOpen() bool

	Protocol() bxmessage.Protocol
	SetProtocol(bxmessage.Protocol)

	Log() *log.Entry

	Connect() error
	ReadMessages(callBack func(bxmessage.MessageBytes), readDeadline time.Duration, headerLen int, readPayloadLen func([]byte) int) (int, error)
	Send(msg bxmessage.Message) error
	SendWithDelay(msg bxmessage.Message, delay time.Duration) error
	Close(reason string) error
}

// NodeStatus defines any metadata a connection may need to know about the running node. This is expected to be rarely necessary.
type NodeStatus struct {
	// temporary property for sending sync status with pings
	TransactionServiceSynced bool
}

// MsgHandlingOptions represents background/foreground options for message handling
type MsgHandlingOptions bool

// MsgHandlingOptions enumeration
const (
	RunBackground MsgHandlingOptions = true
	RunForeground MsgHandlingOptions = false
)

// BxListener defines a struct that is capable of processing bloxroute messages
type BxListener interface {
	NodeStatus() NodeStatus
	HandleMsg(msg bxmessage.Message, conn Conn, background MsgHandlingOptions) error

	// OnConnEstablished is a callback for when a connection has been connected and finished its handshake
	OnConnEstablished(conn Conn) error

	// OnConnClosed is a callback for when a connection is closed with no expectation of retrying
	OnConnClosed(conn Conn) error
}

// ConnList represents the set of connections a node is maintaining
type ConnList []Conn
