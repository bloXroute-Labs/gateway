package bxmessage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/bloXroute-Labs/gateway/types"
	"math"
	"time"
)

// Tx is the bloxroute message struct that carries a transaction from a specific blockchain
type Tx struct {
	BroadcastHeader
	shortID   types.ShortID
	flags     types.TxFlags
	timestamp time.Time
	accountID [AccountIDLen]byte
	content   []byte
	quota     byte
}

// NewTx constructs a new transaction message, using the provided hash function on the transaction contents to determine the hash
func NewTx(hash types.SHA256Hash, content []byte, networkNum types.NetworkNum, flags types.TxFlags, accountID types.AccountID) *Tx {
	tx := &Tx{}

	if accountID != types.EmptyAccountID {
		if !flags.IsPaid() {
			panic("invalid usage: account ID cannot be set on unpaid transaction")
		}
		tx.SetAccountID(accountID)
	}

	if accountID == types.EmptyAccountID && flags.IsPaid() {
		panic("invalid usage: account ID must be set on paid transaction")
	}

	tx.SetFlags(flags)
	tx.SetNetworkNum(networkNum)
	tx.SetContent(content)
	tx.SetHash(hash)
	tx.SetTimestamp(time.Now())
	return tx
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

// ClearProtectedAttributes unsets and validates fields that are restricted for gateways
func (m *Tx) ClearProtectedAttributes() {
	m.shortID = types.ShortIDEmpty
	m.timestamp = time.Unix(0, 0)
	m.sourceID = [SourceIDLen]byte{}

	m.flags &= ^types.TFDeliverToNode
	m.flags &= ^types.TFEnterpriseSender
	m.flags &= ^types.TFEliteSender

	// account ID should only be set on paid txs
	if !m.Flags().IsPaid() {
		m.accountID = [AccountIDLen]byte{}
	}
}

// ClearInternalAttributes unsets fields that should not be seen by gateways
func (m *Tx) ClearInternalAttributes() {
	// not reset timestamp. Gateways use it to see if transaction is old or not
	m.sourceID = [SourceIDLen]byte{}
	m.accountID = [AccountIDLen]byte{}

	m.flags &= ^types.TFEnterpriseSender
	m.flags &= ^types.TFEliteSender
	m.flags &= ^types.TFPaidTx
	m.flags &= ^types.TFValidatorsOnly
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

// CompactClone returns a shallow clone of the current transaction, with the content omitted
func (m *Tx) CompactClone() Tx {
	tx := Tx{
		BroadcastHeader: m.BroadcastHeader,
		shortID:         m.shortID,
		timestamp:       m.timestamp,
		flags:           m.flags,
		accountID:       m.accountID,
		content:         nil,
		quota:           m.quota,
	}
	tx.ClearInternalAttributes()
	return tx
}

// CleanClone returns a shallow clone of the current transaction, with the internal attributes removed
func (m *Tx) CleanClone() Tx {
	tx := Tx{
		BroadcastHeader: m.BroadcastHeader,
		shortID:         m.shortID,
		timestamp:       m.timestamp,
		flags:           m.flags,
		accountID:       m.accountID,
		content:         m.content,
		quota:           m.quota,
	}
	tx.ClearInternalAttributes()
	return tx
}

// Pack serializes a Tx into a buffer for sending
func (m Tx) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.size(protocol)
	buf := make([]byte, bufLen)
	m.BroadcastHeader.Pack(&buf, TxType)
	offset := BroadcastHeaderLen

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
		offset += types.UInt32Len
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

	return buf, nil
}

// Unpack deserializes a Tx from a buffer
func (m *Tx) Unpack(buf []byte, protocol Protocol) error {
	if err := m.BroadcastHeader.Unpack(buf, protocol); err != nil {
		return err
	}
	offset := BroadcastHeaderLen
	m.shortID = types.ShortID(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.ShortIDLen
	m.flags = types.TxFlags(binary.LittleEndian.Uint16(buf[offset:]))
	offset += types.TxFlagsLen
	timestamp := float64(0)
	switch {
	case protocol < 21:
		timestamp = float64(binary.LittleEndian.Uint32(buf[offset:]))
		offset += types.UInt32Len
	default:
		timestamp = math.Float64frombits(binary.LittleEndian.Uint64(buf[offset:]))
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

	m.content = buf[offset : len(buf)-ControlByteLen]
	offset += len(m.content)
	return nil
}

func (m *Tx) size(protocol Protocol) uint32 {
	switch {
	case protocol < 19:
		return 0
	case protocol < 21:
		return m.BroadcastHeader.Size() + types.ShortIDLen + types.TxFlagsLen + types.UInt32Len + uint32(len(m.content))
	case protocol < 22:
		return m.BroadcastHeader.Size() + types.ShortIDLen + types.TxFlagsLen + TimestampLen + uint32(len(m.content))
	}
	return m.BroadcastHeader.Size() + types.ShortIDLen + types.TxFlagsLen + TimestampLen + AccountIDLen + uint32(len(m.content))
}

// String serializes a Tx as a string for pretty printing
func (m Tx) String() string {
	return fmt.Sprintf("Tx<hash: %v, priority: %v, flags: %v>", m.HashString(false), m.priority, m.flags)
}
