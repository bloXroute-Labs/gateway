package handler

import (
	"container/list"
	"context"
	"encoding/binary"
	"fmt"
	"math"
	"sync"
	"time"

	"github.com/bloXroute-Labs/gateway/v2"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const (
	connTimeout        = 5 * time.Second
	receiveChannelSize = 500
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
	capabilities          types.CapabilityFlags
	clientVersion         string
	sameRegion            bool
	connectedAt           time.Time
	receiveChan           chan bxmessage.MessageBytes
}

// NewBxConn constructs a connection to a bloxroute node.
func NewBxConn(node connections.BxListener, connect func() (connections.Socket, error), handler connections.ConnHandler,
	sslCerts *utils.SSLCerts, ip string, port int64, nodeID types.NodeID, connectionType utils.NodeType,
	usePQ bool, logMessages bool, localGEO bool, privateNetwork bool, localPort int64, clock utils.Clock,
	sameRegion bool,
) *BxConn {
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
			"connType":   connectionType.String(),
			"remoteAddr": "<connecting>",
		}),
		clock:       clock,
		sameRegion:  sameRegion,
		receiveChan: make(chan bxmessage.MessageBytes, receiveChannelSize),
	}
	bc.stringRepresentation = fmt.Sprintf("%v/%v@<connecting...>", connectionType, bc.Conn)
	go bc.readFromChannel()

	return bc
}

// readFromChannel reads from receiveChan and send it to ProcessMessage, the go routing terminates when the channel is closed
func (b *BxConn) readFromChannel() {
	for msg := range b.receiveChan {
		msg.SetWaitingDuration()
		b.Handler.ProcessMessage(msg)
	}
}

// Start kicks off main read loop
func (b *BxConn) Start(ctx context.Context) {
	b.readLoop(ctx)
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

// GetNodeID return node ID
func (b *BxConn) GetNodeID() types.NodeID { return b.peerID }

// GetLocalPort return local port
func (b *BxConn) GetLocalPort() int64 { return b.localPort }

// GetVersion return version
func (b *BxConn) GetVersion() string { return b.clientVersion }

// GetCapabilities return capabilities
func (b *BxConn) GetCapabilities() types.CapabilityFlags { return b.capabilities }

// GetConnectionType returns type of the connection
func (b *BxConn) GetConnectionType() utils.NodeType { return b.connectionType }

// GetConnectionState returns state of the connection
func (b *BxConn) GetConnectionState() string { return "todo" }

// GetConnectedAt gets ttime of connection
func (b *BxConn) GetConnectedAt() time.Time { return b.connectedAt }

// GetNetworkNum returns network number
func (b *BxConn) GetNetworkNum() types.NetworkNum { return b.networkNum }

// IsLocalGEO indicates if the peer is form the same GEO as we (China vs non-China)
func (b *BxConn) IsLocalGEO() bool { return b.localGEO }

// IsSameRegion indicates if the peer is from the same region as we (us-east1, eu-west1, ...)
func (b *BxConn) IsSameRegion() bool { return b.sameRegion }

// IsPrivateNetwork indicates of the peer connection is over a private network (CEN)
func (b *BxConn) IsPrivateNetwork() bool { return b.privateNetwork }

// GetAccountID return account ID
func (b *BxConn) GetAccountID() types.AccountID { return b.accountID }

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

	b.peerID = b.Conn.GetNodeID()
	b.accountID = b.Conn.GetAccountID()
	b.connectedAt = b.clock.Now()
	b.stringRepresentation = fmt.Sprintf("%v/%v@%v{%v}", b.connectionType, b.Conn, b.accountID, b.peerID)

	b.log = log.WithFields(log.Fields{
		"connType":   b.connectionType.String(),
		"remoteAddr": fmt.Sprint(b.Conn),
		"accountID":  b.accountID,
		"peerID":     b.peerID,
	})

	return nil
}

// Close marks the connection for termination from this Node's side. Close will not allow this connection to be retried. Close will not actually stop this connection's event loop, only trigger the read to exit, which will then close the event loop.
func (b *BxConn) Close(reason string) error {
	b.lock.Lock()
	defer b.lock.Unlock()

	if b.closed {
		return nil
	}

	b.closed = true
	err := b.closeWithRetry(reason)

	return err
}

// ProcessMessage constructs a message from the buffer and handles it
// This method only handles messages that do not require querying the BxListener interface
func (b *BxConn) ProcessMessage(msgBytes bxmessage.MessageBytes) {
	msgType := msgBytes.BxType()
	msg := msgBytes.Raw()
	switch msgType {
	case bxmessage.HelloType:
		helloMsg := &bxmessage.Hello{}
		if err := helloMsg.Unpack(msg, 0); err != nil {
			b.Log().Errorf("could not unpack hello message %v. Failed bytes: %v", err, msg)
			return
		}
		if err := b.Node.HandleMsg(helloMsg, b, connections.RunForeground); err != nil {
			return
		}

		// conn previous protocol version can be different from current
		// thats why we need to compare it with current protocol
		if bxmessage.CurrentProtocol > helloMsg.Protocol {
			b.SetProtocol(helloMsg.Protocol)
		} else {
			// if protocol version is higher or equal than current, update with latest supported - current protocol
			// conn can upgrade few versions at once
			b.SetProtocol(bxmessage.CurrentProtocol)
		}

		b.networkNum = helloMsg.GetNetworkNum()
		b.capabilities = helloMsg.Capabilities
		b.clientVersion = helloMsg.ClientVersion

		b.Log().Debugf("completed handshake: network %v, protocol %v, peer id %v ", b.networkNum, b.Protocol(), b.peerID)

		if err := b.Node.ValidateConnection(b); err != nil {
			// invalid connection has been disabled
			b.setConnectionEstablished()
			return
		}

		ack := bxmessage.Ack{}
		_ = b.Send(&ack)
		if !b.IsInitiator() {
			hello := bxmessage.Hello{NodeID: b.nodeID, Protocol: b.Protocol()}
			hello.SetNetworkNum(b.networkNum)
			_ = b.Send(&hello)
		} else {
			b.setConnectionEstablished()
		}
	case bxmessage.AckType:
		b.lock.Lock()
		// avoid racing with Close
		if !b.IsInitiator() {
			b.setConnectionEstablished()
		}
		b.lock.Unlock()

	case bxmessage.PingType:
		ping := &bxmessage.Ping{}
		if err := ping.Unpack(msg, b.Protocol()); err != nil {
			b.Log().Errorf("could not unpack ping message %v. Failed bytes: %v", err, msg)
			return
		}
		b.msgPing(ping)

	case bxmessage.PongType:
		pong := &bxmessage.Pong{}
		if err := pong.Unpack(msg, b.Protocol()); err != nil {
			b.Log().Errorf("could not unpack pong message %v. Failed bytes: %v", err, msg)
			return
		}
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
	case bxmessage.ValidatorUpdatesType:
		vu := &bxmessage.ValidatorUpdates{}
		if err := vu.Unpack(msg, b.Protocol()); err != nil {
			b.Log().Errorf("could not unpack validator update message %v. Failed bytes: %v", err, msg)
			return
		}
		_ = b.Node.HandleMsg(vu, b, connections.RunForeground)
	case bxmessage.BroadcastType:
		block := &bxmessage.Broadcast{}
		if err := block.Unpack(msg, b.Protocol()); err != nil {
			b.Log().Errorf("could not unpack broadcast message: %v. Failed bytes: %v", err, msg)
			return
		}
		// broadcast message should not be processed in background.
		// Background may be delayed by cleanup messages and sync requests from gws
		_ = b.Node.HandleMsg(block, b, connections.RunForeground)

	case bxmessage.TxCleanupType:
		txcleanup := &bxmessage.TxCleanup{}
		if err := txcleanup.Unpack(msg, b.Protocol()); err != nil {
			b.Log().Errorf("could not unpack txcleanup message: %v. Failed bytes: %v", err, msg)
			return
		}
		_ = b.Node.HandleMsg(txcleanup, b, connections.RunBackground)
	case bxmessage.BlockConfirmationType:
		blockConfirmation := &bxmessage.BlockConfirmation{}
		if err := blockConfirmation.Unpack(msg, b.Protocol()); err != nil {
			b.Log().Errorf("could not unpack block confirmation message: %v. Failed bytes: %v", err, msg)
			return
		}
		_ = b.Node.HandleMsg(blockConfirmation, b, connections.RunBackground)
	case bxmessage.SyncReqType:
		syncReq := &bxmessage.SyncReq{}
		if err := syncReq.Unpack(msg, b.Protocol()); err != nil {
			b.Log().Errorf("could not unpack sync message: %v. Failed bytes: %v", err, msg)
			return
		}
		b.Log().Debugf("TxStore sync: got a request for network %v", syncReq.GetNetworkNum())
		_ = b.Node.HandleMsg(syncReq, b, connections.RunBackground)
	case bxmessage.ErrorNotificationType:
		errorNotification := &bxmessage.ErrorNotification{}
		if err := errorNotification.Unpack(msg, b.Protocol()); err != nil {
			b.Log().Errorf("could not unpack error notification message: %v. Failed bytes: %v", err, msg)
			return
		}
		_ = b.Node.HandleMsg(errorNotification, b, connections.RunForeground)
	case bxmessage.MEVBundleType:
		mevBundle := &bxmessage.MEVBundle{}
		if err := mevBundle.Unpack(msg, b.Protocol()); err != nil {
			b.log.Warnf("Failed to unpack mevBundle bxmessage: %v", err)
			return
		}
		_ = b.Node.HandleMsg(mevBundle, b, connections.RunForeground)
	case bxmessage.IntentType:
		intent, err := bxmessage.UnpackIntent(msg, b.Protocol())
		if err != nil {
			b.log.Warnf("failed to unpack intent message: %v", err)
			return
		}

		_ = b.Node.HandleMsg(intent, b, connections.RunBackground) //nolint:errcheck
	case bxmessage.IntentSolutionType:
		solution, err := bxmessage.UnpackIntentSolution(msg, b.Protocol())
		if err != nil {
			b.log.Warnf("failed to unpack intent solution message: %v", err)
			return
		}

		_ = b.Node.HandleMsg(solution, b, connections.RunBackground) //nolint:errcheck
	case bxmessage.IntentSolutionsType:
		solutions, err := bxmessage.UnpackIntentSolutions(msg, b.Protocol())
		if err != nil {
			b.Log().Warnf("failed to unpack intent solutions message: %v", err)
			return
		}

		_ = b.Node.HandleMsg(solutions, b, connections.RunForeground) //nolint:errcheck
	case bxmessage.BeaconMessageType:
		beaconMessage := &bxmessage.BeaconMessage{}
		if err := beaconMessage.Unpack(msg, b.Protocol()); err != nil {
			b.log.Warnf("Failed to unpack beacon message: %v", err)
			return
		}
		_ = b.Node.HandleMsg(beaconMessage, b, connections.RunForeground)
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

func (b *BxConn) processMessage(msgBytes bxmessage.MessageBytes) {
	msgBytes.SetNetworkChannelPositionAndInsertTime(len(b.receiveChan), b.clock.Now())
	select {
	case b.receiveChan <- msgBytes:
	default:
		b.log.Errorf("receiveChan is full")
		return
	}
}

// readLoop is the main loop of the connection. It reads messages from the socket and processes them.
func (b *BxConn) readLoop(ctx context.Context) {
	isInitiator := b.IsInitiator()
	defer close(b.receiveChan)

	for {
		select {
		case <-ctx.Done():
			return
		default:
			proceed := b.read(ctx, isInitiator)
			if !proceed {
				return
			}
			// sleep before next connection attempt
			// note - in docker environment the docker-proxy may keep the port open after the docker was stopped. we
			// need this sleep to avoid fast connect/disconnect loop
			b.clock.Sleep(connTimeout)
		}
	}
}

// read connects and reads messages from the socket.
// If we are the initiator of the connection we auto-recover on disconnect.
func (b *BxConn) read(ctx context.Context, isInitiator bool) bool {
	err := b.Connect()
	if err != nil {
		b.Log().Errorf("encountered connection error while connecting: %v", err)

		reason := "could not connect to remote"
		if !isInitiator {
			_ = b.Close(reason)
			return false
		}

		_ = b.closeWithRetry(reason)

		return true
	}

	if isInitiator {
		hello := bxmessage.Hello{NodeID: b.nodeID, Protocol: bxmessage.CurrentProtocol}
		hello.SetNetworkNum(b.networkNum)
		nodeStatus := b.Node.NodeStatus()
		hello.ClientVersion = nodeStatus.Version
		hello.Capabilities = nodeStatus.Capabilities
		_ = b.Send(&hello)
	}

	closeReason := "read loop closed"
	for b.Conn.IsOpen() {
		select {
		case <-ctx.Done():
			return false
		default:
			_, err := b.ReadMessages(b.processMessage, 30*time.Second, bxmessage.HeaderLen,
				func(b []byte) int {
					return int(binary.LittleEndian.Uint32(b[bxmessage.PayloadSizeOffset:]))
				})
			if err != nil {
				closeReason = err.Error()
				b.Log().Tracef("connection closed: %v", err)
				break
			}
		}
	}
	if !isInitiator {
		_ = b.Close(closeReason)
		return false
	}

	_ = b.closeWithRetry(closeReason)
	if b.closed {
		return false
	}

	return true
}

// IsBloxroute detect if the peer belongs to bloxroute
func (b *BxConn) IsBloxroute() bool {
	return b.accountID == types.BloxrouteAccountID
}

// String represents a string conversion of this connection
func (b *BxConn) String() string {
	return b.stringRepresentation
}

// closeWithRetry does not shutdown the main go routines present in BxConn, only the ones in the ssl connection, which can be restarted on the next Connect
func (b *BxConn) closeWithRetry(reason string) error {
	b.connectionEstablished = false
	_ = b.Node.OnConnClosed(b)
	return b.Conn.Close(reason)
}

// GetMinLatencies exposes the best latencies in ms form and to peer
func (b *BxConn) GetMinLatencies() (int64, int64, int64, int64) {
	return b.minFromRelay, b.minToRelay, b.slowCount, b.minRoundTrip
}
