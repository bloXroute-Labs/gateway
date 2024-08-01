package blockchain

import "github.com/bloXroute-Labs/gateway/v2/types"

// FutureValidatorWindowSize represents default length of types.FutureValidatorInfo
const FutureValidatorWindowSize = 2

// DefaultValidatorInfo creates default types.FutureValidatorInfo for next FutureValidatorWindowSize blocks
func DefaultValidatorInfo(blockHeight uint64) []*types.FutureValidatorInfo {
	validatorInfo := make([]*types.FutureValidatorInfo, 0, FutureValidatorWindowSize)

	for i := 0; i < FutureValidatorWindowSize; i++ {
		validatorInfo = append(validatorInfo, &types.FutureValidatorInfo{
			BlockHeight: blockHeight + uint64(i+1),
			WalletID:    "nil",
			Accessible:  true,
		})
	}

	return validatorInfo
}
