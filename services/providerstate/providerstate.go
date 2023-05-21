package providerstate

import (
	"math/big"

	"github.com/bloXroute-Labs/gateway/v2/services"
)

// ProviderGWState is the interface for the delayer state
type ProviderGWState interface {
	services.Runner

	DelayedTransactions() <-chan *services.TransactionData
	StoreTx(*services.TransactionData) bool // returns true if should send to validator
	OnBlockReceived(*big.Int)
}
