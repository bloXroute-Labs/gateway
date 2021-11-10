package connections

import (
	"github.com/bloXroute-Labs/gateway/bxgateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"github.com/bloXroute-Labs/gateway/bxgateway/utils"
	log "github.com/sirupsen/logrus"
	"time"
)

var rpcTLSConn = TLS{}

// RPCConn is a placeholder struct to represent connection requests from RPC transaction requests
type RPCConn struct {
	AccountID     types.AccountID
	RemoteAddress string
	networkNum    types.NetworkNum
	log           *log.Entry
}

// NewRPCConn return a new instance of RPCConn
func NewRPCConn(accountID types.AccountID, remoteAddr string, networkNum types.NetworkNum) RPCConn {
	return RPCConn{
		AccountID:     accountID,
		RemoteAddress: remoteAddr,
		networkNum:    networkNum,
		log: log.WithFields(log.Fields{
			"connType":   "RPC",
			"remoteAddr": remoteAddr,
			"accountID":  accountID,
		}),
	}
}

// ID identifies the underlying socket
func (r RPCConn) ID() Socket {
	return rpcTLSConn
}

// Info returns connection metadata
func (r RPCConn) Info() Info {
	return Info{
		AccountID:      r.AccountID,
		ConnectionType: utils.Websocket,
		NetworkNum:     r.networkNum,
	}
}

// IsOpen is never true, since the RPCConn is not writable
func (r RPCConn) IsOpen() bool {
	return false
}

// Protocol indicates that the RPCConn does not have a protocol
func (r RPCConn) Protocol() bxmessage.Protocol {
	return bxmessage.EmptyProtocol
}

// SetProtocol is a no-op
func (r RPCConn) SetProtocol(protocol bxmessage.Protocol) {
}

// Connect is a no-op
func (r RPCConn) Connect() error {
	return nil
}

// Log returns the context logger for the RPC connection
func (r RPCConn) Log() *log.Entry {
	return r.log
}

// ReadMessages is a no-op
func (r RPCConn) ReadMessages(callBack func(bxmessage.MessageBytes), readDeadline time.Duration, headerLen int, readPayloadLen func([]byte) int) (int, error) {
	return 0, nil
}

// Send is a no-op
func (r RPCConn) Send(msg bxmessage.Message) error {
	return nil
}

// SendWithDelay is a no-op
func (r RPCConn) SendWithDelay(msg bxmessage.Message, delay time.Duration) error {
	return nil
}

// Close is a no-op
func (r RPCConn) Close(reason string) error {
	return nil
}

// String returns the formatted representation of this placeholder connection
func (r RPCConn) String() string {
	accountID := string(r.AccountID)
	return "websockets connection with remote address: " + r.RemoteAddress + ", account id:" + accountID
}
