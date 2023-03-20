package polygon

import (
	ethtypes "github.com/ethereum/go-ethereum/core/types"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// ValidatorInfoManager responsible for getting next validators' info.
type ValidatorInfoManager interface {
	Run() error
	IsRunning() bool
	FutureValidators(header *ethtypes.Header) [2]*types.FutureValidatorInfo
}
