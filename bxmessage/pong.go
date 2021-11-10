package bxmessage

import (
	"encoding/binary"
	"time"
)

// Pong is a response to Ping messages
type Pong struct {
	Header
	Nonce     uint64
	TimeStamp uint64
}

func (pm *Pong) size() uint32 {
	return pm.Header.Size() + 8 + 8 + 1
}

// Pack serializes a Pong into a buffer for sending
func (pm Pong) Pack(protocol Protocol) ([]byte, error) {
	bufLen := pm.size()
	buf := make([]byte, bufLen)
	binary.LittleEndian.PutUint64(buf[20:], pm.Nonce)
	pm.TimeStamp = uint64(time.Now().UnixNano() / 1000)
	binary.LittleEndian.PutUint64(buf[28:], pm.TimeStamp)
	buf[bufLen-1] = 0x01
	pm.Header.Pack(&buf, "pong")
	return buf, nil
}

// Unpack deserializes a Pong from a buffer
func (pm *Pong) Unpack(buf []byte, protocol Protocol) error {
	pm.Nonce = binary.LittleEndian.Uint64(buf[20:])
	pm.TimeStamp = binary.LittleEndian.Uint64(buf[28:])
	return nil
}
