package bxmessage

import (
	"encoding/binary"
	"fmt"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	uuid "github.com/satori/go.uuid"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// BroadcastHeader represents the shared header of a bloxroute broadcast message
type BroadcastHeader struct {
	Header
	hash          types.SHA256Hash
	networkNumber bxtypes.NetworkNum
	sourceID      [SourceIDLen]byte
}

// GetNetworkNum gets the message network number
func (b *BroadcastHeader) GetNetworkNum() bxtypes.NetworkNum {
	return b.networkNumber
}

// SetNetworkNum sets the message network number
func (b *BroadcastHeader) SetNetworkNum(networkNum bxtypes.NetworkNum) {
	b.networkNumber = networkNum
}

// Hash returns the message identifier
func (b *BroadcastHeader) Hash() (hash types.SHA256Hash) {
	return b.hash
}

// SetHash sets the block hash
func (b *BroadcastHeader) SetHash(hash types.SHA256Hash) {
	b.hash = hash
}

// SourceID returns the source ID of the broadcast in string format
func (b *BroadcastHeader) SourceID() (sourceID bxtypes.NodeID) {
	u, err := uuid.FromBytes(b.sourceID[:])
	if err != nil {
		log.Errorf("Failed to parse source id from broadcast message, raw bytes: %v", b.sourceID)
		return
	}
	return bxtypes.NodeID(u.String())
}

// SetSourceID sets the source id of the tx
func (b *BroadcastHeader) SetSourceID(sourceID bxtypes.NodeID) error {
	sourceIDBytes, err := uuid.FromString(string(sourceID))
	if err != nil {
		return fmt.Errorf("failed to set source id, source id: %v", sourceIDBytes)
	}

	copy(b.sourceID[:], sourceIDBytes[:])
	return nil
}

// Pack serializes a BroadcastHeader into a buffer for sending on the wire
func (b *BroadcastHeader) Pack(buf *[]byte, msgType string, _ Protocol) {
	offset := HeaderLen

	copy((*buf)[offset:], b.hash[:])
	offset += types.SHA256HashLen // 32
	binary.LittleEndian.PutUint32((*buf)[offset:], uint32(b.networkNumber))
	offset += types.NetworkNumLen // 4
	copy((*buf)[offset:], b.sourceID[:])
	offset += SourceIDLen //nolint:ineffassign
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
	b.networkNumber = bxtypes.NetworkNum(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.NetworkNumLen
	copy(b.sourceID[:], buf[offset:])
	return nil
}

// Size returns the byte length of BroadcastHeader
func (b *BroadcastHeader) Size() uint32 {
	return BroadcastHeaderLen
}
