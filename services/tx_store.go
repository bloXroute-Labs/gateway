package services

import (
	"time"

	pbbase "github.com/bloXroute-Labs/gateway/v2/protobuf"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// ReEntryProtectionFlags protect against hash re-entrance
type ReEntryProtectionFlags uint8

// flag constant values
const (
	NoReEntryProtection ReEntryProtectionFlags = iota
	ShortReEntryProtection
	FullReEntryProtection
)

// ShortReEntryProtectionDuration defines the short duration for TxStore reentry protection
const ShortReEntryProtectionDuration = 30 * time.Minute

// TxStore is the service interface for transaction storage and processing
type TxStore interface {
	Start() error
	Stop()

	Add(hash types.SHA256Hash, content types.TxContent, shortID types.ShortID, network types.NetworkNum,
		validate bool, flags types.TxFlags, timestamp time.Time, networkChainID int64, sender types.Sender) TransactionResult
	Get(hash types.SHA256Hash) (*types.BxTransaction, bool)
	Known(hash types.SHA256Hash) bool
	HasContent(hash types.SHA256Hash) bool

	RemoveShortIDs(*types.ShortIDList, ReEntryProtectionFlags, string)
	RemoveHashes(*types.SHA256HashList, ReEntryProtectionFlags, string) types.ShortIDList
	GetTxByShortID(types.ShortID, bool) (*types.BxTransaction, error)

	GetTxByKzgCommitment(string) (*types.BxTransaction, error)

	Clear()

	Iter() (iter <-chan *types.BxTransaction)
	Count() int
	Summarize() *pbbase.TxStoreReply
	CleanNow()
}

// TransactionResult is returned after the transaction service processes a new tx message, deciding whether to process it
type TransactionResult struct {
	NewTx            bool
	NewContent       bool
	NewSID           bool
	Reprocess        bool
	FailedValidation bool
	Transaction      *types.BxTransaction
	AssignedShortID  types.ShortID
	DebugData        interface{}
	AlreadySeen      bool
	Nonce            uint64
}
