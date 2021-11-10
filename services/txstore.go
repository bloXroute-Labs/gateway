package services

import (
	pbbase "github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/protobuf"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/types"
	"time"
)

const timeToAvoidReEntry = 30 * time.Minute

// ReEntryProtection - protect against hash re-entrance
const ReEntryProtection = true

// NoReEntryProtection - no re-entrance protection needed
const NoReEntryProtection = false

// TxStore is the service interface for transaction storage and processing
type TxStore interface {
	Start() error
	Stop()

	Add(hash types.SHA256Hash, content types.TxContent, shortID types.ShortID, network types.NetworkNum,
		validate bool, flags types.TxFlags, timestamp time.Time, networkChainID int64) TransactionResult
	Get(hash types.SHA256Hash) (*types.BxTransaction, bool)
	HasContent(hash types.SHA256Hash) bool

	RemoveShortIDs(*types.ShortIDList, bool, string)
	RemoveHashes(*types.SHA256HashList, bool, string)
	GetTxByShortID(types.ShortID) (*types.BxTransaction, error)

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
	FailedValidation bool
	ReuseSenderNonce bool
	Transaction      *types.BxTransaction
	AssignedShortID  types.ShortID
	DebugData        interface{}
}
