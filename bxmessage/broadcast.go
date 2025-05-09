package bxmessage

import (
	"encoding/binary"
	"fmt"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

type broadcastType string

const (
	broadcastTypeEth           broadcastType = "blck"
	broadcastTypeBeaconDeneb   broadcastType = "bcnd"
	broadcastTypeBeaconElectra broadcastType = "bcne"
)

// Broadcast - represent the "broadcast" message
type Broadcast struct {
	BroadcastHeader
	broadcastType [BroadcastTypeLen]byte
	encrypted     bool
	block         []byte
	sids          types.ShortIDList
	beaconHash    types.SHA256Hash
}

func blockToBroadcastType(blockType types.BxBlockType) broadcastType {
	switch blockType {
	case types.BxBlockTypeBeaconDeneb:
		return broadcastTypeBeaconDeneb
	case types.BxBlockTypeBeaconElectra:
		return broadcastTypeBeaconElectra
	case types.BxBlockTypeEth:
		fallthrough
	default:
		return broadcastTypeEth
	}
}

// NewBlockBroadcast creates a new broadcast message containing block message bytes
func NewBlockBroadcast(hash, beaconHash types.SHA256Hash, bType types.BxBlockType, block []byte, shortIDs types.ShortIDList, networkNum bxtypes.NetworkNum) *Broadcast {
	var broadcastType [BroadcastTypeLen]byte
	copy(broadcastType[:], blockToBroadcastType(bType))

	b := &Broadcast{
		broadcastType: broadcastType,
		encrypted:     false,
		block:         block,
		sids:          shortIDs,
	}
	b.SetHash(hash)
	b.SetBeaconHash(beaconHash)
	b.SetNetworkNum(networkNum)
	return b
}

// String implements Stringer interface
func (b Broadcast) String() string {
	if b.IsBeaconBlock() {
		return fmt.Sprintf("broadcast beacon(hash: %s, type: %s, network: %d, short txs: %d)", b.beaconHash, b.broadcastType, b.networkNumber, len(b.ShortIDs()))
	}

	return fmt.Sprintf("broadcast(hash: %s, type: %s, network: %d, short txs: %d)", b.hash, b.broadcastType, b.networkNumber, len(b.ShortIDs()))
}

// IsBeaconBlock returns true if block is beacon
func (b *Broadcast) IsBeaconBlock() bool {
	switch broadcastType(b.broadcastType[:]) {
	case broadcastTypeBeaconDeneb, broadcastTypeBeaconElectra:
		return true
	default:
		return false
	}
}

// BlockType returns block type
func (b Broadcast) BlockType() types.BxBlockType {
	switch broadcastType(b.broadcastType[:]) {
	case broadcastTypeEth:
		return types.BxBlockTypeEth
	case broadcastTypeBeaconDeneb:
		return types.BxBlockTypeBeaconDeneb
	case broadcastTypeBeaconElectra:
		return types.BxBlockTypeBeaconElectra
	default:
		return types.BxBlockTypeUnknown
	}
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

// SetBeaconHash sets the beacon block hash
func (b *Broadcast) SetBeaconHash(hash types.SHA256Hash) {
	b.beaconHash = hash
}

// BeaconHash returns the beacon block hash
func (b *Broadcast) BeaconHash() (hash types.SHA256Hash) {
	return b.beaconHash
}

// SetSids sets the sids
func (b *Broadcast) SetSids(sids types.ShortIDList) {
	b.sids = sids
}

// Pack serializes a Broadcast into a buffer for sending
func (b *Broadcast) Pack(protocol Protocol) ([]byte, error) {
	bufLen := b.Size(protocol)
	buf := make([]byte, bufLen)
	b.BroadcastHeader.Pack(&buf, BroadcastType, protocol)
	offset := BroadcastHeaderOffset
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

	// Put in the end to provide back compatibility
	if b.IsBeaconBlock() {
		copy(buf[offset:], b.beaconHash[:])
		offset += types.SHA256HashLen
	}

	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack deserializes a Broadcast from a buffer
func (b *Broadcast) Unpack(buf []byte, protocol Protocol) error {
	if err := b.BroadcastHeader.Unpack(buf, protocol); err != nil {
		return err
	}
	offset := BroadcastHeaderOffset

	if err := checkBufSize(&buf, offset, BroadcastTypeLen); err != nil {
		return err
	}
	copy(b.broadcastType[:], buf[offset:])
	offset += BroadcastTypeLen

	if err := checkBufSize(&buf, offset, EncryptedTypeLen); err != nil {
		return err
	}
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

	// Put in the end to provide back compatibility
	if b.IsBeaconBlock() {
		copy(b.beaconHash[:], buf[offset:])
		offset += types.SHA256HashLen
	}

	return nil
}

// Size calculate msg size
func (b *Broadcast) Size(protocol Protocol) uint32 {
	size := b.fixedSize() +
		types.UInt64Len + // sids offset
		uint32(len(b.block)) +
		types.UInt32Len + // sids len
		(uint32(len(b.sids)) * types.UInt32Len)

	if b.IsBeaconBlock() {
		size += types.SHA256HashLen // beacon hash
	}

	return size
}

func (b *Broadcast) fixedSize() uint32 {
	return b.BroadcastHeader.Size() + BroadcastTypeLen + EncryptedTypeLen
}
