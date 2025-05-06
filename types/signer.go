package types

import (
	"math/big"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// LatestSignerForChainID creates a new signer for the latest network version.
func LatestSignerForChainID(chainID *big.Int) ethtypes.Signer {
	if chainID != nil && chainID.Sign() <= 0 {
		chainID = nil
	}

	return ethtypes.LatestSignerForChainID(chainID)
}
