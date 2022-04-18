package utils

import (
	"time"
)

// Clock should be injected into any component that requires access to time
type Clock interface {
	Now() time.Time
	Timer(d time.Duration) Timer
	Sleep(d time.Duration)
	AfterFunc(d time.Duration, f func()) *time.Timer
	Ticker(d time.Duration) Ticker
}

// Timer wraps the time.Timer object for mockability in test cases
type Timer interface {
	Alert() <-chan time.Time
	Reset(d time.Duration) bool
	Stop() bool
}

// Ticker wraps the time.Ticker object for mockbility in the test cases
type Ticker interface {
	Alert() <-chan time.Time
	Reset(d time.Duration)
	Stop()
}

// RealClock represents the typical clock implementation using the built-in time.Time
type RealClock struct{}

// Now returns the current system time
func (RealClock) Now() time.Time {
	return time.Now()
}

// Timer returns a timer that will fire after the provided duration
func (RealClock) Timer(d time.Duration) Timer {
	return realTimer{time.NewTimer(d)}
}

// Sleep pauses the current goroutine for the specified duration
func (RealClock) Sleep(d time.Duration) {
	time.Sleep(d)
}

// Ticker returns a ticker given duration
func (RealClock) Ticker(d time.Duration) Ticker {
	return realTicker{time.NewTicker(d)}
}

// AfterFunc calls function f in new go routine after specified duration d
func (RealClock) AfterFunc(d time.Duration, f func()) *time.Timer {
	return time.AfterFunc(d, f)
}

type realTimer struct {
	*time.Timer
}

func (r realTimer) Alert() <-chan time.Time {
	return r.Timer.C
}

func (r realTimer) Reset(d time.Duration) bool {
	return r.Timer.Reset(d)
}

func (r realTimer) Stop() bool {
	return r.Timer.Stop()
}

type realTicker struct {
	*time.Ticker
}

func (r realTicker) Alert() <-chan time.Time {
	return r.Ticker.C
}

func (r realTicker) Reset(d time.Duration) {
	r.Ticker.Reset(d)
}

func (r realTicker) Stop() {
	r.Ticker.Stop()
}
