package utils

import "time"

// Clock should be injected into any component that requires access to time
type Clock interface {
	Now() time.Time
	Timer(d time.Duration) Timer
	Sleep(d time.Duration)
}

// Timer wraps the time.Timer object for mockability in test cases
type Timer interface {
	Alert() <-chan time.Time
	Reset(d time.Duration) bool
	Stop() bool
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
