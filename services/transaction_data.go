package services

import (
	"time"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/connections"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

// TransactionData is a struct that holds all the data needed to process a transaction
type TransactionData struct {
	Tx              *bxmessage.Tx
	Source          connections.Conn
	TxResult        TransactionResult
	SourceEndpoint  types.NodeEndpoint
	BroadcastRes    types.BroadcastResults
	StartTime       time.Time
	DelayStartTime  time.Time
	NetworkDuration int64
}

// NewTransactionData creates a new TransactionData struct
func NewTransactionData(tx *bxmessage.Tx, source connections.Conn, startTime time.Time, sourceEndpoint types.NodeEndpoint, txResult TransactionResult) *TransactionData {
	data := &TransactionData{
		Tx:              tx,
		Source:          source,
		TxResult:        txResult,
		StartTime:       startTime,
		SourceEndpoint:  sourceEndpoint,
		NetworkDuration: startTime.Sub(tx.Timestamp()).Microseconds(),
	}

	return data
}
