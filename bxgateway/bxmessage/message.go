package bxmessage

import (
	"bytes"
	"fmt"
	"github.com/bloXroute-Labs/gateway/bxgateway/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
)

// Message is the base interface of all connection message sent on the wire
type Message interface {
	Pack(protocol Protocol) ([]byte, error)
	Unpack(buf []byte, protocol Protocol) error
	GetPriority() SendPriority
	SetPriority(priority SendPriority)
	GetNetworkNum() types.NetworkNum
	SetNetworkNum(types.NetworkNum)
	String() string
}

// MessageBytes type def for message byte sets
type MessageBytes []byte

// BxType parses the message type from a bx message
func (mb MessageBytes) BxType() string {
	return string(bytes.Trim(mb[TypeOffset:TypeOffset+TypeLength], NullByte))
}

// String formats the message bytes for hex output
func (mb MessageBytes) String() string {
	return fmt.Sprintf("%v[%v]", mb.BxType(), hexutil.Encode(mb))
}
