package types

import (
	"sync"
	"time"

	"google.golang.org/protobuf/types/known/timestamppb"

	pbbase "github.com/bloXroute-Labs/gateway/v2/protobuf"
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
	rawTx      string
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
	bt.m.RLock()
	defer bt.m.RUnlock()

	return bt.hash
}

// Flags returns the transaction flags for routing
func (bt *BxTransaction) Flags() TxFlags {
	bt.m.RLock()
	defer bt.m.RUnlock()

	return bt.flags
}

// AddFlags adds the provided flag to the transaction flag set
func (bt *BxTransaction) AddFlags(flags TxFlags) {
	bt.m.Lock()
	defer bt.m.Unlock()

	bt.flags |= flags
}

// RemoveFlags sets off txFlag
func (bt *BxTransaction) RemoveFlags(flags TxFlags) {
	bt.m.Lock()
	defer bt.m.Unlock()

	bt.flags &^= flags
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
	bt.m.RLock()
	defer bt.m.RUnlock()

	return bt.shortIDs
}

// NetworkNum provides the network number of the transaction
func (bt *BxTransaction) NetworkNum() NetworkNum {
	bt.m.RLock()
	defer bt.m.RUnlock()

	return bt.networkNum
}

// Sender returns the transaction sender
func (bt *BxTransaction) Sender() Sender {
	bt.m.RLock()
	defer bt.m.RUnlock()

	return bt.sender
}

// AddTime returns the time the transaction was added
func (bt *BxTransaction) AddTime() time.Time {
	bt.m.RLock()
	defer bt.m.RUnlock()

	return bt.addTime
}

// SetAddTime sets the time the transaction was added. Should be called with Lock()
func (bt *BxTransaction) SetAddTime(t time.Time) {
	bt.m.Lock()
	defer bt.m.Unlock()

	bt.addTime = t
}

// GetRawTx returns preconfigured raw tx string, normally the raw tx is calculated base on tx content
func (bt *BxTransaction) GetRawTx() string {
	bt.m.RLock()
	defer bt.m.RUnlock()

	return bt.rawTx
}

// SetRawTx sets the raw_tx, this is used by cloud-api with type 3 tx due to missing sidecar in txContent
func (bt *BxTransaction) SetRawTx(rawTx string) {
	bt.m.Lock()
	defer bt.m.Unlock()

	bt.rawTx = rawTx
}

// AddShortID adds an assigned shortID, indicating whether it was actually new. Should be called with Lock()
func (bt *BxTransaction) AddShortID(shortID ShortID) bool {
	bt.m.Lock()
	defer bt.m.Unlock()

	return bt.addShortID(shortID)
}

func (bt *BxTransaction) addShortID(shortID ShortID) bool {
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
	bt.m.Lock()
	defer bt.m.Unlock()

	return bt.setContent(content)
}

func (bt *BxTransaction) setContent(content TxContent) bool {
	if len(bt.content) == 0 && len(content) > 0 {
		bt.content = make(TxContent, len(content))
		copy(bt.content, content)
		return true
	}

	return false
}

// BlockchainTransaction parses and returns a transaction for the given network number's spec
func (bt *BxTransaction) BlockchainTransaction(sender Sender) (BlockchainTransaction, error) {
	bt.m.RLock()
	defer bt.m.RUnlock()

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
	bt.m.RLock()
	defer bt.m.RUnlock()

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

// CloneWithEmptyContent returns a new transaction with the same data but no content
func (bt *BxTransaction) CloneWithEmptyContent() *BxTransaction {
	bt.m.RLock()
	defer bt.m.RUnlock()

	clone := NewBxTransaction(bt.hash, bt.networkNum, bt.flags, bt.addTime)
	clone.content = make(TxContent, 0)
	clone.shortIDs = make(ShortIDList, len(bt.shortIDs))
	copy(clone.shortIDs, bt.shortIDs)
	copy(clone.sender[:], bt.sender[:])

	return clone
}

// Update updates the transaction with new content, flags, shortID, and sender. It returns the assigned shortID.
func (bt *BxTransaction) Update(result *TransactionResult, flags TxFlags, shortID ShortID, content TxContent, sender Sender, nextID func() ShortID) ShortID {
	bt.m.Lock()
	defer bt.m.Unlock()

	if !bt.flags.IsPaid() && flags.IsPaid() {
		result.Reprocess = true
		bt.flags |= TFPaidTx
	}
	if !bt.flags.ShouldDeliverToNode() && flags.ShouldDeliverToNode() {
		result.Reprocess = true
		bt.flags |= TFDeliverToNode
	}

	// if shortID was not provided, assign shortID (if we are running as assigner)
	// note that assigner.Next() provides ShortIDEmpty if we are not assigning
	// also, shortID is not assigned if transaction is validators_only
	// if we assigned shortID, result.AssignedShortID hold non ShortIDEmpty value
	if result.NewTx && shortID == ShortIDEmpty && !bt.flags.IsValidatorsOnly() && !bt.flags.IsNextValidator() {
		shortID = nextID()
		result.AssignedShortID = shortID
	}

	result.NewSID = bt.addShortID(shortID)
	result.NewContent = bt.setContent(content)

	// set sender only if it has new content in order to avoid false sender when the shortID is not new
	if result.NewContent {
		copy(bt.sender[:], sender[:])
	}

	result.Transaction = bt

	return shortID
}
