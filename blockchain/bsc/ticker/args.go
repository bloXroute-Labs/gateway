package ticker

import (
	"errors"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

var (
	// ErrInvalidDelaysAndInterval is the error for unset delays and interval
	ErrInvalidDelaysAndInterval = errors.New("invalid delays and interval")

	// ErrInvalidDelays is the error for invalid delays
	ErrInvalidDelays = errors.New("delays must be in ascending order")

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
func (a *Args) Validate() error {
	if a.Log == nil {
		return ErrLogIsNotInit
	}

	if a.Clock == nil {
		return ErrClockIsNotInit
	}

	if len(a.Delays) == 0 && a.Interval == time.Duration(0) {
		return ErrInvalidDelaysAndInterval
	}

	var prev time.Duration
	for _, delay := range a.Delays {
		if !(prev < delay) {
			return ErrInvalidDelays
		}

		prev = delay
	}

	return nil
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
