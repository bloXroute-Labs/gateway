package bxmessage

import (
	"encoding/binary"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/bxmessage/utils"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/types"
	uuid "github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
)

// abstractCleanup represents a transactions that can be cleaned from tx-service
type abstractCleanup struct {
	Header
	hash     types.SHA256Hash
	sourceID [SourceIDLen]byte
	Hashes   types.SHA256HashList
	ShortIDs types.ShortIDList
}

// Hash provides the cleanup message hash
func (m *abstractCleanup) Hash() types.SHA256Hash {
	return m.hash
}

// SetHash sets hash
func (m *abstractCleanup) SetHash() {
	bufLen := (len(m.ShortIDs) * types.UInt32Len) + (len(m.Hashes) * types.SHA256HashLen)
	buf := make([]byte, bufLen)

	offset := 0
	for i := 0; i < len(m.ShortIDs); i++ {
		binary.LittleEndian.PutUint32(buf[offset:], uint32(m.ShortIDs[i]))
		offset += types.UInt32Len
	}

	for i := 0; i < len(m.Hashes); i++ {
		copy(buf[offset:], m.Hashes[i][:])
		offset += types.SHA256HashLen
	}

	m.hash = utils.DoubleSHA256(buf[:])
}

// SetSourceID sets the source ID
func (m *abstractCleanup) SetSourceID(sourceID types.NodeID) error {
	sourceIDBytes, err := uuid.FromString(string(sourceID))
	if err == nil {
		copy(m.sourceID[:], sourceIDBytes[:])
	}

	return err
}

// SourceID gets sourceID
func (m *abstractCleanup) SourceID() (SourceID types.NodeID) {
	u, err := uuid.FromBytes(m.sourceID[:])
	if err != nil {
		log.Errorf("Failed to parse source id from cleanup message, raw bytes: %v", m.sourceID)
		return
	}
	return types.NodeID(u.String())
}

func (m *abstractCleanup) size() uint32 {
	return uint32(HeaderLen + types.SHA256HashLen + types.NetworkNumLen + SourceIDLen + 2*types.UInt32Len +
		len(m.Hashes)*types.SHA256HashLen + len(m.ShortIDs)*types.ShortIDLen + 1)
}

// Pack serializes a cleanup message into a buffer for sending
func (m abstractCleanup) Pack(_ Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)

	offset := HeaderLen
	copy(buf[offset:], m.hash[:])
	offset += types.SHA256HashLen
	binary.LittleEndian.PutUint32(buf[offset:], uint32(m.networkNumber))
	offset += types.NetworkNumLen
	copy(buf[offset:], m.sourceID[:])
	offset += SourceIDLen
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

	buf[bufLen-1] = ControlByte
	return buf, nil
}

// Unpack deserializes a cleanup message from a buffer
func (m *abstractCleanup) Unpack(buf []byte, _ Protocol) error {
	offset := HeaderLen
	copy(m.hash[:], buf[offset:])
	offset += types.SHA256HashLen
	m.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.NetworkNumLen
	copy(m.sourceID[:], buf[offset:])
	offset += SourceIDLen
	sidCount := binary.LittleEndian.Uint32(buf[offset:])
	offset += types.UInt32Len
	for i := 0; i < int(sidCount); i++ {
		sid := types.ShortID(binary.LittleEndian.Uint32(buf[offset:]))
		offset += types.UInt32Len
		m.ShortIDs = append(m.ShortIDs, sid)
	}
	hashCount := binary.LittleEndian.Uint32(buf[offset:])
	offset += types.UInt32Len
	for i := 0; i < int(hashCount); i++ {
		var hash types.SHA256Hash
		copy(hash[:], buf[offset:])
		offset += types.SHA256HashLen
		m.Hashes = append(m.Hashes, hash)
	}
	return nil
}
