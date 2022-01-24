package bxmock

import (
	"bytes"
	"crypto/tls"
	"encoding/binary"
	"errors"
	"github.com/bloXroute-Labs/gateway/bxmessage"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/bloXroute-Labs/gateway/utils"
	"net"
	"strconv"
	"strings"
	"time"
)

const connectionTimeout = 200 * time.Millisecond

// MockBytes represents a struct for passing messages to MockTLS to queue up a message or close the connection
type MockBytes struct {
	b     []byte
	close bool
}

// MockTLS is a tls.Conn that is easily manipulated for test cases
type MockTLS struct {
	*tls.Conn
	ip           string
	netIP        net.IP
	port         int
	nodeID       types.NodeID
	nodeType     utils.NodeType
	accountID    types.AccountID
	queuedBytes  chan MockBytes
	sendingBytes chan []byte
	buf          bytes.Buffer
	Timeout      time.Duration
}

// NewMockTLS constructs a new mock for testing
func NewMockTLS(ip string, port int64, nodeID types.NodeID, nodeType utils.NodeType, accountID types.AccountID) MockTLS {
	split := strings.Split(ip, ".")
	ipBytes := make([]byte, len(split))
	for i, part := range split {
		intRep, _ := strconv.Atoi(part)
		ipBytes[i] = byte(intRep)
	}
	return MockTLS{
		ip:           ip,
		netIP:        ipBytes,
		port:         int(port),
		nodeID:       nodeID,
		nodeType:     nodeType,
		accountID:    accountID,
		queuedBytes:  make(chan MockBytes, 100),
		sendingBytes: make(chan []byte),
		Timeout:      connectionTimeout,
	}
}

// Read pulls messages queued onto the MockTLS connection
func (m MockTLS) Read(b []byte) (int, error) {
	msg := <-m.queuedBytes
	if msg.close {
		return 0, errors.New("closing connection")
	}
	copy(b, msg.b)
	bytesRead := len(msg.b)
	return bytesRead, nil
}

// SetReadDeadline currently does nothing
func (m MockTLS) SetReadDeadline(_ time.Time) error {
	return nil
}

// Write currently does nothing. An expected implementation in the future would track bytes written for comparison for tests.
func (m MockTLS) Write(b []byte) (int, error) {
	var header [bxmessage.HeaderLen]byte
	m.buf.Write(b)
	for {
		if m.buf.Len() < bxmessage.HeaderLen {
			return len(b), nil
		}

		_, _ = m.buf.Read(header[:])
		payloadLen := int(binary.LittleEndian.Uint32(header[bxmessage.PayloadSizeOffset:]))
		if m.buf.Len() < payloadLen {
			panic("unhandled case (need to create a test that Writes a message in chunks")
		}
		payload := make([]byte, payloadLen)
		_, _ = m.buf.Read(payload)
		m.sendingBytes <- append(header[:], payload...)
	}
}

// RemoteAddr is a filler implementation that returns the data this mock was constructed with
func (m MockTLS) RemoteAddr() net.Addr {
	addr := net.TCPAddr{
		IP:   m.netIP,
		Port: m.port,
		Zone: "",
	}
	return &addr
}

// Properties is a filler implementation that returns the data this mock was constructed with
func (m MockTLS) Properties() (utils.BxSSLProperties, error) {
	return utils.BxSSLProperties{
		NodeType:  m.nodeType,
		NodeID:    m.nodeID,
		AccountID: m.accountID,
	}, nil
}

// Close simulates an EOF from the remote connection
func (m MockTLS) Close(string) error {
	m.queuedBytes <- MockBytes{close: true}
	return nil
}

// MockQueue is a mock only method to queue up bytes to be read by the connection
func (m MockTLS) MockQueue(b []byte) {
	m.queuedBytes <- MockBytes{b: b}
}

// MockAdvanceSent is a mock only method that processes the sent bytes so the next message can be sent on the socket. MockTLS only allows a single message to be queued up at a time.
func (m MockTLS) MockAdvanceSent() ([]byte, error) {
	t := time.NewTimer(m.Timeout)
	select {
	case sentBytes := <-m.sendingBytes:
		return sentBytes, nil
	case <-t.C:
		return nil, errors.New("no bytes were sent on the expected connection")
	}
}
