package bxmessage

import (
	"encoding/binary"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// abstractCleanup represents a transactions that can be cleaned from tx-service
type abstractCleanup struct {
	BroadcastHeader
	Hashes   types.SHA256HashList
	ShortIDs types.ShortIDList
}

// Pack serializes a cleanup message into a buffer for sending
func (m abstractCleanup) Pack(protocol Protocol, msgType string) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)

	m.BroadcastHeader.Pack(&buf, msgType, protocol)
	offset := BroadcastHeaderOffset
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(m.ShortIDs)))
	offset += types.UInt32Len
	for i := 0; i < len(m.ShortIDs); i++ {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(m.ShortIDs[i]))
		offset += types.UInt32Len
	}
	binary.LittleEndian.PutUint32(buf[offset:], uint32(len(m.Hashes)))
	offset += types.UInt32Len
	for i := 0; i < len(m.Hashes); i++ {
		copy(buf[offset:], m.Hashes[i][:])
		offset += types.SHA256HashLen
	}

	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack deserializes a cleanup message from a buffer
func (m *abstractCleanup) Unpack(buf []byte, protocol Protocol) error {
	offset := HeaderLen

	if err := checkBufSize(&buf, offset, types.SHA256HashLen); err != nil {
		return err
	}
	copy(m.hash[:], buf[offset:])
	offset += types.SHA256HashLen

	if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
		return err
	}
	m.networkNumber = bxtypes.NetworkNum(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.NetworkNumLen

	if err := checkBufSize(&buf, offset, SourceIDLen); err != nil {
		return err
	}
	copy(m.sourceID[:], buf[offset:])
	offset += SourceIDLen

	if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
		return err
	}
	sidCount := binary.LittleEndian.Uint32(buf[offset:])
	offset += types.UInt32Len
	for i := 0; i < int(sidCount); i++ {
		if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
			return err
		}
		sid := types.ShortID(binary.LittleEndian.Uint32(buf[offset:]))
		offset += types.UInt32Len
		m.ShortIDs = append(m.ShortIDs, sid)
	}

	if err := checkBufSize(&buf, offset, types.UInt32Len); err != nil {
		return err
	}
	hashCount := binary.LittleEndian.Uint32(buf[offset:])
	offset += types.UInt32Len
	for i := 0; i < int(hashCount); i++ {
		if err := checkBufSize(&buf, offset, types.SHA256HashLen); err != nil {
			return err
		}
		var hash types.SHA256Hash
		copy(hash[:], buf[offset:])
		offset += types.SHA256HashLen
		m.Hashes = append(m.Hashes, hash)
	}
	return m.BroadcastHeader.Unpack(buf, protocol)
}

func (m *abstractCleanup) size() uint32 {
	return m.BroadcastHeader.Size() + uint32(2*types.UInt32Len+len(m.Hashes)*types.SHA256HashLen+len(m.ShortIDs)*types.ShortIDLen)
}
