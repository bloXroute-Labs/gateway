package connections

import (
	"bufio"
	"bytes"
	"fmt"
	"github.com/bloXroute-Labs/gateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/utils"
	log "github.com/sirupsen/logrus"
	"time"
)

const (
	// RemoteInitiatedPort is a special constant used to indicate connections initiated from the remote
	RemoteInitiatedPort = 0

	// LocalInitiatedPort is a special constant used to indicate connections initiated locally
	LocalInitiatedPort = 0

	// PriorityQueueInterval represents the minimum amount of time that must be elapsed between non highest priority messages intended to sent along the connection
	PriorityQueueInterval = 500 * time.Microsecond

	readPacketSize = 4096
)

// SSLConn represents the basic connection properties for connections opened between bloxroute nodes. SSLConn does not define any message handlers, and only implements the Conn interface.
type SSLConn struct {
	Socket
	writer *bufio.Writer

	connect        func() (Socket, error)
	ip             string
	port           int64
	protocol       bxmessage.Protocol
	sslCerts       *utils.SSLCerts
	connectionOpen bool
	disabled       bool
	// done is used to stop the sendLoop routine
	done            chan bool
	sendMessages    chan bxmessage.Message
	sendChannelSize int
	buf             bytes.Buffer
	usePQ           bool
	pq              *utils.MsgPriorityQueue
	logMessages     bool
	extensions      utils.BxSSLProperties
	packet          []byte
	log             *log.Entry
	clock           utils.Clock
}

// NewSSLConnection constructs a new SSL connection. If socket is not nil, then the connection was initiated by the remote.
func NewSSLConnection(connect func() (Socket, error), sslCerts *utils.SSLCerts, ip string, port int64, protocol bxmessage.Protocol, usePQ bool, logMessages bool, sendChannelSize int, clock utils.Clock) *SSLConn {
	conn := &SSLConn{
		connect:         connect,
		sslCerts:        sslCerts,
		ip:              ip,
		port:            port,
		protocol:        protocol,
		buf:             bytes.Buffer{},
		usePQ:           usePQ,
		logMessages:     logMessages,
		sendChannelSize: sendChannelSize,
		packet:          make([]byte, readPacketSize),
		log:             log.WithField("remoteAddr", fmt.Sprintf("%v:%v", ip, port)),
		clock:           clock,
	}
	return conn
}

// sendLoop waits for messages on channel and send them to the socket
// terminates when the channel is closed
func (s *SSLConn) sendLoop() {
	s.Log().Trace("starting send loop")
	for {
		select {
		case <-s.done:
			s.Log().Trace("stopping send loop (done)")
			return
		case msg := <-s.sendMessages:
			s.packAndWrite(msg)
			continueReading := true
			for continueReading {
				select {
				case msg = <-s.sendMessages:
					s.packAndWrite(msg)
				default:
					continueReading = false
				}
			}
			err := s.writer.Flush()
			if err != nil {
				s.Log().Trace("stopping send loop (failed to Flush output buffer)")
				return
			}
		}
	}
}

// packAndWrite is called by the sendLoop go routine
func (s *SSLConn) packAndWrite(msg bxmessage.Message) {
	if !s.connectionOpen {
		return
	}

	buf, err := msg.Pack(s.protocol)
	if err != nil {
		s.Log().Warnf("can't pack message %v: %v. skipping", msg, err)
		return
	}

	// these lines can be enabled for logging exactly when we send transactions to the relays
	//if s.logMessages {
	//	log.Tracef("sending %v to %v", msg, s)
	//}

	_, err = s.writer.Write(buf)
	if err != nil {
		s.Log().Warnf("can't write message: %v. marking connection as closed", err)
		_ = s.Close("could not write message to socket")
	}
}

// ID returns the underlying connection for checking identity
func (s *SSLConn) ID() Socket {
	return s.Socket
}

// Info returns connection details, include details parsed from certificates
func (s *SSLConn) Info() Info {
	return Info{
		NodeID:          s.extensions.NodeID,
		AccountID:       s.extensions.AccountID,
		PeerIP:          s.ip,
		PeerPort:        s.port,
		LocalPort:       LocalInitiatedPort,
		ConnectionType:  s.extensions.NodeType,
		ConnectionState: "",
		NetworkNum:      0,
		FromMe:          s.isInitiator(),
	}
}

// IsOpen indicates whether the socket connection is open
func (s SSLConn) IsOpen() bool {
	return s.connectionOpen
}

// IsDisabled indicates whether the socket connection is disabled (ping and pong only)
func (s SSLConn) IsDisabled() bool {
	return s.disabled
}

// Protocol provides the protocol version of the connection
func (s *SSLConn) Protocol() bxmessage.Protocol {
	return s.protocol
}

// SetProtocol sets the protocol version the connection is using
func (s *SSLConn) SetProtocol(p bxmessage.Protocol) {
	s.protocol = p
}

// Log returns the context logger for the SSL connection
func (s *SSLConn) Log() *log.Entry {
	return s.log
}

// Connect initializes a connection to a bloxroute node. If this is called when the remote addr is the one initiating the connection, then this function is does little besides mark some connection states as ready. Connect is also responsible for starting any goroutines relevant to the connection.
func (s *SSLConn) Connect() error {
	s.pq = utils.NewMsgPriorityQueue(s.queueToMessageChan, PriorityQueueInterval)
	s.buf.Reset()

	var err error
	s.Socket, err = s.connect()
	if err != nil {
		s.connectionOpen = false
		return err
	}
	// allocate a buffered writer to combine outgoing messages
	s.writer = bufio.NewWriter(s.Socket)

	extensions, err := s.Properties()
	if err != nil {
		return err
	}
	s.extensions = extensions
	s.connectionOpen = true
	// start send loop now that connection is connected
	s.sendMessages = make(chan bxmessage.Message, s.sendChannelSize)
	s.done = make(chan bool)
	go s.sendLoop()

	return nil
}

// ReadMessages reads series of messages from the socket, placing each distinct message on the channel
func (s *SSLConn) ReadMessages(callBack func(msg bxmessage.MessageBytes), readDeadline time.Duration, headerLen int, readPayloadLen func([]byte) int) (int, error) {

	n, err := s.readWithDeadline(s.packet, readDeadline)
	if err != nil {
		s.Log().Debugf("connection closed while reading: %v", err)
		_ = s.Close("connect closed by remote while reading")
		return n, err
	}
	// TODO: why ReadMessages has to return every socket read?
	s.buf.Write(s.packet[:n])
	for {
		bufLen := s.buf.Len()
		if bufLen < headerLen {
			break
		}

		payloadLen := readPayloadLen(s.buf.Bytes())
		if bufLen < headerLen+payloadLen {
			break
		}
		// allocate an array for the message to protect from overrun by multiple go routines
		msg := make([]byte, headerLen+payloadLen)
		_, err = s.buf.Read(msg)
		if err != nil {
			s.Log().Warnf("encountered error while reading message: %v, skipping", err)
			continue
		}
		msgType := bxmessage.MessageBytes(msg).BxType()
		if s.IsDisabled() && msgType != bxmessage.PingType && msgType != bxmessage.PongType {
			continue
		}
		callBack(msg)
	}
	return n, nil
}

// readWithDeadline reads bytes from the connection onto a buffer
func (s *SSLConn) readWithDeadline(buf []byte, deadline time.Duration) (int, error) {
	if !s.connectionOpen {
		return 0, fmt.Errorf("connection is closing. Read from socket disabled")
	}
	_ = s.Socket.SetReadDeadline(s.clock.Now().Add(deadline))
	return s.Socket.Read(buf)
}

// Send sends messages over the wire to the peer node
func (s *SSLConn) Send(msg bxmessage.Message) error {
	var err error
	if s.Socket == nil {
		err = fmt.Errorf("trying to send a message to connection before it's connected")
		s.Log().Debug(err)
		return err
	}
	if !s.connectionOpen {
		// note - can't use s.String() or s.s.RemoteAddr()  here since RemoteAddr() may produce nil
		err = fmt.Errorf("trying to send a message to %v:%v while it is closed", s.ip, s.port)
		s.Log().Debug(err)
		return err
	}
	// in order not to overload the python code - use priority queue and a gap of 0.5ms between sends
	// this should be removed once relay/gw are in GOLANG
	// Note: as of BX-2912 the priority queue mechanism is disabled without an option to activate it
	// TODO: remove PQ from code base
	if true || !s.usePQ || msg.GetPriority() == bxmessage.HighestPriority {
		s.queueToMessageChan(msg)
	} else {
		s.pq.Push(msg)
	}
	return nil
}

// SendWithDelay sends messages over the wire to the peer node after waiting the requests delay
func (s *SSLConn) SendWithDelay(msg bxmessage.Message, delay time.Duration) error {
	// avoid goroutine creation for no delay
	if delay == 0 {
		return s.Send(msg)
	}
	go func() {
		s.clock.Sleep(delay)
		err := s.Send(msg)
		if err != nil {
			s.Log().Errorf("could not send on conn %v", err)
		}
	}()
	return nil
}

func (s *SSLConn) queueToMessageChan(msg bxmessage.Message) {
	select {
	case s.sendMessages <- msg:
		// all good if we are here
	default:
		_ = s.Close("cannot place message on channel without blocking")
	}
}

// Disable marks the connection as disabled, meaning it sends/processes only ping/pong messages. Must be called in a go routine.
func (s *SSLConn) Disable(reason string) {
	s.Log().Errorf("disabling connection: %v", reason)
	s.disabled = true
}

// Close shuts down a connection. If the connection was initiated by this node, it can be reopened with another Connect call. If the connection was initiated by the remote, it cannot be reopened.
func (s *SSLConn) Close(reason string) error {
	return s.close(reason)
}

// String represents a printable/readable identifier for the connection
func (s SSLConn) String() string {
	if s.Socket == nil {
		return fmt.Sprintf("%v:%v", s.ip, s.port)
	}
	return s.Socket.RemoteAddr().String()
}

// isInitiator returns whether this node initiated the connection
func (s *SSLConn) isInitiator() bool {
	return s.port != RemoteInitiatedPort
}

// close should only be called when s.lock is already held
func (s *SSLConn) close(reason string) error {
	// connection already closed
	if !s.connectionOpen {
		return nil
	}

	s.connectionOpen = false
	s.disabled = false
	// don't close s.sendMessages - not needed and can create race with sendLoop

	// stop sendLoop
	s.done <- true

	if s.Socket != nil {
		err := s.Socket.Close(reason)
		if err != nil {
			s.Log().Debugf("unable to close connection: %v", err)
			return err
		}
		s.Log().Infof("TLS is now closed: %v", reason)
	}
	return nil
}
