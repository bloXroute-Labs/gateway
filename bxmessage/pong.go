package bxmessage

import (
	"encoding/binary"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// Pong is a response to Ping messages
type Pong struct {
	Header
	Nonce     uint64
	TimeStamp uint64
}

func (pm *Pong) size() uint32 {
	return pm.Header.Size() + (2 * types.UInt64Len)
}

// Pack serializes a Pong into a buffer for sending
func (pm Pong) Pack(protocol Protocol) ([]byte, error) {
	bufLen := pm.size()
	buf := make([]byte, bufLen)
	offset := HeaderLen
	binary.LittleEndian.PutUint64(buf[offset:], pm.Nonce)
	offset += types.UInt64Len
	pm.TimeStamp = uint64(time.Now().UnixNano() / 1000)
	binary.LittleEndian.PutUint64(buf[offset:], pm.TimeStamp)
	offset += types.UInt64Len

	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	pm.Header.Pack(&buf, "pong")
	return buf, nil
}

// Unpack deserializes a Pong from a buffer
func (pm *Pong) Unpack(buf []byte, protocol Protocol) error {
	offset := HeaderLen
	if err := checkBufSize(&buf, offset, HeaderLen); err != nil {
		return err
	}
	pm.Nonce = binary.LittleEndian.Uint64(buf[offset:])

	offset += types.UInt64Len
	if err := checkBufSize(&buf, offset, types.UInt64Len); err != nil {
		return err
	}
	pm.TimeStamp = binary.LittleEndian.Uint64(buf[offset:])

	return pm.Header.Unpack(buf, protocol)
}
