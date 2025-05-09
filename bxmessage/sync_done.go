package bxmessage

import (
	"encoding/binary"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// SyncDone indicates that transaction service sync is completed
type SyncDone struct {
	Header
	networkNumber bxtypes.NetworkNum
}

// GetNetworkNum gets the message network number
func (m *SyncDone) GetNetworkNum() bxtypes.NetworkNum {
	return m.networkNumber
}

// SetNetworkNum sets the message network number
func (m *SyncDone) SetNetworkNum(networkNum bxtypes.NetworkNum) {
	m.networkNumber = networkNum
}

func (m *SyncDone) size() uint32 {
	return m.Header.Size() + types.NetworkNumLen
}

// Pack serializes a SyncDone into a buffer for sending
func (m *SyncDone) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	binary.LittleEndian.PutUint32(buf[HeaderLen:], uint32(m.networkNumber))
	m.Header.Pack(&buf, SyncDoneType)
	return buf, nil
}

// Unpack deserializes a SyncDone from a buffer
func (m *SyncDone) Unpack(buf []byte, protocol Protocol) error {
	m.networkNumber = bxtypes.NetworkNum(binary.LittleEndian.Uint32(buf[HeaderLen:]))
	return m.Header.Unpack(buf, protocol)
}
