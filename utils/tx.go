package utils

import (
	"fmt"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// ParseRawTransaction parses a raw Ethereum transaction
func ParseRawTransaction(txBytes []byte) (*ethtypes.Transaction, error) {
	var ethTx ethtypes.Transaction
	if err := ethTx.UnmarshalBinary(txBytes); err != nil {
		// If UnmarshalBinary failed, we will try RLP in case user made mistake
		e := rlp.DecodeBytes(txBytes, &ethTx)
		if e != nil {
			return nil, fmt.Errorf("could not decode Ethereum transaction: %v", err)
		}
		log.Warnf("Ethereum transaction was in RLP format instead of binary," +
			" transaction has been processed anyway, but it'd be best to use the Ethereum binary standard encoding")
	}
	return &ethTx, nil
}
