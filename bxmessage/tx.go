package bxmessage

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"time"

	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

const (
	nanosInSecond        = 1e9
	invalidWalletAddress = "\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00\x00"
)

// Tx is the bloxroute message struct that carries a transaction from a specific blockchain
type Tx struct {
	BroadcastHeader
	shortID   types.ShortID
	flags     types.TxFlags
	walletIDs []string
	fallback  uint16
	timestamp time.Time
	accountID [AccountIDLen]byte
	content   []byte
	quota     byte
	sender    types.Sender
}

// NewTx constructs a new transaction message, using the provided hash function on the transaction contents to determine the hash
func NewTx(hash types.SHA256Hash, content []byte, networkNum bxtypes.NetworkNum, flags types.TxFlags, accountID bxtypes.AccountID) *Tx {
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
	if flags.IsNextValidator() {
		tx.walletIDs = make([]string, 2)
	}
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

// Fallback returns the seconds they relay will broadcast the tx to all gateway if not zero
func (m *Tx) Fallback() uint16 {
	return m.fallback
}

// WalletIDs returns the wallet id for the next validator
func (m *Tx) WalletIDs() []string {
	var walletIDs []string
	for _, walletID := range m.walletIDs {
		if walletID == invalidWalletAddress {
			walletID = "0x"
		}
		walletIDs = append(walletIDs, walletID)
	}
	return walletIDs
}

// Timestamp indicates when the TxMessage was sent.
func (m *Tx) Timestamp() time.Time {
	return m.timestamp
}

// AccountID indicates the account ID the TxMessage originated from
func (m *Tx) AccountID() bxtypes.AccountID {
	if bytes.Equal(m.accountID[:], NullByteAccountID) {
		return ""
	}
	return types.NewAccountID(m.accountID[:])
}

// SetAccountID sets the account ID
func (m *Tx) SetAccountID(accountID bxtypes.AccountID) {
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

// SetFallback sets the seconds they relay will broadcast the tx to all gateway if not zero
func (m *Tx) SetFallback(f uint16) {
	m.fallback = f
}

// SetWalletID sets the wallet id for the next validator
func (m *Tx) SetWalletID(index int, id string) {
	// TODO, decide what to do in this case
	if index >= 2 {
		return
	}
	if m.walletIDs == nil {
		m.walletIDs = make([]string, 2)
	}
	m.walletIDs[index] = id
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
	if m.flags.IsNextValidator() {
		m.walletIDs = make([]string, 2)
	}

	m.flags &= ^types.TFEnterpriseSender
	m.flags &= ^types.TFEliteSender
	m.flags &= ^types.TFPaidTx
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
	return Tx{
		BroadcastHeader: m.BroadcastHeader,
		shortID:         m.shortID,
		timestamp:       m.timestamp,
		flags:           m.flags,
		accountID:       m.accountID,
		content:         nil,
		quota:           m.quota,
	}
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
	if tx.flags.IsNextValidator() {
		tx.walletIDs = make([]string, 2)
	}
	tx.ClearInternalAttributes()
	return tx
}

// Clone clones the caller Tx object and return cloned tx
func (m Tx) Clone() *Tx {
	return &Tx{
		m.BroadcastHeader,
		m.shortID,
		m.flags,
		m.walletIDs,
		m.fallback,
		m.timestamp,
		m.accountID,
		m.content,
		m.quota,
		m.sender,
	}
}

// Pack serializes a Tx into a buffer for sending
func (m Tx) Pack(protocol Protocol) ([]byte, error) {
	bufLen := m.Size(protocol)
	buf := make([]byte, bufLen)
	m.BroadcastHeader.Pack(&buf, TxType, protocol)
	offset := BroadcastHeaderOffset

	binary.LittleEndian.PutUint32(buf[offset:], uint32(m.shortID))
	offset += types.ShortIDLen
	flags := m.flags
	binary.LittleEndian.PutUint16(buf[offset:], uint16(flags))

	offset += types.TxFlagsLen
	timestamp := m.timestamp.UnixNano() >> 10
	binary.LittleEndian.PutUint32(buf[offset:], uint32(timestamp))
	offset += ShortTimestampLen
	if m.flags.IsNextValidator() {
		binary.LittleEndian.PutUint16(buf[offset:], m.fallback)
		offset += types.UInt16Len
		for _, walletID := range m.walletIDs {
			copy(buf[offset:offset+types.WalletIDLen], walletID)
			offset += types.WalletIDLen
		}
	}
	copy(buf[offset:], m.accountID[:])
	offset += AccountIDLen

	// should include sender and nonce only if content is included
	if len(m.content) > 0 {
		copy(buf[offset:], m.content)
		offset += len(m.content)
		copy(buf[offset:], m.sender[:])
		offset += len(m.sender)
	}

	if err := checkBuffEnd(&buf, offset); err != nil {
		return nil, err
	}

	return buf, nil
}

// Unpack deserializes a Tx from a buffer
func (m *Tx) Unpack(buf []byte, protocol Protocol) error {
	if err := m.BroadcastHeader.Unpack(buf, protocol); err != nil {
		return err
	}
	offset := BroadcastHeaderOffset
	if err := checkBufSize(&buf, offset, types.ShortIDLen+types.TxFlagsLen); err != nil {
		return err
	}
	m.shortID = types.ShortID(binary.LittleEndian.Uint32(buf[offset:]))
	offset += types.ShortIDLen
	m.flags = types.TxFlags(binary.LittleEndian.Uint16(buf[offset:]))
	offset += types.TxFlagsLen

	if err := checkBufSize(&buf, offset, ShortTimestampLen); err != nil {
		return err
	}
	timestamp := binary.LittleEndian.Uint32(buf[offset:])
	offset += ShortTimestampLen
	result := decodeTimestamp(timestamp)
	m.timestamp = time.Unix(0, result)

	if m.flags.IsNextValidator() {
		m.walletIDs = make([]string, 2)
		if err := checkBufSize(&buf, offset, types.UInt16Len+types.WalletIDLen+types.WalletIDLen); err != nil {
			return err
		}
		m.fallback = binary.LittleEndian.Uint16(buf[offset:])
		offset += types.UInt16Len
		for i := 0; i < 2; i++ {
			m.SetWalletID(i, string(buf[offset:offset+types.WalletIDLen]))
			offset += types.WalletIDLen
		}
	}

	switch {
	case protocol < 22:
		// do nothing. accountID added in protocol 22
	default:
		if err := checkBufSize(&buf, offset, AccountIDLen); err != nil {
			return err
		}
		copy(m.accountID[:], buf[offset:])
		offset += AccountIDLen
	}

	if len(buf)-offset-ControlByteLen == 0 {
		m.content = buf[offset : len(buf)-ControlByteLen]
	} else {
		m.content = buf[offset : len(buf)-SenderLen-ControlByteLen]
		offset += len(m.content)
		copy(m.sender[:], buf[offset:])
	}

	return nil
}

func decodeTimestamp(timestamp uint32) int64 {
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

	return result
}

// Size return size of tx
func (m *Tx) Size(protocol Protocol) uint32 {
	contentSize := uint32(len(m.content))
	if contentSize > 0 {
		contentSize += SenderLen
	}

	if m.flags.IsNextValidator() {
		contentSize += types.WalletIDLen*2 + types.UInt16Len
	}
	return m.BroadcastHeader.Size() + types.ShortIDLen + types.TxFlagsLen + ShortTimestampLen + AccountIDLen + contentSize
}

// String serializes a Tx as a string for pretty printing
func (m Tx) String() string {
	return fmt.Sprintf("Tx<hash: %v, priority: %v, flags: %v>", m.HashString(false), m.priority, m.flags)
}
