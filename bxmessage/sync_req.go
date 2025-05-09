package bxmessage

import (
	"encoding/binary"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// SyncReq requests to start transaction state sync on a network
type SyncReq struct {
	Header
	networkNumber bxtypes.NetworkNum
}

// GetNetworkNum gets the message network number
func (m *SyncReq) GetNetworkNum() bxtypes.NetworkNum {
	return m.networkNumber
}

// SetNetworkNum sets the message network number
func (m *SyncReq) SetNetworkNum(networkNum bxtypes.NetworkNum) {
	m.networkNumber = networkNum
}

func (m *SyncReq) size() uint32 {
	return m.Header.Size() + types.NetworkNumLen
}

// Pack serializes a SyncReq into a buffer for sending
func (m SyncReq) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	binary.LittleEndian.PutUint32(buf[HeaderLen:], uint32(m.networkNumber))
	m.Header.Pack(&buf, SyncReqType)
	return buf, nil
}

// Unpack deserializes a SyncReq from a buffer
func (m *SyncReq) Unpack(buf []byte, protocol Protocol) error {
	m.networkNumber = bxtypes.NetworkNum(binary.LittleEndian.Uint32(buf[HeaderLen:]))
	return m.Header.Unpack(buf, protocol)
}
