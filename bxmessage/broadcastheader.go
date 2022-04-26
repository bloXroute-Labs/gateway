package bxmessage

import (
	"encoding/binary"
	"fmt"
	log "github.com/bloXroute-Labs/gateway/logger"
	"github.com/bloXroute-Labs/gateway/types"
	uuid "github.com/satori/go.uuid"
)

// BroadcastHeader represents the shared header of a bloxroute broadcast message
type BroadcastHeader struct {
	Header
	hash          types.SHA256Hash
	networkNumber types.NetworkNum
	sourceID      [SourceIDLen]byte
}

// GetNetworkNum gets the message network number
func (b *BroadcastHeader) GetNetworkNum() types.NetworkNum {
	return b.networkNumber
}

// SetNetworkNum sets the message network number
func (b *BroadcastHeader) SetNetworkNum(networkNum types.NetworkNum) {
	b.networkNumber = networkNum
}

// Hash returns the message hash
func (b *BroadcastHeader) Hash() (hash types.SHA256Hash) {
	return b.hash
}

// SetHash sets the block hash
func (b *BroadcastHeader) SetHash(hash types.SHA256Hash) {
	b.hash = hash
}

// SourceID returns the source ID of the broadcast in string format
func (b *BroadcastHeader) SourceID() (sourceID types.NodeID) {
	u, err := uuid.FromBytes(b.sourceID[:])
	if err != nil {
		log.Errorf("Failed to parse source id from broadcast message, raw bytes: %v", b.sourceID)
		return
	}
	return types.NodeID(u.String())
}

// SetSourceID sets the source id of the tx
func (b *BroadcastHeader) SetSourceID(sourceID types.NodeID) error {
	sourceIDBytes, err := uuid.FromString(string(sourceID))
	if err != nil {
		return fmt.Errorf("Failed to set source id, source id: %v", sourceIDBytes)
	}

	copy(b.sourceID[:], sourceIDBytes[:])
	return nil
}

// Pack serializes a BroadcastHeader into a buffer for sending on the wire
func (b *BroadcastHeader) Pack(buf *[]byte, msgType string) {
	offset := HeaderLen

	copy((*buf)[offset:], b.hash[:])
	offset += types.SHA256HashLen
	binary.LittleEndian.PutUint32((*buf)[offset:], uint32(b.networkNumber))
	offset += types.NetworkNumLen
	copy((*buf)[offset:], b.sourceID[:])
	offset += SourceIDLen
	b.Header.Pack(buf, msgType)
}

// Unpack deserializes a BroadcastHeader from a buffer
func (b *BroadcastHeader) Unpack(buf []byte, protocol Protocol) error {
	if err := b.Header.Unpack(buf, protocol); err != nil {
		return err
	}
	if err := checkBufSize(&buf, 0, int(b.Size())); err != nil {
		return err
	}

	offset := HeaderLen
	copy(b.hash[:], buf[HeaderLen:])
	offset += types.SHA256HashLen
	b.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.NetworkNumLen
	copy(b.sourceID[:], buf[offset:])
	return nil
}

// Size returns the byte length of BroadcastHeader
func (b *BroadcastHeader) Size() uint32 {
	return b.Header.Size() + uint32(types.SHA256HashLen+types.NetworkNumLen+SourceIDLen)
}
