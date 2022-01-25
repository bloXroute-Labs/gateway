package bxmessage

import (
	"bytes"
	"encoding/binary"
	"encoding/hex"
	"fmt"
)

// Protocol represents the Protocol version number
type Protocol uint32

// SendPriority controls the priority send queue
type SendPriority int

// message sending priorities
const (
	HighestPriority SendPriority = iota
	HighPriority
	NormalPriority
	LowPriority
	LowestPriority
	OnPongPriority
)

func (p SendPriority) String() string {
	return [...]string{"HighestPriority", "HighPriority", "NormalPriority", "LowPriority", "LowestPriority"}[p]
}

// Header represents the shared header of a bloxroute message
type Header struct {
	priority *SendPriority
	msgType  string
}

// Pack serializes a Header into a buffer for sending on the wire
func (h *Header) Pack(buf *[]byte, msgType string) {
	h.msgType = msgType

	binary.BigEndian.PutUint32(*buf, 0xfffefdfc)
	copy((*buf)[TypeOffset:], msgType)
	binary.LittleEndian.PutUint32((*buf)[PayloadSizeOffset:], uint32(len(*buf))-HeaderLen)

	(*buf)[len(*buf)-ControlByteLen] = ValidControlByte
}

// Unpack deserializes a Header from a buffer
func (h *Header) Unpack(buf []byte, _ Protocol) error {
	h.msgType = string(bytes.Trim(buf[TypeOffset:TypeOffset+TypeLength], NullByte))
	return nil
}

// Size returns the byte length of header plus ControlByteLen
func (h *Header) Size() uint32 {
	return HeaderLen + ControlByteLen
}

// GetPriority extracts the message send priority
func (h *Header) GetPriority() SendPriority {
	if h.priority == nil {
		return NormalPriority
	}
	return *h.priority
}

// SetPriority sets the message send priority
func (h *Header) SetPriority(priority SendPriority) {
	h.priority = &priority
}

func (h *Header) String() string {
	return fmt.Sprintf("Message<type: %v>", h.msgType)
}

func checkBufSize(buf *[]byte, offset int, size int) error {
	if len(*buf) < offset+size {
		return fmt.Errorf("Invalid message format. %v bytes needed at offset %v but buff size is %v. buffer: %v",
			size, offset, len(*buf), hex.EncodeToString(*buf))
	}
	return nil
}
