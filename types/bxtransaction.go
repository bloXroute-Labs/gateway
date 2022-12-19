package types

import (
	"sync"
	"time"

	pbbase "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"google.golang.org/protobuf/types/known/timestamppb"
)

// TxContent represents a byte array containing full transaction bytes
type TxContent []byte

// BxTransaction represents a single bloXroute transaction
type BxTransaction struct {
	m          sync.RWMutex
	hash       SHA256Hash
	content    TxContent
	shortIDs   ShortIDList
	addTime    time.Time
	flags      TxFlags
	networkNum NetworkNum
	sender     Sender
}

// NewBxTransaction creates a new transaction to be stored. Transactions are not expected to be initialized with content or shortIDs; they should be added via AddShortID and SetContent.
func NewBxTransaction(hash SHA256Hash, networkNum NetworkNum, flags TxFlags, timestamp time.Time) *BxTransaction {
	return &BxTransaction{
		hash:       hash,
		addTime:    timestamp,
		networkNum: networkNum,
		flags:      flags,
	}
}

// NewRawBxTransaction creates a new transaction directly from the hash and content. In general, NewRawBxTransaction should not be added directly to TxStore, and should only be validated further before storing.
func NewRawBxTransaction(hash SHA256Hash, content TxContent) *BxTransaction {
	return &BxTransaction{
		hash:    hash,
		content: content,
	}
}

// Hash returns the transaction hash
func (bt *BxTransaction) Hash() SHA256Hash {
	return bt.hash
}

// Flags returns the transaction flags for routing
func (bt *BxTransaction) Flags() TxFlags {
	return bt.flags
}

// AddFlags adds the provided flag to the transaction flag set
func (bt *BxTransaction) AddFlags(flags TxFlags) {
	bt.flags |= flags
}

// SetFlags sets the message flags
func (bt *BxTransaction) SetFlags(flags TxFlags) {
	bt.flags = flags
}

// RemoveFlags sets off txFlag
func (bt *BxTransaction) RemoveFlags(flags TxFlags) {
	bt.SetFlags(bt.Flags() &^ flags)
}

// Content returns the transaction contents (usually the blockchain transaction bytes)
func (bt *BxTransaction) Content() TxContent {
	bt.m.RLock()
	defer bt.m.RUnlock()

	return bt.content
}

// HasContent indicates if transaction has content bytes
func (bt *BxTransaction) HasContent() bool {
	bt.m.RLock()
	defer bt.m.RUnlock()

	return len(bt.content) > 0
}

// ShortIDs returns the (possibly multiple) short IDs assigned to a transaction
func (bt *BxTransaction) ShortIDs() ShortIDList {
	return bt.shortIDs
}

// NetworkNum provides the network number of the transaction
func (bt *BxTransaction) NetworkNum() NetworkNum {
	return bt.networkNum
}

// Sender returns the transaction sender
func (bt *BxTransaction) Sender() Sender {
	return bt.sender
}

// SetSender returns the transaction sender
func (bt *BxTransaction) SetSender(sender Sender) {
	copy(bt.sender[:], sender[:])
}

// AddTime returns the time the transaction was added
func (bt *BxTransaction) AddTime() time.Time {
	return bt.addTime
}

// SetAddTime sets the time the transaction was added. Should be called with Lock()
func (bt *BxTransaction) SetAddTime(t time.Time) {
	bt.addTime = t
}

// Lock locks the transaction so changes can be made
func (bt *BxTransaction) Lock() {
	bt.m.Lock()
}

// Unlock unlocks the transaction
func (bt *BxTransaction) Unlock() {
	bt.m.Unlock()
}

// AddShortID adds an assigned shortID, indicating whether it was actually new. Should be called with Lock()
func (bt *BxTransaction) AddShortID(shortID ShortID) bool {
	if shortID == ShortIDEmpty {
		return false
	}

	for _, existingShortID := range bt.shortIDs {
		if shortID == existingShortID {
			return false
		}
	}
	bt.shortIDs = append(bt.shortIDs, shortID)
	return true
}

// SetContent sets the blockchain transaction contents only if the contents are new and has never been set before. SetContent returns whether the content was updated. Should be called with Lock()
func (bt *BxTransaction) SetContent(content TxContent) bool {
	if len(bt.content) == 0 && len(content) > 0 {
		bt.content = make(TxContent, len(content))
		copy(bt.content, content)
		return true
	}
	return false
}

// BlockchainTransaction parses and returns a transaction for the given network number's spec
func (bt *BxTransaction) BlockchainTransaction(sender Sender) (BlockchainTransaction, error) {
	return bt.parseTransaction(sender)
}

func (bt *BxTransaction) parseTransaction(sender Sender) (BlockchainTransaction, error) {
	// TODO - add support for additional networks

	// for now, since we only support Ethereum based transaction
	// we are not checking but parsing as if the transaction is Ethereum based.
	return ethTransactionFromBytes(bt.hash, bt.content, sender)
	/*
		switch bt.networkNum {
		case EthereumNetworkNum:
			return NewEthTransaction(bt.hash, bt.content)
		default:
			return nil, fmt.Errorf("no message converter found for network num %v", bt.networkNum)
		}
	*/
}

// Protobuf formats transaction info as a protobuf response struct
func (bt *BxTransaction) Protobuf() *pbbase.BxTransaction {
	shortIDs := make([]uint64, 0)
	for _, shortID := range bt.shortIDs {
		shortIDs = append(shortIDs, uint64(shortID))
	}
	ts := timestamppb.New(bt.addTime)

	return &pbbase.BxTransaction{
		Hash:     bt.hash.Format(false),
		ShortIds: shortIDs,
		AddTime:  ts,
	}
}
