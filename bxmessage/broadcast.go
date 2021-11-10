package bxmessage

import (
	"encoding/binary"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/types"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

// Broadcast - represent the "broadcast" message
type Broadcast struct {
	Header
	blockHash     types.SHA256Hash
	sourceID      [SourceIDLen]byte
	broadcastType [BroadcastTypeLen]byte
	encrypted     bool
	block         []byte
	sids          types.ShortIDList
}

// NewBlockBroadcast creates a new broadcast message containing block message bytes
func NewBlockBroadcast(hash types.SHA256Hash, block []byte, shortIDs types.ShortIDList, networkNum types.NetworkNum) *Broadcast {
	var broadcastType [BroadcastTypeLen]byte
	copy(broadcastType[:], "blck")

	b := &Broadcast{
		blockHash:     hash,
		broadcastType: broadcastType,
		encrypted:     false,
		block:         block,
		sids:          shortIDs,
	}
	b.networkNumber = networkNum
	return b
}

// BlockHash returns the block hash
func (broadcast Broadcast) BlockHash() types.SHA256Hash {
	return broadcast.blockHash
}

// SourceID returns the source ID of block in string format
func (broadcast Broadcast) SourceID() (sourceID types.NodeID) {
	u, err := uuid.FromBytes(broadcast.sourceID[:])
	if err != nil {
		log.Errorf("Failed to parse source id from broadcast message, raw bytes: %v", broadcast.block)
		return
	}
	return types.NodeID(u.String())
}

// BroadcastType returns the broadcast type
func (broadcast Broadcast) BroadcastType() [BroadcastTypeLen]byte {
	return broadcast.broadcastType
}

// Encrypted returns the encrypted byte
func (broadcast Broadcast) Encrypted() bool {
	return broadcast.encrypted
}

// Block returns the block
func (broadcast Broadcast) Block() []byte {
	return broadcast.block
}

// ShortIDs return sids
func (broadcast Broadcast) ShortIDs() types.ShortIDList {
	return broadcast.sids
}

// SetBlockHash sets the block hash
func (broadcast *Broadcast) SetBlockHash(hash types.SHA256Hash) {
	broadcast.blockHash = hash
}

// SetSourceID sets the source ID
func (broadcast *Broadcast) SetSourceID(sourceID types.NodeID) {
	sourceIDBytes, err := uuid.FromString(string(sourceID))
	if err != nil {
		log.Errorf("Failed to set source id, source id: %v", sourceIDBytes)
		return
	}

	copy(broadcast.sourceID[:], sourceIDBytes[:])
}

// SetBroadcastType sets the broadcast type
func (broadcast *Broadcast) SetBroadcastType(broadcastType [BroadcastTypeLen]byte) {
	broadcast.broadcastType = broadcastType
}

// SetEncrypted sets the encrypted byte
func (broadcast *Broadcast) SetEncrypted(encrypted bool) {
	broadcast.encrypted = encrypted
}

// SetBlock sets the block
func (broadcast *Broadcast) SetBlock(block []byte) {
	broadcast.block = block
}

// SetSids sets the sids
func (broadcast *Broadcast) SetSids(sids types.ShortIDList) {
	broadcast.sids = sids
}

// Pack serializes a Broadcast into a buffer for sending
func (broadcast *Broadcast) Pack(protocol Protocol) ([]byte, error) {
	bufLen := broadcast.Size()
	buf := make([]byte, bufLen)
	offset := HeaderLen
	copy(buf[offset:], broadcast.blockHash[:])
	offset += types.SHA256HashLen
	binary.LittleEndian.PutUint32(buf[offset:], uint32(broadcast.networkNumber))
	offset += types.NetworkNumLen
	copy(buf[offset:], broadcast.sourceID[:])
	offset += SourceIDLen
	copy(buf[offset:], broadcast.broadcastType[:])
	offset += BroadcastTypeLen
	if broadcast.encrypted {
		copy(buf[offset:], []uint8{1})
	} else {
		copy(buf[offset:], []uint8{0})
	}
	offset += EncryptedTypeLen
	binary.LittleEndian.PutUint64(buf[offset:], uint64(len(broadcast.block)+types.UInt64Len))
	offset += types.UInt64Len
	copy(buf[offset:], broadcast.block)
	offset += len(broadcast.block)
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(broadcast.sids)))
	offset += types.UInt32Len
	for _, sid := range broadcast.sids {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(sid))
		offset += types.UInt32Len
	}
	buf[bufLen-1] = 0x01
	broadcast.Header.Pack(&buf, BroadcastType)
	return buf, nil
}

// Unpack deserializes a Broadcast from a buffer
func (broadcast *Broadcast) Unpack(buf []byte, protocol Protocol) error {
	if err := checkBufSize(&buf, 0, int(broadcast.fixedSize())); err != nil {
		return err
	}

	offset := HeaderLen
	copy(broadcast.blockHash[:], buf[HeaderLen:])
	offset += types.SHA256HashLen
	broadcast.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.NetworkNumLen
	copy(broadcast.sourceID[:], buf[offset:])
	offset += SourceIDLen
	copy(broadcast.broadcastType[:], buf[offset:])
	offset += BroadcastTypeLen
	broadcast.encrypted = int(buf[offset : offset+EncryptedTypeLen][0]) != 0
	offset += EncryptedTypeLen

	if err := checkBufSize(&buf, offset, types.UInt64Len); err != nil {
		return err
	}
	sidsOffset := int(binary.LittleEndian.Uint64(buf[offset:]))

	// sidsOffset includes its types.UInt64Len
	if err := checkBufSize(&buf, offset, sidsOffset); err != nil {
		return err
	}
	broadcast.block = buf[offset+types.UInt64Len : offset+sidsOffset]
	offset += sidsOffset

	if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
		return err
	}
	sidsLen := int(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.UInt32Len

	if err := checkBufSize(&buf, offset, types.UInt32Len*sidsLen); err != nil {
		return err
	}
	for i := 0; i < sidsLen; i++ {
		sid := types.ShortID(binary.LittleEndian.Uint32(buf[offset:]))
		offset += types.UInt32Len
		broadcast.sids = append(broadcast.sids, sid)
	}

	return nil
}

// Size calculate msg size
func (broadcast *Broadcast) Size() uint32 {
	return broadcast.fixedSize() +
		types.UInt64Len + // sids offset
		uint32(len(broadcast.block)) +
		types.UInt32Len + // sids len
		(uint32(len(broadcast.sids)) * types.UInt32Len)
}

func (broadcast *Broadcast) fixedSize() uint32 {
	return broadcast.Header.Size() + types.SHA256HashLen + types.NetworkNumLen + SourceIDLen + BroadcastTypeLen +
		EncryptedTypeLen + ControlByteLen
}
