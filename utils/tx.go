package utils

import (
	"fmt"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"golang.org/x/crypto/sha3"
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

// ParseStringTransaction is a helper function used by blxr_tx and blxr_batch_tx for processing a rawTransaction
// here it is used with the blxr_submit_bundle method on gateway
func ParseStringTransaction(tx string) (*ethtypes.Transaction, error) {
	txBytes, err := types.DecodeHex(tx)
	if err != nil {
		return nil, fmt.Errorf("invalid hex string: %w", err)
	}

	var ethTx ethtypes.Transaction
	err = ethTx.UnmarshalBinary(txBytes)

	if err != nil {
		// If UnmarshalBinary failed, we will try RLP in case user made mistake
		e := rlp.DecodeBytes(txBytes, &ethTx)
		if e != nil {
			return nil, fmt.Errorf("could not decode Ethereum transaction: %v", err)
		}
		log.Warnf("Ethereum transaction was in RLP format instead of binary, transaction has been processed anyway, but it'd be best to use the Ethereum binary standard encoding. err: %v", err)
	}
	return &ethTx, nil
}

// CalculateBundleHash calculate bundle hash from bundle transactions
func CalculateBundleHash(bundleTransactions []string) (string, error) {
	bundleHash := sha3.NewLegacyKeccak256()
	for _, tx := range bundleTransactions {
		transaction, err := ParseStringTransaction(tx)
		if err != nil {
			return "", err
		}
		bundleHash.Write(transaction.Hash().Bytes())
	}
	return "0x" + common.Bytes2Hex(bundleHash.Sum(nil)), nil
}
