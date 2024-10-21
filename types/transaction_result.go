package types

// TransactionResult is returned after the transaction service processes a new tx message, deciding whether to process it
type TransactionResult struct {
	NewTx            bool
	NewContent       bool
	NewSID           bool
	Reprocess        bool
	FailedValidation bool
	Transaction      *BxTransaction
	AssignedShortID  ShortID
	DebugData        interface{}
	AlreadySeen      bool
	Nonce            uint64
}
