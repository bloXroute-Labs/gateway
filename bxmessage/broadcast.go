package bxmessage

import (
	"encoding/binary"
	"github.com/bloXroute-Labs/gateway/types"
)

// Broadcast - represent the "broadcast" message
type Broadcast struct {
	BroadcastHeader
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
		broadcastType: broadcastType,
		encrypted:     false,
		block:         block,
		sids:          shortIDs,
	}
	b.SetHash(hash)
	b.SetNetworkNum(networkNum)
	return b
}

// BroadcastType returns the broadcast type
func (b Broadcast) BroadcastType() [BroadcastTypeLen]byte {
	return b.broadcastType
}

// Encrypted returns the encrypted byte
func (b Broadcast) Encrypted() bool {
	return b.encrypted
}

// Block returns the block
func (b Broadcast) Block() []byte {
	return b.block
}

// ShortIDs return sids
func (b Broadcast) ShortIDs() types.ShortIDList {
	return b.sids
}

// SetBroadcastType sets the broadcast type
func (b *Broadcast) SetBroadcastType(broadcastType [BroadcastTypeLen]byte) {
	b.broadcastType = broadcastType
}

// SetEncrypted sets the encrypted byte
func (b *Broadcast) SetEncrypted(encrypted bool) {
	b.encrypted = encrypted
}

// SetBlock sets the block
func (b *Broadcast) SetBlock(block []byte) {
	b.block = block
}

// SetSids sets the sids
func (b *Broadcast) SetSids(sids types.ShortIDList) {
	b.sids = sids
}

// Pack serializes a Broadcast into a buffer for sending
func (b *Broadcast) Pack(protocol Protocol) ([]byte, error) {
	bufLen := b.Size()
	buf := make([]byte, bufLen)
	b.BroadcastHeader.Pack(&buf, BroadcastType)
	offset := BroadcastHeaderLen
	copy(buf[offset:], b.broadcastType[:])
	offset += BroadcastTypeLen
	if b.encrypted {
		copy(buf[offset:], []uint8{1})
	} else {
		copy(buf[offset:], []uint8{0})
	}
	offset += EncryptedTypeLen
	binary.LittleEndian.PutUint64(buf[offset:], uint64(len(b.block)+types.UInt64Len))
	offset += types.UInt64Len
	copy(buf[offset:], b.block)
	offset += len(b.block)
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(b.sids)))
	offset += types.UInt32Len
	for _, sid := range b.sids {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(sid))
		offset += types.UInt32Len
	}
	return buf, nil
}

// Unpack deserializes a Broadcast from a buffer
func (b *Broadcast) Unpack(buf []byte, protocol Protocol) error {
	if err := b.BroadcastHeader.Unpack(buf, protocol); err != nil {
		return err
	}

	offset := BroadcastHeaderLen
	copy(b.broadcastType[:], buf[offset:])
	offset += BroadcastTypeLen
	b.encrypted = int(buf[offset : offset+EncryptedTypeLen][0]) != 0
	offset += EncryptedTypeLen

	if err := checkBufSize(&buf, offset, types.UInt64Len); err != nil {
		return err
	}
	sidsOffset := int(binary.LittleEndian.Uint64(buf[offset:]))

	// sidsOffset includes its types.UInt64Len
	if err := checkBufSize(&buf, offset, sidsOffset); err != nil {
		return err
	}
	b.block = buf[offset+types.UInt64Len : offset+sidsOffset]
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
		b.sids = append(b.sids, sid)
	}

	return nil
}

// Size calculate msg size
func (b *Broadcast) Size() uint32 {
	return b.fixedSize() +
		types.UInt64Len + // sids offset
		uint32(len(b.block)) +
		types.UInt32Len + // sids len
		(uint32(len(b.sids)) * types.UInt32Len)
}

func (b *Broadcast) fixedSize() uint32 {
	return b.BroadcastHeader.Size() + BroadcastTypeLen + EncryptedTypeLen
}
