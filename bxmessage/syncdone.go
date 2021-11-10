package bxmessage

import (
	"encoding/binary"
	"github.com/bloXroute-Labs/gateway/types"
)

// SyncDone indicates that transaction service sync is completed
type SyncDone struct {
	Header
}

func (m *SyncDone) size() uint32 {
	return m.Header.Size() + types.NetworkNumLen + 1
}

// Pack serializes a SyncDone into a buffer for sending
func (m *SyncDone) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	binary.LittleEndian.PutUint32(buf[20:], uint32(m.networkNumber))
	buf[bufLen-1] = 0x01
	m.Header.Pack(&buf, SyncDoneType)
	return buf, nil
}

// Unpack deserializes a SyncDone from a buffer
func (m *SyncDone) Unpack(buf []byte, protocol Protocol) error {
	m.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[20:]))
	return nil
}
