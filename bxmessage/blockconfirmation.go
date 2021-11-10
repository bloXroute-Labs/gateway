package bxmessage

import (
	log "github.com/sirupsen/logrus"
)

// BlockConfirmation represents a transactions that can be cleaned from tx-service due to block confirmation
type BlockConfirmation struct {
	abstractCleanup
}

func (m *BlockConfirmation) size() uint32 {
	return m.abstractCleanup.size()
}

// Pack serializes a BlockConfirmation into a buffer for sending
func (m BlockConfirmation) Pack(protocol Protocol) ([]byte, error) {
	buf, err := m.abstractCleanup.Pack(protocol)
	m.Header.Pack(&buf, BlockConfirmationTYpe)
	return buf, err
}

// Unpack deserializes a BlockConfirmation from a buffer
func (m *BlockConfirmation) Unpack(buf []byte, Protoco Protocol) error {
	err := m.abstractCleanup.Unpack(buf, Protoco)
	log.Tracef("%v: network %v, sids %v, hashes %v", BlockConfirmationTYpe, m.networkNumber, len(m.ShortIDs), len(m.Hashes))
	return err
}
