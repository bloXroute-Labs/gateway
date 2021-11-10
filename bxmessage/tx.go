package bxmessage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/bloXroute-Labs/gateway/types"
	"github.com/satori/go.uuid"
	log "github.com/sirupsen/logrus"
	"math"
	"time"
)

// Tx is the bloxroute message struct that carries a transaction from a specific blockchain
type Tx struct {
	Header
	hash      types.SHA256Hash
	sourceID  [SourceIDLen]byte
	shortID   types.ShortID
	timestamp time.Time
	flags     types.TxFlags
	accountID [AccountIDLen]byte
	content   []byte
	quota     byte
}

// NewTx constructs a new transaction message, using the provided hash function on the transaction contents to determine the hash
func NewTx(hash types.SHA256Hash, content []byte, networkNum types.NetworkNum, flags types.TxFlags, accountID types.AccountID) *Tx {
	tx := &Tx{}
	tx.SetAccountID(accountID)
	tx.SetFlags(flags)
	tx.SetNetworkNumber(networkNum)
	tx.SetContent(content)
	tx.SetHash(hash)
	tx.SetTimestamp(time.Now())
	return tx
}

// Hash returns the transaction hash
func (m *Tx) Hash() (hash types.SHA256Hash) {
	return m.hash
}

// HashString returns the hex encoded Hash, optionally with 0x prefix
func (m *Tx) HashString(prefix bool) string {
	return m.hash.Format(prefix)
}

// Content returns the blockchain transaction bytes
func (m *Tx) Content() (content []byte) {
	return m.content
}

// ShortID returns the assigned short ID of the transaction
func (m *Tx) ShortID() (sid types.ShortID) {
	return m.shortID
}

// SourceID returns the source ID of transaction in string format
func (m *Tx) SourceID() (sourceID types.NodeID) {
	u, err := uuid.FromBytes(m.sourceID[:])
	if err != nil {
		log.Errorf("Failed to parse source id from tx message, raw bytes: %v", m.content)
		return
	}
	return types.NodeID(u.String())
}

// Timestamp indicates when the TxMessage was created. This attribute is only set by relays.
func (m *Tx) Timestamp() time.Time {
	return m.timestamp
}

// AccountID indicates the account ID the TxMessage originated from
func (m *Tx) AccountID() types.AccountID {
	if bytes.Equal(m.accountID[:], NullByteAccountID) {
		return ""
	}
	return types.NewAccountID(m.accountID[:])
}

// SetAccountID sets the account ID
func (m *Tx) SetAccountID(accountID types.AccountID) {
	copy(m.accountID[:], accountID)
}

// SetHash sets the transaction hash
func (m *Tx) SetHash(hash [32]byte) {
	m.hash = hash
}

// SetContent sets the transaction content
func (m *Tx) SetContent(content []byte) {
	m.content = content
}

// SetShortID sets the assigned short ID
func (m *Tx) SetShortID(sid types.ShortID) {
	m.shortID = sid
}

// SetTimestamp sets the message creation timestamp
func (m *Tx) SetTimestamp(timestamp time.Time) {
	m.timestamp = timestamp
}

// ClearProtectedAttributes unsets fields that cannot be set by external gateways
func (m *Tx) ClearProtectedAttributes() {
	m.ClearInternalAttributes()
	m.shortID = types.ShortIDEmpty
	m.accountID = [AccountIDLen]byte{}
	m.flags &= ^types.TFDeliverToNode
}

// ClearInternalAttributes unsets fields that should not be seen by external gateways
func (m *Tx) ClearInternalAttributes() {
	// TODO: discuss - why do we have a timestamp in tx? should we clear it? should GW set it?
	//m.timestamp = time.Unix(0, 0)
	m.sourceID = [SourceIDLen]byte{}
	m.flags &= ^types.TFEnterpriseSender
	m.flags &= ^types.TFEliteSender
}

// Flags returns the transaction flags
func (m *Tx) Flags() (flags types.TxFlags) {
	return m.flags
}

// AddFlags adds the provided flag to the transaction flag set
func (m *Tx) AddFlags(flags types.TxFlags) {
	m.flags |= flags
}

// SetFlags sets the message flags
func (m *Tx) SetFlags(flags types.TxFlags) {
	m.flags = flags
}

// RemoveFlags sets off txFlag
func (m *Tx) RemoveFlags(flags types.TxFlags) {
	m.SetFlags(m.Flags() &^ flags)
}

// IsPaid indicates whether the transaction is considered paid from the user and consumes quota
func (m *Tx) IsPaid() bool {
	return m.flags&types.TFPaidTx != 0
}

// SetNetworkNumber sets the network number
func (m *Tx) SetNetworkNumber(networkNumber types.NetworkNum) {
	m.networkNumber = networkNumber
}

// NetworkNumber provides the transaction network number
func (m Tx) NetworkNumber() types.NetworkNum {
	return m.networkNumber
}

// SetSourceID sets the source id of the tx
func (m *Tx) SetSourceID(sourceID types.NodeID) {
	sourceIDBytes, err := uuid.FromString(string(sourceID))
	if err != nil {
		log.Errorf("Failed to set source id, source id: %v", sourceIDBytes)
		return
	}

	copy(m.sourceID[:], sourceIDBytes[:])
}

// CompactClone returns a shallow clone of the current transaction, with the content omitted
func (m *Tx) CompactClone() Tx {
	tx := Tx{
		Header:    m.Header,
		hash:      m.hash,
		sourceID:  m.sourceID,
		shortID:   m.shortID,
		timestamp: m.timestamp,
		flags:     m.flags,
		accountID: m.accountID,
		content:   nil,
		quota:     m.quota,
	}
	tx.ClearInternalAttributes()
	return tx
}

// CleanClone returns a shallow clone of the current transaction, with the internal attributes removed
func (m *Tx) CleanClone() Tx {
	tx := Tx{
		Header:    m.Header,
		hash:      m.hash,
		sourceID:  m.sourceID,
		shortID:   m.shortID,
		timestamp: m.timestamp,
		flags:     m.flags,
		accountID: m.accountID,
		content:   m.content,
		quota:     m.quota,
	}
	tx.ClearInternalAttributes()
	return tx
}

func (m *Tx) size(protocol Protocol) uint32 {
	switch {
	case protocol < 19:
		return 0
	case protocol < 21:
		return m.Header.Size() + types.SHA256HashLen + types.NetworkNumLen + SourceIDLen + types.ShortIDLen + types.TxFlagsLen + 4 +
			uint32(len(m.content)) + 1
	case protocol < 22:
		return m.Header.Size() + types.SHA256HashLen + types.NetworkNumLen + SourceIDLen + types.ShortIDLen + types.TxFlagsLen +
			TimestampLen + uint32(len(m.content)) + 1
	}
	return m.Header.Size() + types.SHA256HashLen + types.NetworkNumLen + SourceIDLen + types.ShortIDLen + types.TxFlagsLen +
		TimestampLen + AccountIDLen + uint32(len(m.content)) + 1
}

// String serializes a Tx as a string for pretty printing
func (m Tx) String() string {
	return fmt.Sprintf("Tx<hash: %v, priority: %v, flags: %v>", m.HashString(false), m.priority, m.flags)
}

// Pack serializes a Tx into a buffer for sending
func (m Tx) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size(protocol)
	buf := make([]byte, bufLen)
	offset := HeaderLen

	copy(buf[offset:], m.hash[:])
	offset += types.SHA256HashLen
	binary.LittleEndian.PutUint32(buf[offset:], uint32(m.networkNumber))
	offset += types.NetworkNumLen
	copy(buf[offset:], m.sourceID[:])
	offset += SourceIDLen
	binary.LittleEndian.PutUint32(buf[offset:], uint32(m.shortID))
	offset += types.ShortIDLen
	flags := m.flags
	switch {
	case protocol < 20:
		flags &= types.TFStatusMonitoring | types.TFPaidTx | types.TFNonceMonitoring | types.TFRePropagate | types.TFEnterpriseSender
	default:
	}
	binary.LittleEndian.PutUint16(buf[offset:], uint16(flags))

	offset += types.TxFlagsLen
	timestamp := float64(m.timestamp.UnixNano()) / 1e9
	switch {
	case protocol < 21:
		binary.LittleEndian.PutUint32(buf[offset:], uint32(timestamp+1.0))
		offset += 4
	default:
		binary.LittleEndian.PutUint64(buf[offset:], math.Float64bits(timestamp))
		offset += TimestampLen
	}
	switch {
	case protocol < 22:
		// do nothing. accountID added in protocol 22
	default:
		copy(buf[offset:], m.accountID[:])
		offset += AccountIDLen
	}
	copy(buf[offset:], m.content)
	offset += len(m.content)

	buf[bufLen-1] = 0x01
	m.Header.Pack(&buf, TxType)
	return buf, nil
}

// Unpack deserializes a Tx from a buffer
func (m *Tx) Unpack(buf []byte, protocol Protocol) error {
	offset := HeaderLen
	copy(m.hash[:], buf[offset:])
	offset += types.SHA256HashLen
	m.networkNumber = types.NetworkNum(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.NetworkNumLen
	copy(m.sourceID[:], buf[offset:])
	offset += SourceIDLen
	m.shortID = types.ShortID(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.ShortIDLen
	m.flags = types.TxFlags(binary.LittleEndian.Uint16(buf[offset:]))
	offset += types.TxFlagsLen
	timestamp := float64(0)
	switch {
	case protocol < 21:
		timestamp = float64(binary.LittleEndian.Uint32(buf[offset:]))
		offset += 4
	default:
		timestamp = math.Float64frombits(binary.LittleEndian.Uint64(buf[78:]))
		offset += TimestampLen
	}
	nanoseconds := int64(timestamp) * int64(1e9)
	m.timestamp = time.Unix(0, nanoseconds)
	switch {
	case protocol < 22:
		// do nothing. accountID added in protocol 22
	default:
		copy(m.accountID[:], buf[offset:])
		offset += AccountIDLen
	}

	m.content = buf[offset : len(buf)-1]
	offset += len(m.content)

	return nil
}
