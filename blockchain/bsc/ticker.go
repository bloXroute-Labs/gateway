package bsc

import (
	"time"

	"github.com/bloXroute-Labs/gateway/v2/services"
)

// Ticker is the interface for the different tickers
type Ticker interface {
	services.Runner

	// Alert returns a channel that is fired when the ticker ticks
	Alert() <-chan struct{}

	// Reset resets the ticker to the delay from the timestamp
	Reset(ts time.Time) bool
}
