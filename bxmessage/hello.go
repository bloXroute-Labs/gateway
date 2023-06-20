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
	size := m.Header.Size() + ProtocolLen + types.NetworkNumLen + types.NodeIDLen
	if m.Protocol >= MEVProtocol {
		size += ClientVersionLen + CapabilitiesLen
	}

	return size
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
	if m.Protocol >= MEVProtocol {
		binary.LittleEndian.PutUint16(buf[offset:], uint16(m.Capabilities))
		offset += CapabilitiesLen
		copy(buf[offset:], m.ClientVersion)
		offset += ClientVersionLen
	}
	m.Header.Pack(&buf, "hello")
	return buf, nil

}

// Unpack deserializes a Hello from a buffer
func (m *Hello) Unpack(buf []byte, protocol Protocol) error {
	offset := HeaderLen
	m.Protocol = Protocol(binary.LittleEndian.Uint32(buf[offset:]))
	offset += ProtocolLen
	m.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.NetworkNumLen
	m.NodeID = types.NodeID(buf[offset:])
	offset += types.NodeIDLen
	if m.Protocol >= MEVProtocol {
		m.Capabilities = types.CapabilityFlags(binary.LittleEndian.Uint16(buf[offset:]))
		offset += CapabilitiesLen
		m.ClientVersion = string(buf[offset:])
		offset += ClientVersionLen
	}

	if m.Protocol < FlashbotsGatewayProtocol {
		m.Capabilities |= types.CapabilityBDN
	}

	return m.Header.Unpack(buf, protocol)
}
