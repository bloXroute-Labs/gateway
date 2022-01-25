package bxmessage

import (
	log "github.com/sirupsen/logrus"
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
