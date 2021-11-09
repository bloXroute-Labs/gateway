package bxmessage

import (
	"encoding/binary"
	"time"
)

// Ping initiates a ping for connection liveliness
type Ping struct {
	Header
	Nonce uint64
}

func (pm *Ping) size() uint32 {
	return pm.Header.Size() + 8 + 1
}

// Pack serializes a Ping into a buffer for sending
func (pm Ping) Pack(protocol Protocol) ([]byte, error) {
	if pm.Nonce == 0 {
		pm.Nonce = uint64(time.Now().UnixNano() / 1000)
	}
	bufLen := pm.size()
	buf := make([]byte, bufLen)
	binary.LittleEndian.PutUint64(buf[20:], pm.Nonce)
	buf[bufLen-1] = 0x01
	pm.Header.Pack(&buf, "ping")
	return buf, nil
}

// Unpack deserializes a Ping from a buffer
func (pm *Ping) Unpack(buf []byte, protocol Protocol) error {
	pm.Nonce = binary.LittleEndian.Uint64(buf[20:])
	return nil
}
