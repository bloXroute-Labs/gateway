package services

import (
	"time"

	"github.com/bloXroute-Labs/gateway/v2/utils"
)

// RateSnapshot indicates the number of events in the last intervals
type RateSnapshot struct {
	ts5m *utils.TimeSeriesCounter
	ts1h *utils.TimeSeriesCounter
	ts1d *utils.TimeSeriesCounter
}

var emptySnapshot = RateSnapshot{
	ts5m: utils.NewTimeSeriesCounter(utils.RealClock{}, 5*time.Minute, 30*time.Second),
	ts1h: utils.NewTimeSeriesCounter(utils.RealClock{}, time.Hour, time.Minute),
	ts1d: utils.NewTimeSeriesCounter(utils.RealClock{}, 24*time.Hour, time.Hour),
}

// NewRateSnapshot instantiate a new snapshot tracker with 5m, 1h, and 1d intervals
func NewRateSnapshot(clock utils.Clock) RateSnapshot {
	return RateSnapshot{
		ts5m: utils.NewTimeSeriesCounter(clock, 5*time.Minute, 30*time.Second),
		ts1h: utils.NewTimeSeriesCounter(clock, time.Hour, time.Minute),
		ts1d: utils.NewTimeSeriesCounter(clock, 24*time.Hour, time.Hour),
	}
}

// Track increments the time series counters
func (r RateSnapshot) Track() {
	r.ts5m.Track()
	r.ts1h.Track()
	r.ts1d.Track()
}

// FiveMinute gets the number of events in the last 5 min
func (r RateSnapshot) FiveMinute() int64 {
	return int64(r.ts5m.Count())
}

// OneHour gets the number of events in the last 1 hour
func (r RateSnapshot) OneHour() int64 {
	return int64(r.ts1h.Count())
}

// OneDay gets the number of events in the last 24 hours
func (r RateSnapshot) OneDay() int64 {
	return int64(r.ts1d.Count())
}
