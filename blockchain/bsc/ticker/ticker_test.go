package ticker

import (
	"context"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/stretchr/testify/require"
	"go.uber.org/atomic"

	"github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const defaultSleep = time.Millisecond * 100

func testTickerRun(ctx context.Context, t *testing.T, wg *sync.WaitGroup, ticker *Ticker) {
	t.Helper()

	startCh := make(chan struct{}, 1)
	wg.Add(1)
	go func() {
		close(startCh)
		defer wg.Done()

		require.NoError(t, ticker.Run(ctx))
	}()
	<-startCh
}

func testTickerAlert(ctx context.Context, cancel context.CancelFunc, t *testing.T, wg *sync.WaitGroup, ticker *Ticker, alerts *atomic.Int32, doneCh <-chan struct{}) {
	t.Helper()

	startCh := make(chan struct{}, 1)
	wg.Add(1)
	go func() {
		close(startCh)
		defer wg.Done()

		delay := time.Second * 5

		timer := time.NewTimer(delay)
		defer timer.Stop()

		for {
			select {
			case <-ctx.Done():
				return
			case <-doneCh:
				return
			case <-timer.C:
				cancel()
				return
			case <-ticker.Alert():
				alerts.Inc()
				timer.Stop()
				timer.Reset(delay)
			}
		}
	}()
	<-startCh
}

func TestTicker(t *testing.T) {
	const (
		delay1 = time.Second
		delay2 = delay1 * 2
		delay3 = delay2 * 2
	)

	logger.SetOutput(os.Stderr)
	logger.SetLevel(logger.DebugLevel)

	clock := new(utils.MockClock)
	clock.SetTime(time.Now())

	log := logger.Discard()

	invalidArgs := new(Args)
	validDelayArgs := &Args{
		Log:      log,
		Clock:    clock,
		Interval: delay1,
		Delays:   []time.Duration{delay1, delay2, delay3},
	}
	validDelayArgsDelays := validDelayArgs.getDelays()
	validIntervalArgs := &Args{
		Log:      log,
		Clock:    clock,
		Interval: delay1,
	}

	rootCtx, rootCancel := context.WithCancel(context.Background())
	defer rootCancel()

	t.Run("InvalidArgs", func(t *testing.T) {
		ticker, err := New(invalidArgs)
		require.Nil(t, ticker)
		require.Error(t, err)
	})

	t.Run("ValidDelayWithIntervalArgs", func(t *testing.T) {
		ticker, err := New(validDelayArgs)
		require.NoError(t, err)
		require.NotNil(t, ticker)

		t.Run("handleAlert", func(t *testing.T) {
			/*
				checks that the ticker handles the alerts correctly
				and subscribers receive the alerts
			*/
			now := clock.Now()
			ticker.Reset(now)

			wg := new(sync.WaitGroup)

			var (
				fireTime time.Time

				index int32

				expectedAlerts int32

				alerts = atomic.NewInt32(0)
			)

			doneCh := make(chan struct{})

			ctx, cancel := context.WithCancel(rootCtx)
			defer cancel()

			testTickerAlert(ctx, cancel, t, wg, ticker, alerts, doneCh)

			{ // first expected alert
				fireTime = now.Add(ticker.delays[index])
				clock.SetTime(fireTime)
				time.Sleep(defaultSleep)
				ticker.handleAlert(fireTime)
				index = ticker.current.Load()
				require.Equal(t, int32(1), index)
				require.Equal(t, validDelayArgsDelays[1], ticker.delays[index])
				expectedAlerts++
			}

			{ // second expected alert
				fireTime = clock.Now().Add(ticker.delays[index])
				clock.SetTime(fireTime)
				time.Sleep(defaultSleep)
				ticker.handleAlert(fireTime)
				index = ticker.current.Load()
				require.Equal(t, int32(2), index)
				require.Equal(t, validDelayArgsDelays[2], ticker.delays[index])
				expectedAlerts++
			}

			{ // third expected alert
				fireTime = clock.Now().Add(ticker.delays[index])
				clock.SetTime(fireTime)
				time.Sleep(defaultSleep)
				ticker.handleAlert(fireTime)
				index = ticker.current.Load()
				require.Equal(t, int32(3), index)
				expectedAlerts++
			}

			{ // fourth expected alert (interval)
				fireTime = clock.Now().Add(ticker.interval)
				clock.SetTime(fireTime)
				time.Sleep(defaultSleep)
				ticker.handleAlert(fireTime)
				index = ticker.current.Load()
				require.Equal(t, int32(3), index)
				expectedAlerts++
			}

			{ // fifth expected alert (interval)
				fireTime = clock.Now().Add(ticker.interval)
				clock.SetTime(fireTime)
				time.Sleep(defaultSleep)
				ticker.handleAlert(fireTime)
				index = ticker.current.Load()
				require.Equal(t, int32(3), index)
				expectedAlerts++
			}

			for expectedAlerts != alerts.Load() {
				select {
				case <-ctx.Done():
					require.Fail(t, "context done")
					return
				default:
				}
			}

			close(doneCh)

			wg.Wait()

			require.Equal(t, expectedAlerts, alerts.Load())
		})

		t.Run("RunAndStop", func(t *testing.T) {
			/*
				checks that the ticker runs and stops correctly
			*/
			ctx, cancel := context.WithCancel(rootCtx)
			defer cancel()

			now := clock.Now()
			ticker.Reset(now)

			wg := new(sync.WaitGroup)

			testTickerRun(ctx, t, wg, ticker)

			cancel()
			wg.Wait()

			var stopped bool
			for !stopped {
				select {
				case <-time.After(time.Second * 3):
					require.Fail(t, "ticker did not stop")
				default:
					stopped = int32(0) == ticker.current.Load()
				}
			}
		})
	})

	t.Run("ValidDelayArgs", func(t *testing.T) {
		ticker, err := New(validIntervalArgs)
		require.NoError(t, err)
		require.NotNil(t, ticker)

		t.Run("handleAlert", func(t *testing.T) {
			/*
				checks that the ticker handles the alerts correctly
				and subscribers receive the alerts
			*/
			now := clock.Now()
			ticker.Reset(now)

			wg := new(sync.WaitGroup)

			var (
				fireTime time.Time

				index int32

				expectedAlerts int32

				alerts = atomic.NewInt32(0)
			)

			doneCh := make(chan struct{})

			ctx, cancel := context.WithCancel(rootCtx)
			defer cancel()

			testTickerAlert(ctx, cancel, t, wg, ticker, alerts, doneCh)

			{ // first expected alert
				fireTime = now.Add(ticker.interval)
				clock.SetTime(fireTime)
				time.Sleep(defaultSleep)
				ticker.handleAlert(fireTime)
				index = ticker.current.Load()
				require.Equal(t, int32(0), index)
				expectedAlerts++
			}

			{ // second expected alert
				fireTime = clock.Now().Add(ticker.interval)
				clock.SetTime(fireTime)
				time.Sleep(defaultSleep)
				ticker.handleAlert(fireTime)
				index = ticker.current.Load()
				require.Equal(t, int32(0), index)
				expectedAlerts++
			}

			for expectedAlerts != alerts.Load() {
				select {
				case <-ctx.Done():
					require.Fail(t, "context done")
					return
				default:
				}
			}

			close(doneCh)

			wg.Wait()

			require.Equal(t, expectedAlerts, alerts.Load())
		})

		t.Run("RunAndStop", func(t *testing.T) {
			/*
				checks that the ticker runs and stops correctly
			*/
			ctx, cancel := context.WithCancel(rootCtx)
			defer cancel()

			now := clock.Now()
			ticker.Reset(now)

			wg := new(sync.WaitGroup)

			testTickerRun(ctx, t, wg, ticker)

			cancel()
			wg.Wait()

			var stopped bool
			for !stopped {
				select {
				case <-time.After(time.Second * 3):
					require.Fail(t, "ticker did not stop")
				default:
					stopped = int32(0) == ticker.current.Load()
				}
			}
		})
	})
}
