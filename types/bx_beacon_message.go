package types

import "fmt"

// BxBeaconMessageType represents the type of a beacon message
type BxBeaconMessageType int

// String implements Stringer interface
func (t BxBeaconMessageType) String() string {
	switch t {
	case BxBeaconMessageTypeBlob:
		return "Blob"
	default:
		return "Unknown"
	}
}

// BxBeaconMessageType constants
const (
	BxBeaconMessageTypeUnknown BxBeaconMessageType = iota
	BxBeaconMessageTypeBlob
)

// BxBeaconMessage represents a beacon message
type BxBeaconMessage struct {
	Hash      SHA256Hash
	BlockHash SHA256Hash
	Data      []byte
	Type      BxBeaconMessageType
	Index     uint32
	Slot      uint32
}

// NewBxBeaconMessage creates a new BxBeaconMessage
func NewBxBeaconMessage(hash, blockHash SHA256Hash, mType BxBeaconMessageType, data []byte, index uint32, slot uint32) *BxBeaconMessage {
	return &BxBeaconMessage{
		Hash:      hash,
		BlockHash: blockHash,
		Type:      mType,
		Data:      data,
		Index:     index,
		Slot:      slot,
	}
}

// String implements Stringer interface
func (b *BxBeaconMessage) String() string {
	return fmt.Sprintf("BxBeaconMessage{Hash: %v, BlockHash: %v, Type: %v, Index: %v, Slot: %v}", b.Hash, b.BlockHash, b.Type, b.Index, b.Slot)
}
