package connections

import (
	"fmt"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/bxmessage"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/utils"
	log "github.com/sirupsen/logrus"
	"time"
)

// Blockchain is a placeholder struct to represent a connection for blockchain nodes
type Blockchain struct {
	endpoint types.NodeEndpoint
	log      *log.Entry
}

var blockchainTLSPlaceholder = TLS{}

// NewBlockchainConn return a new instance of the Blockchain placeholder connection
func NewBlockchainConn(ipEndpoint types.NodeEndpoint) Blockchain {
	return Blockchain{
		endpoint: ipEndpoint,
		log: log.WithFields(log.Fields{
			"connType":   utils.Blockchain.String(),
			"remoteAddr": fmt.Sprintf("%v:%v", ipEndpoint.IP, ipEndpoint.Port),
		}),
	}
}

// Info returns connection metadata
func (b Blockchain) Info() Info {
	return Info{
		ConnectionType: utils.Blockchain,
		NetworkNum:     types.AllNetworkNum,
		PeerIP:         b.endpoint.IP,
		PeerPort:       int64(b.endpoint.Port),
		PeerEnode:      b.endpoint.PublicKey,
	}
}

// ID returns placeholder
func (b Blockchain) ID() Socket {
	return blockchainTLSPlaceholder
}

// IsOpen is never true, since the Blockchain is not writable
func (b Blockchain) IsOpen() bool {
	return false
}

// Protocol indicates that the Blockchain does not have a protocol
func (b Blockchain) Protocol() bxmessage.Protocol {
	return bxmessage.EmptyProtocol
}

// SetProtocol is a no-op
func (b Blockchain) SetProtocol(protocol bxmessage.Protocol) {
}

// Log returns the blockchain connection logger
func (b Blockchain) Log() *log.Entry {
	return b.log
}

// Connect is a no-op
func (b Blockchain) Connect() error {
	return nil
}

// ReadMessages is a no-op
func (b Blockchain) ReadMessages(callBack func(bxmessage.MessageBytes), readDeadline time.Duration, headerLen int, readPayloadLen func([]byte) int) (int, error) {
	return 0, nil
}

// Send is a no-op
func (b Blockchain) Send(msg bxmessage.Message) error {
	return nil
}

// SendWithDelay is a no-op
func (b Blockchain) SendWithDelay(msg bxmessage.Message, delay time.Duration) error {
	return nil
}

// Close is a no-op
func (b Blockchain) Close(reason string) error {
	return nil
}

// String returns the formatted representation of this placeholder connection
func (b Blockchain) String() string {
	return fmt.Sprintf("Blockchain %v", b.endpoint.String())
}
