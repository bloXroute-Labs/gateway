package utils

import (
	"sync"
	"time"
)

// MockClock represents a fake time service for testing purposes
type MockClock struct {
	m sync.Mutex

	currentTime time.Time
	timers      []*mockTimer
	tickers     []*mockTicker
}

// SetTime sets the time that this mock clock will always return
func (mc *MockClock) SetTime(t time.Time) {
	mc.m.Lock()
	defer mc.m.Unlock()

	mc.currentTime = t
	mc.timers = *mc.fireTimers(t)
	mc.tickers = *mc.fireTickers(t)
}

func (mc *MockClock) fireTimers(t time.Time) *[]*mockTimer {
	unfiredTimers := make([]*mockTimer, 0)
	for _, timer := range mc.timers {
		if timer.fireTime.Before(t) || timer.fireTime.Equal(t) {
			timer.Fire(timer.fireTime)
		} else {
			unfiredTimers = append(unfiredTimers, timer)
		}
	}
	return &unfiredTimers
}

func (mc *MockClock) fireTickers(t time.Time) *[]*mockTicker {
	unfiredTickers := make([]*mockTicker, 0)
	for _, ticker := range mc.tickers {
		if ticker.fireTime.Before(t) || ticker.fireTime.Equal(t) {
			ticker.Fire(ticker.fireTime)
			ticker.Reset(ticker.duration)
		}
		unfiredTickers = append(unfiredTickers, ticker)
	}
	return &unfiredTickers
}

// IncTime advances the clock by the duration
func (mc *MockClock) IncTime(d time.Duration) {
	mc.SetTime(mc.currentTime.Add(d))
}

// Now returns the desired time for tests
func (mc MockClock) Now() time.Time {
	return mc.currentTime
}

// Sleep blocks the current goroutine until the timer has been incremented
func (mc *MockClock) Sleep(d time.Duration) {
	t := mc.Timer(d)
	<-t.Alert()
}

// AfterFunc advances the clock duration by d then calls function f in its own go routine
func (mc *MockClock) AfterFunc(d time.Duration, f func()) *time.Timer {
	go func() {
		mc.Sleep(d)
		f()
	}()
	return nil
}

// Timer returns a mock timer that will fire when mock clock ticks past its fire time
func (mc *MockClock) Timer(d time.Duration) Timer {
	mc.m.Lock()
	defer mc.m.Unlock()

	timer := &mockTimer{
		clock:    mc,
		fireTime: mc.Now().Add(d),
		alertCh:  make(chan time.Time, 1),
	}
	mc.timers = append(mc.timers, timer)
	return timer
}

type mockTimer struct {
	clock    *MockClock
	fireTime time.Time
	active   bool
	alertCh  chan time.Time
}

func (m *mockTimer) Fire(t time.Time) {
	m.alertCh <- t
}

func (m *mockTimer) Alert() <-chan time.Time {
	defer func() {
		m.active = false
	}()
	return m.alertCh
}

func (m *mockTimer) Reset(d time.Duration) bool {
	m.fireTime = m.clock.Now().Add(d)
	active := m.active
	m.active = true
	return active
}

func (m *mockTimer) Stop() bool {
	active := m.active
	m.active = false
	return active
}

type mockTicker struct {
	clock    *MockClock
	fireTime time.Time
	active   bool
	alertCh  chan time.Time
	duration time.Duration
}

// Ticker returns a ticker given duration
func (mc *MockClock) Ticker(d time.Duration) Ticker {
	mc.m.Lock()
	defer mc.m.Unlock()

	ticker := &mockTicker{
		clock:    mc,
		fireTime: mc.Now().Add(d),
		alertCh:  make(chan time.Time, 1),
		duration: d,
	}
	mc.tickers = append(mc.tickers, ticker)
	return ticker
}

func (m *mockTicker) Fire(t time.Time) {
	m.alertCh <- t
}

func (m *mockTicker) Alert() <-chan time.Time {
	return m.alertCh
}

func (m *mockTicker) Reset(d time.Duration) {
	m.fireTime = m.clock.Now().Add(d)
	m.duration = d
	m.active = true
}

func (m *mockTicker) Stop() {
	m.active = false
}
