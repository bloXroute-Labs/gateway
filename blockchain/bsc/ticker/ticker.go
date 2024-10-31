package ticker

import (
	"context"
	"fmt"
	"strconv"
	"time"

	"go.uber.org/atomic"

	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const (
	// defaultOutChBuffer is the default buffer size of the outCh
	defaultOutChBuffer = 10
)

// Ticker is the interval ticker
// - clock is the clock that is used for the interval
// - timer is the timer that is used for the interval
// - delays are the delays between ticks each delay is the delay from previous alert
// - interval is the interval between ticks if no delays are set or the last delay is reached
// - current is the current index of the delays
// - outCh is the channel to signal the interval
type Ticker struct {
	log *logger.Entry

	clock utils.Clock
	timer utils.Timer

	// exact duration to wait before sending the next block (in milliseconds)
	// from the previous alert
	delays []time.Duration

	interval time.Duration

	current *atomic.Int32

	outCh chan struct{}
}

// New creates a new Ticker
func New(args *Args) (*Ticker, error) {
	if err := args.Validate(); err != nil {
		return nil, fmt.Errorf("invalid interval args: %w", err)
	}

	return &Ticker{
		log:      args.Log,
		clock:    args.Clock,
		timer:    args.Clock.Timer(0),
		delays:   args.getDelays(),
		interval: args.Interval,
		current:  atomic.NewInt32(0),
		outCh:    make(chan struct{}, defaultOutChBuffer),
	}, nil
}

// cleanup stops the timer and drains the channel also resets the current index of the delays.
func (t *Ticker) cleanup() {
	if !t.timer.Stop() {
		select {
		case <-t.timer.Alert():
			// drain the channel, but usually it's shouldn't reach here
		default:
		}
	}

	t.current.Store(0)
}

// runLoop runs the interval loop
// - increments the current delay
// - resets the timer
// - sends to the outCh if it's not full and someone is waiting by Alert (non-blocking)
func (t *Ticker) runLoop(ctx context.Context) {
	for {
		select {
		case <-ctx.Done():
			t.log.Debug("ticker context is done, cleaning up")
			t.cleanup()
			return
		case ts := <-t.timer.Alert():
			t.handleAlert(ts)
		}
	}
}

// calcDelay calculates the delay from the timestamp to the next alert
func (t *Ticker) calcDelay(ts time.Time, delay time.Duration) time.Duration {
	return ts.Add(delay).Sub(t.clock.Now().UTC())
}

func (t *Ticker) getInitialDelay() time.Duration {
	if len(t.delays) == 0 {
		return t.interval
	}

	return t.delays[0]
}

// handleAlert handles the alert from the timer
// - sends to the outCh if it's not full and someone is waiting by Alert (non-blocking)
// - resets the timer with the interval if there are no delays
// - increments the current delay and resets the timer if the current delay is less than the number of delays
// - all other cases we are not resetting the timer
func (t *Ticker) handleAlert(ts time.Time) {
	// send to the channel if it's not full and someone is waiting (non-blocking)
	// - this is done to avoid blocking the interval loop
	// - the interval loop is not blocked by the channel or the timer
	select {
	case t.outCh <- struct{}{}:
	default:
		t.log.Warn("ticker channel is full, skipping alert")
	}

	// if there are no delays, reset the timer with the interval
	if len(t.delays) == 0 {
		t.timer.Reset(t.calcDelay(ts, t.interval))
		return
	}

	// if the current delay is greater than the number of delays, reset the timer with the interval
	current := t.current.Load()
	if int(current) > len(t.delays)-1 {
		t.timer.Reset(t.calcDelay(ts, t.interval))
		return
	}

	// if the current delay wasn't changed from outside, increment the current delay and reset the timer
	if t.current.CompareAndSwap(current, current+1) {
		t.timer.Reset(t.calcDelay(ts, t.delays[current]))
		return
	}

	t.log.Info(
		"current index was changed from outside, no need to reset the timer: " +
			"inLoopCurrentIndex(" + strconv.FormatInt(int64(current), 10) + ") " +
			"actualDelaysLength(" + strconv.FormatInt(int64(t.current.Load()), 10) + ")",
	)
}

// Run validates interval and starts the interval loop
func (t *Ticker) Run(ctx context.Context) error { go t.runLoop(ctx); return nil }

// Alert returns the alert channel of the interval
// - the channel is non-blocking
// - the channel is buffered with size 1
// - the channel is used to signal the interval
func (t *Ticker) Alert() <-chan struct{} { return t.outCh }

// Reset resets the interval
// - timestamp is the time to start the interval from
func (t *Ticker) Reset(ts time.Time) bool {
	defer func() { t.log.Debug("resetting the timer at: " + ts.String()) }()
	t.cleanup()

	return t.timer.Reset(t.calcDelay(ts, t.getInitialDelay()))
}
