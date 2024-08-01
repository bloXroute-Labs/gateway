package bxmessage

import (
	"encoding/binary"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// Ping initiates a ping for connection liveliness
type Ping struct {
	Header
	Nonce uint64
}

func (pm *Ping) size() uint32 {
	return pm.Header.Size() + types.UInt64Len
}

// Pack serializes a Ping into a buffer for sending
func (pm Ping) Pack(protocol Protocol) ([]byte, error) {
	if pm.Nonce == 0 {
		pm.Nonce = uint64(time.Now().UnixNano() / 1000)
	}
	bufLen := pm.size()
	buf := make([]byte, bufLen)
	binary.LittleEndian.PutUint64(buf[HeaderLen:], pm.Nonce)
	pm.Header.Pack(&buf, "ping")
	return buf, nil
}

// Unpack deserializes a Ping from a buffer
func (pm *Ping) Unpack(buf []byte, protocol Protocol) error {
	offset := HeaderLen
	if err := checkBufSize(&buf, offset, types.UInt64Len); err != nil {
		return err
	}
	pm.Nonce = binary.LittleEndian.Uint64(buf[offset:])
	return pm.Header.Unpack(buf, protocol)
}
