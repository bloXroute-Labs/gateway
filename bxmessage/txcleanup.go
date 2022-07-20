package bxmessage

import (
	"encoding/binary"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage/utils"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// TxCleanup represents a transactions that can be cleaned from tx-service
type TxCleanup struct {
	abstractCleanup
}

func (m *TxCleanup) size() uint32 {
	return m.abstractCleanup.size()
}

// Pack serializes a SyncTxsMessage into a buffer for sending
func (m TxCleanup) Pack(protocol Protocol) ([]byte, error) {
	buf, err := m.abstractCleanup.Pack(protocol, TxCleanupType)
	return buf, err
}

// Unpack deserializes a SyncTxsMessage from a buffer
func (m *TxCleanup) Unpack(buf []byte, protocol Protocol) error {
	err := m.abstractCleanup.Unpack(buf, protocol)
	log.Tracef("%v: network %v, sids %v, hashes %v", TxCleanupType, m.networkNumber, len(m.ShortIDs), len(m.Hashes))
	return err
}

// SetHash sets hash
func (m *TxCleanup) SetHash() {
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
