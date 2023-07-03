package bxmessage

import (
	"bytes"
	"fmt"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Message is the base interface of all connection message sent on the wire
type Message interface {
	Pack(protocol Protocol) ([]byte, error)
	Unpack(buf []byte, protocol Protocol) error
	GetPriority() SendPriority
	SetPriority(priority SendPriority)
	String() string
}

// BroadcastMessage is the base interface of all broadcast message sent on the wire
type BroadcastMessage interface {
	Message
	GetNetworkNum() types.NetworkNum
}

// MessageBytes struct for msg with data
type MessageBytes struct {
	raw                 []byte
	receiveTime         time.Time
	processingStartTime time.Time
	channelPosition     int
}

// NewMessageBytes create new MessageBytes object
func NewMessageBytes(raw []byte, receiveTime time.Time) MessageBytes {
	return MessageBytes{
		raw:         raw,
		receiveTime: receiveTime,
	}
}

// SetProcessingTime set processingTime
func (mb *MessageBytes) SetProcessingTime(processingTime time.Time) {
	mb.processingStartTime = processingTime
}

// SetChannelPosition set channelPosition
func (mb *MessageBytes) SetChannelPosition(txsInChannel int) {
	mb.channelPosition = txsInChannel
}

// BxType parses the message type from a bx message
func (mb MessageBytes) BxType() string {
	return string(bytes.Trim(mb.raw[TypeOffset:TypeOffset+TypeLength], NullByte))
}

// String formats the message bytes for hex output
func (mb MessageBytes) String() string {
	return fmt.Sprintf("%v[%v]", mb.BxType(), hexutil.Encode(mb.raw))
}

// Raw return raw msg
func (mb MessageBytes) Raw() []byte {
	return mb.raw
}

// ReceiveTime return receiveTime
func (mb MessageBytes) ReceiveTime() time.Time {
	return mb.receiveTime
}

// ProcessingStartTime return processingStartTime
func (mb MessageBytes) ProcessingStartTime() time.Time {
	return mb.processingStartTime
}

// ChannelPosition return channelPosition
func (mb MessageBytes) ChannelPosition() int {
	return mb.channelPosition
}
