package bxmessage

import (
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
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
	return m.abstractCleanup.Pack(protocol, BlockConfirmationType)
}

// Unpack deserializes a BlockConfirmation from a buffer
func (m *BlockConfirmation) Unpack(buf []byte, protocol Protocol) error {
	err := m.abstractCleanup.Unpack(buf, protocol)
	log.Tracef("%v: network %v, sids %v, hashes %v", BlockConfirmationType, m.networkNumber, len(m.ShortIDs), len(m.Hashes))
	return err
}
