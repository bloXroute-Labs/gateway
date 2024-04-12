package bxmessage

import (
	"encoding/binary"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

type beaconMessageType string

// Beacon message types
const (
	beaconMessageTypeBlob beaconMessageType = "blob"
	// Attestations
	// etc
)

func beaconMessageTypeToBx(bType types.BxBeaconMessageType) beaconMessageType {
	switch bType {
	case types.BxBeaconMessageTypeBlob:
		return beaconMessageTypeBlob
	default:
		return ""
	}
}

// NewBeaconMessage creates a new beacon message
func NewBeaconMessage(hash, blockHash types.SHA256Hash, mType types.BxBeaconMessageType, data []byte, index, slot uint32, networkNum types.NetworkNum) *BeaconMessage {
	var beaconMessageType [BeaconMessageTypeLen]byte
	copy(beaconMessageType[:], []byte(beaconMessageTypeToBx(mType)))

	m := &BeaconMessage{
		blockHash:         blockHash,
		beaconMessageType: beaconMessageType,
		data:              data,
		index:             index,
		slot:              slot,
	}
	m.SetHash(hash)
	m.SetNetworkNum(networkNum)

	return m
}

// BeaconMessage defines the structure of a beacon message
type BeaconMessage struct {
	BroadcastHeader

	blockHash         types.SHA256Hash
	beaconMessageType [BeaconMessageTypeLen]byte
	data              []byte
	index             uint32
	slot              uint32
}

// String returns a string representation of the message
func (m *BeaconMessage) String() string {
	return fmt.Sprintf("BeaconMessage(hash: %s, blockHash: %s, type: %s, index: %d, slot: %d)", m.Hash(), m.blockHash, m.Type(), m.index, m.slot)
}

// BlockHash returns the block hash
func (m *BeaconMessage) BlockHash() types.SHA256Hash {
	return m.blockHash
}

// BeaconMessageType returns the beacon message type
func (m *BeaconMessage) BeaconMessageType() types.BxBeaconMessageType {
	return m.Type()
}

// Data returns the data
func (m *BeaconMessage) Data() []byte {
	return m.data
}

// Index returns the index
func (m *BeaconMessage) Index() uint32 {
	return m.index
}

// Slot returns the slot
func (m *BeaconMessage) Slot() uint32 {
	return m.slot
}

// Size returns the size of the message in bytes
func (m *BeaconMessage) Size(protocol Protocol) uint32 {
	return m.BroadcastHeader.Size() + BroadcastTypeLen + types.UInt32Len + uint32(len(m.data)) + types.UInt32Len + types.UInt32Len + types.SHA256HashLen
}

// Pack serializes the message into bytes
func (m *BeaconMessage) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.Size(protocol)

	// Header
	buf := make([]byte, bufLen)
	m.BroadcastHeader.Pack(&buf, BeaconMessageType, protocol)
	offset := BroadcastHeaderOffset

	// Type
	copy(buf[offset:], m.beaconMessageType[:])
	offset += BroadcastTypeLen

	// Data
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(m.data)))
	offset += types.UInt32Len
	copy(buf[offset:], m.data)
	offset += len(m.data)

	// Index
	binary.LittleEndian.PutUint32(buf[offset:], m.index)
	offset += types.UInt32Len

	// Slot
	binary.LittleEndian.PutUint32(buf[offset:], m.slot)
	offset += types.UInt32Len

	// Block hash
	copy(buf[offset:], m.blockHash[:])
	offset += types.SHA256HashLen

	return buf, nil
}

// Unpack deserializes the message from bytes
func (m *BeaconMessage) Unpack(buf []byte, protocol Protocol) error {
	// Header
	if err := m.BroadcastHeader.Unpack(buf, protocol); err != nil {
		return err
	}
	offset := BroadcastHeaderOffset

	// Type
	if err := checkBufSize(&buf, offset, BroadcastTypeLen); err != nil {
		return err
	}
	copy(m.beaconMessageType[:], buf[offset:])
	offset += BroadcastTypeLen

	// Data
	if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
		return err
	}
	dataLen := binary.LittleEndian.Uint32(buf[offset:])
	offset += types.UInt32Len
	if err := checkBufSize(&buf, offset, int(dataLen)); err != nil {
		return err
	}
	m.data = buf[offset : offset+int(dataLen)]
	offset += int(dataLen)

	// Index
	if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
		return err
	}
	m.index = binary.LittleEndian.Uint32(buf[offset:])
	offset += int(types.UInt32Len)

	// Slot
	if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
		return err
	}
	m.slot = binary.LittleEndian.Uint32(buf[offset:])
	offset += int(types.UInt32Len)

	// Block hash
	if err := checkBufSize(&buf, offset, types.SHA256HashLen); err != nil {
		return err
	}
	copy(m.blockHash[:], buf[offset:])
	offset += types.SHA256HashLen

	return nil
}

// Type returns the type of the message
func (m *BeaconMessage) Type() types.BxBeaconMessageType {
	switch string(m.beaconMessageType[:]) {
	case string(beaconMessageTypeBlob):
		return types.BxBeaconMessageTypeBlob
	default:
		return types.BxBeaconMessageTypeUnknown
	}
}
