package bxmessage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"github.com/bloXroute-Labs/gateway/types"
	"math"
	"time"
)

const nanosInSecond = 1e9

/*
   as of protocol version 25 (FullTxTimeStampProtocol):
      The timestamp field is in nanoseconds and not in seconds
      The timestamp field records the send time for this TX message. If the Message received by B from A it records
		the time A sent the message. If B is sending the same message to C, B should update the timestamp field before
		sending to C. The timestamp field is now use to analyse the network latency.
*/

// Tx is the bloxroute message struct that carries a transaction from a specific blockchain
type Tx struct {
	BroadcastHeader
	shortID   types.ShortID
	flags     types.TxFlags
	timestamp time.Time
	accountID [AccountIDLen]byte
	content   []byte
	quota     byte
	sender    types.Sender
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
	tx.SetTimestamp(clock.Now())
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

// Timestamp indicates when the TxMessage was sent.
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

// SetSender sets the sender
func (m *Tx) SetSender(sender types.Sender) {
	copy(m.sender[:], sender[:])
}

// ClearProtectedAttributes unsets and validates fields that are restricted for gateways
func (m *Tx) ClearProtectedAttributes() {
	// Don't clear m.timestamp - GW should set it for relay to analyze
	m.shortID = types.ShortIDEmpty
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

// Sender return tx sender
func (m *Tx) Sender() types.Sender {
	return m.sender
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
		sender:          m.sender,
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

	switch {
	case protocol < 21:
		timestamp := float64(m.timestamp.UnixNano()) / nanosInSecond
		binary.LittleEndian.PutUint32(buf[offset:], uint32(timestamp+1.0))
		offset += types.UInt32Len
	case protocol < FullTxTimeStampProtocol:
		timestamp := float64(m.timestamp.UnixNano()) / nanosInSecond
		binary.LittleEndian.PutUint64(buf[offset:], math.Float64bits(timestamp))
		offset += TimestampLen
	default:
		timestamp := m.timestamp.UnixNano() >> 10
		binary.LittleEndian.PutUint32(buf[offset:], uint32(timestamp))
		offset += ShortTimestampLen
	}
	switch {
	case protocol < 22:
		// do nothing. accountID added in protocol 22
	default:
		copy(buf[offset:], m.accountID[:])
		offset += AccountIDLen
	}

	// should include sender and nonce only if content is included
	if len(m.content) > 0 {
		copy(buf[offset:], m.content)
		offset += len(m.content)

		switch {
		case protocol < SenderProtocol:
			// do nothing. sender nonce added in protocol 25
		default:
			copy(buf[offset:], m.sender[:])
		}
	}

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
	switch {
	case protocol < 21:
		timestamp := float64(binary.LittleEndian.Uint32(buf[offset:]))
		offset += types.UInt32Len
		nanoseconds := int64(timestamp) * int64(nanosInSecond)
		m.timestamp = time.Unix(0, nanoseconds)
	case protocol < FullTxTimeStampProtocol:
		tmp := binary.LittleEndian.Uint64(buf[offset:])
		timestamp := math.Float64frombits(tmp)
		offset += TimestampLen
		nanoseconds := int64(timestamp) * int64(nanosInSecond)
		m.timestamp = time.Unix(0, nanoseconds)
	default:
		timestamp := binary.LittleEndian.Uint32(buf[offset:])
		offset += ShortTimestampLen
		toMerge := uint64(timestamp) << 10

		// the leading 22 bits were wiped out during pack(), the trailing 10 bits (nanoseconds) were dropped out as well
		// 0xFFFFFC0000000000 is hex for 0b1111111111111111111111000000000000000000000000000000000000000000
		// leading 22 1s, following by 42 0s

		now := clock.Now().UnixNano()
		nanoseconds := toMerge | uint64(now)&0xFFFFFC0000000000
		result := int64(nanoseconds)

		// There are two edge cases we need to check
		// 1. overflow, the Tx message only track 32 bit, if the sender time is 1234+FFFF..FF(32bit)+(10bits offset, not important)
		//    the receiver time is 1235+00000(32bit)+(10 bits offset,not important), after the merge we will end up with 1235+FFF..FFF+..
		//    the overflow will result in roughly 1 hour longer than expected.
		//    To solve this, we need to compare the current time with the unpacked timestamp, if the gap is huge ( more than an hour )
		//   then overflow happened, and we need to subtract the timestamp by 0x40000000000 nanoseconds
		if result-now >= time.Hour.Nanoseconds() {
			// overflow
			result = result - 0x40000000000
		}

		// 2. underflow, 1234+0000..000+10bits for the sender, and 1233+FFFFF..FF+10bits due to the clock time difference of the machine.
		//    after the merge this can become 1233+000000..00+10bit which is far older than expected, so we need to add 0x40000000000 to the
		//    timestamp
		if now-result >= time.Hour.Nanoseconds() {
			// underflow
			result = result + 0x40000000000
		}

		m.timestamp = time.Unix(0, result)
	}
	switch {
	case protocol < 22:
		// do nothing. accountID added in protocol 22
	default:
		copy(m.accountID[:], buf[offset:])
		offset += AccountIDLen
	}

	if len(buf)-offset-ControlByteLen == 0 || protocol < SenderProtocol {
		m.content = buf[offset : len(buf)-ControlByteLen]
	} else {
		m.content = buf[offset : len(buf)-SenderLen-ControlByteLen]
		offset += len(m.content)
		copy(m.sender[:], buf[offset:])
	}

	return nil
}

func (m *Tx) size(protocol Protocol) uint32 {
	switch {
	case protocol < 19:
		return 0
	case protocol < 21:
		return m.BroadcastHeader.Size() + types.ShortIDLen + types.TxFlagsLen + ShortTimestampLen + uint32(len(m.content))
	case protocol < 22:
		return m.BroadcastHeader.Size() + types.ShortIDLen + types.TxFlagsLen + TimestampLen + uint32(len(m.content))
	case protocol < SenderProtocol:
		return m.BroadcastHeader.Size() + types.ShortIDLen + types.TxFlagsLen + TimestampLen + AccountIDLen + uint32(len(m.content))
	}

	contentSize := uint32(len(m.content))
	if contentSize > 0 {
		contentSize += SenderLen
	}

	return m.BroadcastHeader.Size() + types.ShortIDLen + types.TxFlagsLen + ShortTimestampLen + AccountIDLen + contentSize
}

// String serializes a Tx as a string for pretty printing
func (m Tx) String() string {
	return fmt.Sprintf("Tx<hash: %v, priority: %v, flags: %v>", m.HashString(false), m.priority, m.flags)
}
