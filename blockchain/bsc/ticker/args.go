package ticker

import (
	"time"

	"github.com/pkg/errors"

	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

var (
	// ErrInvalidInterval is the error for invalid interval
	ErrInvalidInterval = errors.New("invalid interval")

	// ErrClockIsNotInit is the error for clock is not initialized
	ErrClockIsNotInit = errors.New("clock is not initialized")

	// ErrLogIsNotInit is the error for log is not initialized
	ErrLogIsNotInit = errors.New("log is not initialized")
)

// Args are the arguments for the interval
// - Clock is the clock that is used for the interval
// - Delays are the delays between ticks each delay is the delay from timestamp passed to Reset
// - Interval is the interval between ticks if no delays are set
type Args struct {
	Clock    utils.Clock
	Delays   []time.Duration
	Interval time.Duration
	Log      *logger.Entry
}

// Validate validates the interval
// - if no delays are set, the Ticker must be set
// - if Delays are set, the Ticker can be set or not
// - if Delays are set, they must be in ascending order
func (a *Args) Validate() (err error) {
	if a.Log == nil {
		err = errors.WithMessage(ErrLogIsNotInit, "log must be initialized")
		return
	}

	if a.Clock == nil {
		err = errors.WithMessage(ErrClockIsNotInit, "clock must be initialized")
		return
	}

	if len(a.Delays) == 0 && a.Interval == time.Duration(0) {
		return errors.WithMessage(ErrInvalidInterval, "delays not set and interval not set")
	}

	var prev time.Duration
	for _, delay := range a.Delays {
		if !(prev < delay) {
			err = errors.WithMessage(ErrInvalidInterval, "delays must be in ascending order")
			return
		}

		prev = delay
	}

	return
}

// getDelays iterate over the delays starting from the last one and calculate the exact time to wait
// n - (n-1) the first delay is setting as is to not fall into negative values
// so the last iteration will be just the first delay
func (a *Args) getDelays() []time.Duration {
	delays := make([]time.Duration, len(a.Delays))

	for i := len(a.Delays) - 1; i >= 0; i-- {
		if i == 0 {
			delays[i] = a.Delays[i]
			continue
		}

		delays[i] = a.Delays[i] - a.Delays[i-1]
	}

	return delays
}
