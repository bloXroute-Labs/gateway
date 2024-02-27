package blockproposer

import (
	"github.com/pkg/errors"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/bsc/caller"
	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

var (
	errClockNotInitialized   = errors.New("clock is not initialized")
	errTxStoreNotInitialized = errors.New("tx store is not initialized")
	errLogNotInitialized     = errors.New("log is not initialized")
	errCallerManager         = errors.New("caller manager is not initialized")
	errTickerNotInitialized  = errors.New("ticker is not initialized")
	errSendingDisabled       = errors.New("sending is not enabled")
)

// Config is the configuration for BlockProposer.
type Config struct {
	// Clock is a pointer to the Clock instance managed from the outside.
	Clock utils.Clock

	// TxStore is a pointer to the TxStore instance managed from the outside.
	TxStore *services.TxStore

	// Log is the logger instance.
	Log *logger.Entry

	// CallerManager is the caller manager instance.
	CallerManager caller.Manager

	// SendingInfo is the sending info with delays and thresholds.
	SendingInfo *SendingConfig

	// RegularTicker is the ticker for the block proposer with regular load.
	RegularTicker bsc.Ticker

	// HighLoadTicker is the ticker for the block proposer with high load.
	HighLoadTicker bsc.Ticker

	// BlocksToCache is the number of blocks stored in memory for BlockProposer.ProposedBlockStats requests.
	BlocksToCache int64
}

// Validate validates the BlockProposerConfig.
func (b *Config) Validate() error {
	if b.Clock == nil {
		return errClockNotInitialized
	}

	if b.TxStore == nil || *b.TxStore == nil {
		return errTxStoreNotInitialized
	}

	if b.Log == nil {
		return errLogNotInitialized
	}

	if b.CallerManager == nil {
		return errCallerManager
	}

	if b.SendingInfo == nil {
		b.SendingInfo = new(SendingConfig)
	}

	if !b.SendingInfo.Enabled {
		return errors.WithMessage(errSendingDisabled, "sending must be enabled to use block proposer")
	}

	b.SendingInfo.Validate(b.Log)

	if b.RegularTicker == nil {
		return errors.WithMessage(errTickerNotInitialized, "regular ticker is not initialized")
	}

	if b.HighLoadTicker == nil {
		return errors.WithMessage(errTickerNotInitialized, "regular ticker is not initialized")
	}

	return nil
}
