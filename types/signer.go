package types

import (
	"fmt"
	"math/big"

	ethtypes "github.com/ethereum/go-ethereum/core/types"
)

// NewPragueSigner creates a new signer for the Prague network.
func NewPragueSigner(chainID *big.Int) (ethtypes.Signer, error) {
	if chainID == nil || chainID.Sign() <= 0 {
		return nil, fmt.Errorf("invalid chainID %v", chainID)
	}

	return ethtypes.NewPragueSigner(chainID), nil
}
