package bxmessage

import (
	"encoding/binary"
	"github.com/bloXroute-Labs/bloxroute-gateway-go/bxgateway/types"
)

// SyncReq requests to start transaction state sync on a network
type SyncReq struct {
	Header
}

func (m *SyncReq) size() uint32 {
	return m.Header.Size() + 4 + 1
}

// Pack serializes a SyncReq into a buffer for sending
func (m SyncReq) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	binary.LittleEndian.PutUint32(buf[20:], uint32(m.networkNumber))
	buf[bufLen-1] = 0x01
	m.Header.Pack(&buf, SyncReqType)
	return buf, nil
}

// Unpack deserializes a SyncReq from a buffer
func (m *SyncReq) Unpack(buf []byte, protocol Protocol) error {
	m.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[20:]))
	return nil
}
