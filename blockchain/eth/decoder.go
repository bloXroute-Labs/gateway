package eth

import (
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/ethereum/go-ethereum/rlp"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// TransactionBDNToBlockchain convert a BDN transaction to an Ethereum one
func TransactionBDNToBlockchain(transaction *types.BxTransaction) (*ethtypes.Transaction, error) {
	var ethTransaction ethtypes.Transaction
	err := rlp.DecodeBytes(transaction.Content(), &ethTransaction)
	return &ethTransaction, err
}
