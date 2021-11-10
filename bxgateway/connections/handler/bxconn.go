package handler

import (
	"container/list"
	"encoding/binary"
	"fmt"
	"github.com/bloXroute-Labs/gateway/bxgateway"
	"github.com/bloXroute-Labs/gateway/bxgateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/bxgateway/connections"
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"github.com/bloXroute-Labs/gateway/bxgateway/utils"
	log "github.com/sirupsen/logrus"
	"math"
	"sync"
	"time"
)

const (
	connTimeout = 5 * time.Second
)

// BxConn is a connection to any other bloxroute Node. BxConn implements connections.ConnHandler.
type BxConn struct {
	connections.Conn

	Node    connections.BxListener
	Handler connections.ConnHandler

	lock                  *sync.Mutex
	connectionEstablished bool
	closed                bool
	nodeID                types.NodeID
	peerID                types.NodeID
	accountID             types.AccountID
	minToRelay            int64 // time in microseconds
	minFromRelay          int64 // time in microseconds
	minRoundTrip          int64 // time in microseconds
	slowCount             int64
	connectionType        utils.NodeType
	stringRepresentation  string
	onPongMsgs            *list.List
	networkNum            types.NetworkNum
	localGEO              bool
	privateNetwork        bool
	localPort             int64
	log                   *log.Entry
	clock                 utils.Clock
}

// NewBxConn constructs a connection to a bloxroute node.
func NewBxConn(node connections.BxListener, connect func() (connections.Socket, error), handler connections.ConnHandler,
	sslCerts *utils.SSLCerts, ip string, port int64, nodeID types.NodeID, connectionType utils.NodeType,
	usePQ bool, logMessages bool, localGEO bool, privateNetwork bool, localPort int64, clock utils.Clock) *BxConn {
	bc := &BxConn{
		Conn:           connections.NewSSLConnection(connect, sslCerts, ip, port, bxmessage.CurrentProtocol, usePQ, logMessages, bxgateway.MaxConnectionBacklog, clock),
		Node:           node,
		Handler:        handler,
		nodeID:         nodeID,
		minFromRelay:   math.MaxInt64,
		minToRelay:     math.MaxInt64,
		minRoundTrip:   math.MaxInt64,
		connectionType: connectionType,
		lock:           &sync.Mutex{},
		onPongMsgs:     list.New(),
		localGEO:       localGEO,
		privateNetwork: privateNetwork,
		localPort:      localPort,
		log: log.WithFields(log.Fields{
			"connType":   connectionType,
			"remoteAddr": "<connecting>",
		}),
		clock: clock,
	}
	bc.stringRepresentation = fmt.Sprintf("%v/%v@<connecting...>", connectionType, bc.Conn)
	return bc
}

// Start kicks off main goroutine of the connection
func (b *BxConn) Start() error {
	go b.readLoop()
	return nil
}

// Send sends a message to the peer.
func (b *BxConn) Send(msg bxmessage.Message) error {
	if msg.GetPriority() != bxmessage.OnPongPriority {
		return b.Conn.Send(msg)
	}
	b.lock.Lock()
	defer b.lock.Unlock()
	b.onPongMsgs.PushBack(msg)
	// if we added the first OnPongPriority message, request a pong from the peer
	if b.onPongMsgs.Len() == 1 {
		// send ping to handle the next message
		ping := &bxmessage.Ping{}
		_ = b.Conn.Send(ping)
	}
	return nil
}

// Info returns connection metadata
func (b *BxConn) Info() connections.Info {
	meta := b.Conn.Info()
	return connections.Info{
		NodeID:          b.peerID,
		AccountID:       b.accountID,
		PeerIP:          meta.PeerIP,
		PeerPort:        meta.PeerPort,
		LocalPort:       b.localPort,
		ConnectionState: "todo",
		ConnectionType:  b.connectionType,
		FromMe:          meta.FromMe,
		NetworkNum:      b.networkNum,
		LocalGEO:        b.localGEO,
		PrivateNetwork:  b.privateNetwork,
	}
}

// IsOpen returns when the connection is ready for broadcasting
func (b *BxConn) IsOpen() bool {
	return b.Conn.IsOpen() && b.connectionEstablished
}

// Log returns the connection context logger
func (b *BxConn) Log() *log.Entry {
	return b.log
}

// Connect establishes the connections.SSLConn if necessary, then sets attributes based on
// the provided SSL certificates
func (b *BxConn) Connect() error {
	err := b.Conn.Connect()
	if err != nil {
		return err
	}

	connInfo := b.Conn.Info()
	b.peerID = connInfo.NodeID
	b.accountID = connInfo.AccountID

	b.stringRepresentation = fmt.Sprintf("%v/%v@%v{%v}", b.connectionType, b.Conn, b.accountID, b.peerID)
	b.log = log.WithFields(log.Fields{
		"connType":   b.connectionType,
		"remoteAddr": b.Conn,
		"accountID":  b.accountID,
		"peerID":     b.peerID,
	})
	return nil
}

// Close marks the connection for termination from this Node's side. Close will not allow this connection to be retried. Close will not actually stop this connection's event loop, only trigger the readLoop to exit, which will then close the event loop.
func (b *BxConn) Close(reason string) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.closed {
		return nil
	}

	err := b.closeWithRetry(reason)
	b.closed = true

	return err
}

// ProcessMessage constructs a message from the buffer and handles it
// This method only handles messages that do not require querying the BxListener interface
func (b *BxConn) ProcessMessage(msg bxmessage.MessageBytes) {
	msgType := msg.BxType()
	switch msgType {
	case bxmessage.HelloType:
		helloMsg := &bxmessage.Hello{}
		_ = helloMsg.Unpack(msg, 0)
		if helloMsg.Protocol < bxmessage.MinProtocol {
			b.Log().Warnf("can't establish connection - proposed protocol %v is below the minimum supported %v",
				helloMsg.Protocol, bxmessage.MinProtocol)
			_ = b.Close("protocol version not supported")
			return
		}
		if b.Protocol() > helloMsg.Protocol {
			b.SetProtocol(helloMsg.Protocol)
		}
		b.networkNum = helloMsg.GetNetworkNum()
		b.Log().Debugf("completed handshake: network %v, protocol %v, peer id %v ", b.networkNum, b.Protocol(), b.peerID)
		ack := bxmessage.Ack{}
		_ = b.Send(&ack)
		if !b.Info().FromMe {
			hello := bxmessage.Hello{NodeID: b.nodeID, Protocol: b.Protocol()}
			hello.SetNetworkNum(b.networkNum)
			_ = b.Send(&hello)
		} else {
			b.setConnectionEstablished()
		}
	case bxmessage.AckType:
		b.lock.Lock()
		// avoid racing with Close
		if !b.Info().FromMe {
			b.setConnectionEstablished()
		}
		b.lock.Unlock()

	case bxmessage.PingType:
		ping := &bxmessage.Ping{}
		_ = ping.Unpack(msg, b.Protocol())
		b.msgPing(ping)

	case bxmessage.PongType:
		pong := &bxmessage.Pong{}
		_ = pong.Unpack(msg, b.Protocol())
		b.msgPong(pong)
		// now, check if there are queued message that should be delivered on pong message
		b.lock.Lock()
		defer b.lock.Unlock()

		if b.onPongMsgs.Len() == 0 {
			break
		}

		onPongMsg := b.onPongMsgs.Front()
		msg := onPongMsg.Value.(bxmessage.Message)
		msg.SetPriority(bxmessage.HighestPriority)
		_ = b.Conn.Send(msg)
		b.onPongMsgs.Remove(onPongMsg)

		if b.onPongMsgs.Len() > 0 {
			// send ping to handle the next message
			ping := &bxmessage.Ping{}
			_ = b.Conn.Send(ping)
		}
	case bxmessage.BroadcastType:
		block := &bxmessage.Broadcast{}
		err := block.Unpack(msg, b.Protocol())
		if err != nil {
			b.Log().Errorf("could not unpack broadcast message: %v. Failed bytes: %v", err, msg)
			return
		}
		_ = b.Node.HandleMsg(block, b, connections.RunForeground)

	case bxmessage.TxCleanupType:
		txcleanup := &bxmessage.TxCleanup{}
		_ = txcleanup.Unpack(msg, b.Protocol())
		_ = b.Node.HandleMsg(txcleanup, b, connections.RunBackground)
	case bxmessage.BlockConfirmationTYpe:
		blockConfirmation := &bxmessage.BlockConfirmation{}
		_ = blockConfirmation.Unpack(msg, b.Protocol())
		_ = b.Node.HandleMsg(blockConfirmation, b, connections.RunBackground)
	case bxmessage.SyncReqType:
		syncReq := &bxmessage.SyncReq{}
		_ = syncReq.Unpack(msg, b.Protocol())
		b.Log().Debugf("TxStore sync: got a request for network %v", syncReq.GetNetworkNum())
		_ = b.Node.HandleMsg(syncReq, b, connections.RunBackground)

	default:
		b.Log().Debugf("read %v (%d bytes)", msgType, len(msg))
	}
}

// SetNetworkNum is used to specify the connection network number
func (b *BxConn) SetNetworkNum(networkNum types.NetworkNum) {
	b.networkNum = networkNum
}

func (b *BxConn) setConnectionEstablished() {
	b.connectionEstablished = true
	_ = b.Node.OnConnEstablished(b)
}

func (b *BxConn) msgPing(ping *bxmessage.Ping) {
	pong := &bxmessage.Pong{Nonce: ping.Nonce}
	_ = b.Send(pong)
	b.handleNonces(0, ping.Nonce)
}

func (b *BxConn) msgPong(pong *bxmessage.Pong) {
	b.handleNonces(pong.Nonce, pong.TimeStamp)
}

func (b *BxConn) handleNonces(nodeNonce, peerNonce uint64) {
	nonceNow := b.clock.Now().UnixNano() / 1000
	timeFromPeer := nonceNow - int64(peerNonce)
	if timeFromPeer < b.minFromRelay {
		b.minFromRelay = timeFromPeer
	}
	if timeFromPeer > b.minFromRelay+bxgateway.SlowPingPong &&
		(!utils.IsGateway || log.GetLevel() > log.InfoLevel) {
		b.slowCount++
		if utils.IsGateway || b.privateNetwork || b.localGEO {
			b.Log().Debugf("slow message: took %v ms vs minimum %v ms", timeFromPeer/1000, b.minFromRelay/1000)
		}
	}
	// if this is a ping message we are done.
	if nodeNonce == 0 {
		return
	}

	// pong message.
	timeToPeer := int64(peerNonce) - int64(nodeNonce)
	if timeToPeer < b.minToRelay {
		b.minToRelay = timeToPeer
	}
	if timeToPeer > b.minToRelay+bxgateway.SlowPingPong &&
		(!utils.IsGateway || log.GetLevel() > log.InfoLevel) {
		b.slowCount++
		if utils.IsGateway || b.privateNetwork || b.localGEO {
			b.Log().Debugf("slow message: took %v ms vs minimum %v ms", timeToPeer/1000, b.minToRelay/1000)
		}
	}
	roundTrip := nonceNow - int64(nodeNonce)
	if roundTrip < b.minRoundTrip {
		b.minRoundTrip = roundTrip
	}

}

// readLoop connects and reads messages from the socket.
// If we are the initiator of the connection we auto-recover on disconnect.
func (b *BxConn) readLoop() {
	isInitiator := b.Info().FromMe
	for {
		err := b.Connect()
		if err != nil {
			b.Log().Errorf("encountered connection error while connecting: %v", err)

			reason := "could not connect to remote"
			if !isInitiator {
				_ = b.Close(reason)
				break
			}

			_ = b.closeWithRetry(reason)
			// sleep before next connection attempt
			b.clock.Sleep(connTimeout)
			continue
		}

		if isInitiator {
			hello := bxmessage.Hello{NodeID: b.nodeID, Protocol: b.Protocol()}
			hello.SetNetworkNum(b.networkNum)
			_ = b.Send(&hello)
		}

		closeReason := "read loop closed"
		for b.Conn.IsOpen() {
			_, err := b.ReadMessages(b.Handler.ProcessMessage, 30*time.Second, bxmessage.HeaderLen,
				func(b []byte) int {
					return int(binary.LittleEndian.Uint32(b[bxmessage.PayloadSizeOffset:]))
				})

			if err != nil {
				closeReason = err.Error()
				b.Log().Tracef("connection closed: %v", err)
				break
			}
		}
		if !isInitiator {
			_ = b.Close(closeReason)
			break
		}

		_ = b.closeWithRetry(closeReason)
		if b.closed {
			break
		}
		// sleep before next connection attempt
		// note - in docker environment the docker-proxy may keep the port open after the docker was stopped. we
		// need this sleep to avoid fast connect/disconnect loop
		b.clock.Sleep(connTimeout)
	}
}

// IsBloxroute detect if the peer belongs to bloxroute
func (b BxConn) IsBloxroute() bool {
	return b.accountID == types.BloxrouteAccountID
}

// String represents a string conversion of this connection
func (b BxConn) String() string {
	return b.stringRepresentation
}

// closeWithRetry does not shutdown the main go routines present in BxConn, only the ones in the ssl connection, which can be restarted on the next Connect
func (b *BxConn) closeWithRetry(reason string) error {
	b.connectionEstablished = false
	_ = b.Node.OnConnClosed(b)
	return b.Conn.Close(reason)
}

// GetMinLatencies exposes the best latencies in ms form and to peer
func (b BxConn) GetMinLatencies() (int64, int64, int64, int64) {
	return b.minFromRelay / 1000, b.minToRelay / 1000, b.slowCount, b.minRoundTrip / 1000
}
