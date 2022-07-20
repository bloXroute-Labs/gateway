package bxmessage

import (
	"encoding/binary"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"time"
)

// SyncTxContentsShortIDs represents information about a sync transaction received over sync
type SyncTxContentsShortIDs struct {
	Hash         types.SHA256Hash
	Content      types.TxContent
	timestamp    time.Time
	ShortIDs     types.ShortIDList
	ShortIDFlags []types.TxFlags
}

// Timestamp return the tx creation time
func (m *SyncTxContentsShortIDs) Timestamp() time.Time {
	return m.timestamp
}

// SetTimestamp sets the message creation timestamp
func (m *SyncTxContentsShortIDs) SetTimestamp(timestamp time.Time) {
	m.timestamp = timestamp
}

// SyncTxsMessage represents a chunk of transactions received during sync
type SyncTxsMessage struct {
	Header
	networkNumber   types.NetworkNum
	ContentShortIds []SyncTxContentsShortIDs
	_size           int

	//ShortIDs int
}

// GetNetworkNum gets the message network number
func (m *SyncTxsMessage) GetNetworkNum() types.NetworkNum {
	return m.networkNumber
}

// SetNetworkNum sets the message network number
func (m *SyncTxsMessage) SetNetworkNum(networkNum types.NetworkNum) {
	m.networkNumber = networkNum
}

// Add adds another transaction into the message
func (m *SyncTxsMessage) Add(txInfo *types.BxTransaction) uint32 {
	// don't sync transactions without short ID
	if len(txInfo.ShortIDs()) == 0 {
		return 0
	}
	shortIDs := txInfo.ShortIDs()
	csi := SyncTxContentsShortIDs{
		Hash:         txInfo.Hash(),
		Content:      txInfo.Content(),
		timestamp:    txInfo.AddTime(),
		ShortIDs:     make(types.ShortIDList, len(shortIDs)),
		ShortIDFlags: make([]types.TxFlags, len(shortIDs)),
	}
	for i, shortID := range shortIDs {
		csi.ShortIDs[i] = shortID
		csi.ShortIDFlags[i] = txInfo.Flags()
	}
	m.ContentShortIds = append(m.ContentShortIds, csi)
	m._size +=
		types.SHA256HashLen + //hash
			types.UInt32Len + // content size
			len(csi.Content) + // content itself
			types.UInt16Len + // shortIds count
			types.UInt32Len + // transaction creation timestamp
			len(csi.ShortIDs)*types.UInt32Len + // shortIds
			len(csi.ShortIDFlags)*types.TxFlagsLen // flags
	return m.size()
}

func (m *SyncTxsMessage) size() uint32 {
	return m.Header.Size() + uint32(types.NetworkNumLen+types.UInt32Len+m._size)
}

// Count provides the number of tx included in the SyncTxsMessage
func (m *SyncTxsMessage) Count() int {
	return len(m.ContentShortIds)
}

// Pack serializes a SyncTxsMessage into a buffer for sending
func (m SyncTxsMessage) Pack(_ Protocol) ([]byte, error) {
	bufLen := m.size()
	buf := make([]byte, bufLen)
	binary.LittleEndian.PutUint32(buf[HeaderLen:], uint32(m.networkNumber))
	binary.LittleEndian.PutUint32(buf[HeaderLen+types.NetworkNumLen:], uint32(len(m.ContentShortIds)))
	m.packContentShortIds(&buf, HeaderLen+types.NetworkNumLen+types.UInt32Len)
	m.Header.Pack(&buf, SyncTxsType)
	return buf, nil
}

// Unpack deserializes a SyncTxsMessage from a buffer
func (m *SyncTxsMessage) Unpack(buf []byte, protocol Protocol) error {
	if err := checkBufSize(&buf, HeaderLen, types.NetworkNumLen+types.UInt32Len); err != nil {
		return err
	}
	m.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[HeaderLen:]))
	txCount := binary.LittleEndian.Uint32(buf[HeaderLen+types.NetworkNumLen:])
	log.Tracef("%v: size %v, network %v, txcount %v", SyncTxsType, len(buf), m.networkNumber, txCount)
	err := m.unpackContentShortIds(&buf, HeaderLen+types.NetworkNumLen+types.UInt32Len, txCount)
	if err != nil {
		return err
	}
	return m.Header.Unpack(buf, protocol)

}

func (m *SyncTxsMessage) packContentShortIds(buf *[]byte, offset int) int {
	for _, csi := range m.ContentShortIds {
		copy((*buf)[offset:], csi.Hash[:])
		offset += types.SHA256HashLen
		binary.LittleEndian.PutUint32((*buf)[offset:], uint32(len(csi.Content)))
		offset += types.UInt32Len
		copy((*buf)[offset:], csi.Content[:])
		offset += len(csi.Content)
		timestamp := uint32(csi.timestamp.UnixNano() / int64(1e9))
		binary.LittleEndian.PutUint32((*buf)[offset:], timestamp)
		offset += types.UInt32Len
		binary.LittleEndian.PutUint16((*buf)[offset:], uint16(len(csi.ShortIDs)))
		offset += types.UInt16Len
		for _, shortID := range csi.ShortIDs {
			// is there a need to read it or just cast the memory?
			binary.LittleEndian.PutUint32((*buf)[offset:], uint32(shortID))
			offset += types.UInt32Len
		}
		for _, flags := range csi.ShortIDFlags {
			binary.LittleEndian.PutUint16((*buf)[offset:], uint16(flags))
			offset += types.UInt16Len
		}

	}
	return offset
}

func (m *SyncTxsMessage) unpackContentShortIds(buf *[]byte, offset int, txCount uint32) error {
	m.ContentShortIds = make([]SyncTxContentsShortIDs, txCount)
	for i := 0; i < int(txCount); i++ {
		if err := checkBufSize(buf, offset, types.SHA256HashLen+types.UInt32Len); err != nil {
			return err
		}
		copy(m.ContentShortIds[i].Hash[:], (*buf)[offset:])
		offset += types.SHA256HashLen
		ContentSize := int(binary.LittleEndian.Uint32((*buf)[offset:]))
		offset += types.UInt32Len
		if err := checkBufSize(buf, offset, ContentSize); err != nil {
			return err
		}
		m.ContentShortIds[i].Content = (*buf)[offset : offset+ContentSize]
		offset += ContentSize
		if err := checkBufSize(buf, offset, types.UInt32Len+types.UInt16Len); err != nil {
			return err
		}
		timestamp := int64(binary.LittleEndian.Uint32((*buf)[offset:])) * int64(1e9)
		m.ContentShortIds[i].timestamp = time.Unix(0, timestamp)
		offset += types.UInt32Len
		ShortIdsCount := int(binary.LittleEndian.Uint16((*buf)[offset:]))
		offset += types.UInt16Len
		m.ContentShortIds[i].ShortIDs = make(types.ShortIDList, ShortIdsCount)
		m.ContentShortIds[i].ShortIDFlags = make([]types.TxFlags, ShortIdsCount)

		if err := checkBufSize(buf, offset, ShortIdsCount*types.UInt32Len); err != nil {
			return err
		}
		for j := 0; j < ShortIdsCount; j++ {
			// is there a need to read it or just cast the memory?
			m.ContentShortIds[i].ShortIDs[j] = types.ShortID(binary.LittleEndian.Uint32((*buf)[offset:]))
			offset += types.UInt32Len
		}
		if err := checkBufSize(buf, offset, ShortIdsCount*types.UInt16Len); err != nil {
			return err
		}
		for j := 0; j < ShortIdsCount; j++ {
			// read short ID flags
			m.ContentShortIds[i].ShortIDFlags[j] = types.TxFlags(binary.LittleEndian.Uint16((*buf)[offset:]))
			offset += types.UInt16Len
		}
	}
	return nil
}
