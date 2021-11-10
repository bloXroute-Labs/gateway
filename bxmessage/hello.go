package bxmessage

import (
	"encoding/binary"
	"encoding/hex"
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"strings"
)

// Hello exchanges node and protocol info when two bloxroute nodes initially connect
type Hello struct {
	Header
	NodeID   types.NodeID
	Protocol Protocol
}

func (m *Hello) size() uint32 {
	return m.Header.Size() + 24 + 1
}

// Pack serializes a Hello into the buffer for sending
func (m *Hello) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	binary.LittleEndian.PutUint32(buf[20:], uint32(m.Protocol))
	binary.LittleEndian.PutUint32(buf[24:], uint32(m.networkNumber))
	nodeID, err := hex.DecodeString(strings.ReplaceAll(string(m.NodeID), "-", ""))
	if err != nil {
		return nil, err
	}
	copy(buf[28:], nodeID)
	buf[bufLen-1] = 0x01
	m.Header.Pack(&buf, "hello")
	return buf, nil

}

// Unpack deserializes a Hello from a buffer
func (m *Hello) Unpack(buf []byte, protocol Protocol) error {
	m.Protocol = Protocol(binary.LittleEndian.Uint32(buf[HeaderLen:]))
	m.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[HeaderLen+ProtocolLen:]))
	return nil
}
