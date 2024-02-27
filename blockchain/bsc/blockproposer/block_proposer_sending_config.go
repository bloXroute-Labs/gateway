package blockproposer

import (
	"time"

	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// SendingConfig is the configuration for block proposer sending.
type SendingConfig struct {
	// Enabled is the flag to enable or disable the block proposer sending.
	Enabled bool

	// RegularBlockSendDelayInitial is the initial delay for sending regular blocks.
	RegularBlockSendDelayInitial time.Duration

	// RegularBlockSendDelaySecond is the second delay for sending regular blocks.
	RegularBlockSendDelaySecond time.Duration

	// RegularBlockSendDelayInterval is the interval delay for sending regular blocks.
	RegularBlockSendDelayInterval time.Duration

	// HighLoadBlockSendDelayInitial is the initial delay for sending high load blocks.
	HighLoadBlockSendDelayInitial time.Duration

	// HighLoadBlockSendDelaySecond is the second delay for sending high load blocks.
	HighLoadBlockSendDelaySecond time.Duration

	// HighLoadBlockSendDelayInterval is the interval delay for sending high load blocks.
	HighLoadBlockSendDelayInterval time.Duration

	// HighLoadTxNumThreshold is the threshold for high load blocks.
	HighLoadTxNumThreshold int
}

// Validate validates the block proposer sending configuration and sets defaults.
func (b *SendingConfig) Validate(log *logger.Entry) {
	if b.RegularBlockSendDelayInitial == 0 {
		log.Warn("regular block send delay initial is not initialized, using default")
		b.RegularBlockSendDelayInitial = time.Duration(utils.DefaultRegularBlockSendDelayInitialMS) * time.Millisecond
	}

	if b.RegularBlockSendDelaySecond == 0 || b.RegularBlockSendDelaySecond < b.RegularBlockSendDelayInitial {
		log.Warn(
			"regular block send delay " +
				"second(" + b.RegularBlockSendDelaySecond.String() + ") " +
				"is not initialized or is lower than " +
				"initial(" + b.RegularBlockSendDelayInitial.String() + "), " +
				"using default",
		)
		b.RegularBlockSendDelaySecond = time.Duration(utils.DefaultRegularBlockSendDelaySecondMS) * time.Millisecond
	}

	if b.RegularBlockSendDelayInterval == 0 {
		log.Warn("regular block send delay interval is not initialized, using default")
		b.RegularBlockSendDelayInterval = time.Duration(utils.DefaultRegularBlockSendDelayIntervalMS) * time.Millisecond
	}

	if b.HighLoadBlockSendDelayInitial == 0 {
		log.Warn("high load block send delay initial is not initialized, using default")
		b.HighLoadBlockSendDelayInitial = time.Duration(utils.DefaultHighLoadBlockSendDelayInitialMS) * time.Millisecond
	}

	if b.HighLoadBlockSendDelaySecond == 0 || b.HighLoadBlockSendDelaySecond < b.HighLoadBlockSendDelayInitial {
		log.Warn(
			"high load block send delay " +
				"second(" + b.HighLoadBlockSendDelaySecond.String() + ") " +
				"is not initialized or is lower than " +
				"initial(" + b.HighLoadBlockSendDelayInitial.String() + "), " +
				"using default",
		)
		b.HighLoadBlockSendDelaySecond = time.Duration(utils.DefaultHighLoadBlockSendDelaySecondMS) * time.Millisecond
	}

	if b.HighLoadBlockSendDelayInterval == 0 {
		log.Warn("high load block send delay interval is not initialized, using default")
		b.HighLoadBlockSendDelayInterval = time.Duration(utils.DefaultHighLoadBlockSendDelayIntervalMS) * time.Millisecond
	}

	if b.HighLoadTxNumThreshold == 0 {
		log.Warn("high load tx num threshold is not initialized, using default")
		b.HighLoadTxNumThreshold = utils.DefaultHighLoadTxNumThreshold
	}
}
