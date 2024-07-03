package bxmessage

import (
	"encoding/binary"
	"encoding/hex"
	"strings"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// Hello exchanges node and protocol info when two bloxroute nodes initially connect
type Hello struct {
	Header
	Protocol      Protocol
	networkNumber types.NetworkNum
	NodeID        types.NodeID
	Capabilities  types.CapabilityFlags
	ClientVersion string
}

// GetNetworkNum gets the message network number
func (m *Hello) GetNetworkNum() types.NetworkNum {
	return m.networkNumber
}

// SetNetworkNum sets the message network number
func (m *Hello) SetNetworkNum(networkNum types.NetworkNum) {
	m.networkNumber = networkNum
}

func (m *Hello) size() uint32 {
	return m.Header.Size() + ProtocolLen + types.NetworkNumLen + types.NodeIDLen + ClientVersionLen + CapabilitiesLen
}

// Pack serializes a Hello into the buffer for sending
func (m *Hello) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	offset := HeaderLen
	binary.LittleEndian.PutUint32(buf[offset:], uint32(m.Protocol))
	offset += ProtocolLen
	binary.LittleEndian.PutUint32(buf[offset:], uint32(m.networkNumber))
	offset += types.NetworkNumLen
	nodeID, err := hex.DecodeString(strings.ReplaceAll(string(m.NodeID), "-", ""))
	if err != nil {
		return nil, err
	}
	copy(buf[offset:], nodeID)
	offset += types.NodeIDLen
	binary.LittleEndian.PutUint16(buf[offset:], uint16(m.Capabilities))
	offset += CapabilitiesLen
	copy(buf[offset:], m.ClientVersion)
	offset += ClientVersionLen
	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}
	m.Header.Pack(&buf, "hello")
	return buf, nil
}

// Unpack deserializes a Hello from a buffer
func (m *Hello) Unpack(buf []byte, protocol Protocol) error {
	offset := HeaderLen

	if err := checkBufSize(&buf, offset, ProtocolLen); err != nil {
		return err
	}
	m.Protocol = Protocol(binary.LittleEndian.Uint32(buf[offset:]))
	offset += ProtocolLen

	if err := checkBufSize(&buf, offset, types.NetworkNumLen); err != nil {
		return err
	}
	m.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.NetworkNumLen

	if err := checkBufSize(&buf, offset, types.NodeIDLen); err != nil {
		return err
	}
	m.NodeID = types.NodeID(buf[offset:])
	offset += types.NodeIDLen
	m.Capabilities = types.CapabilityFlags(binary.LittleEndian.Uint16(buf[offset:]))
	offset += CapabilitiesLen
	if err := checkBufSize(&buf, offset, CapabilitiesLen); err != nil {
		return err
	}
	m.ClientVersion = string(buf[offset:])
	offset += ClientVersionLen
	if err := checkBufSize(&buf, offset, ClientVersionLen); err != nil {
		return err
	}

	return m.Header.Unpack(buf, protocol)
}
