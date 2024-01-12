package utils

import (
	"sync"
	"time"
)

// NewMockClock constructor
func NewMockClock() *MockClock {
	return &MockClock{}
}

// MockClock represents a fake time service for testing purposes
type MockClock struct {
	m sync.RWMutex

	currentTime time.Time
	timers      []*mockTimer
	tickers     []*mockTicker
}

// SetTime sets the time that this mock clock will always return
func (mc *MockClock) SetTime(t time.Time) {
	mc.m.Lock()
	defer mc.m.Unlock()

	mc.currentTime = t
	mc.timers = mc.fireTimers(t)
	mc.tickers = mc.fireTickers(t)
}

func (mc *MockClock) fireTimers(t time.Time) []*mockTimer {
	unfiredTimers := make([]*mockTimer, 0)
	for _, timer := range mc.timers {
		if timer.fireTime.Before(t) || timer.fireTime.Equal(t) {
			timer.Fire(timer.fireTime)
		} else {
			unfiredTimers = append(unfiredTimers, timer)
		}
	}
	return unfiredTimers
}

func (mc *MockClock) fireTickers(t time.Time) []*mockTicker {
	unfiredTickers := make([]*mockTicker, 0)
	for _, ticker := range mc.tickers {
		if ticker.fireTime.Before(t) || ticker.fireTime.Equal(t) {
			ticker.Fire(ticker.fireTime)
			ticker.Reset(ticker.duration)
		}
		unfiredTickers = append(unfiredTickers, ticker)
	}
	return unfiredTickers
}

// IncTime advances the clock by the duration
func (mc *MockClock) IncTime(d time.Duration) {
	mc.m.Lock()
	defer mc.m.Unlock()

	mc.currentTime = mc.currentTime.Add(d)
	mc.timers = mc.fireTimers(mc.currentTime)
	mc.tickers = mc.fireTickers(mc.currentTime)
}

// Now returns the desired time for tests
func (mc *MockClock) Now() time.Time {
	mc.m.RLock()
	defer mc.m.RUnlock()

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
	now := mc.Now()

	timer := &mockTimer{
		clock:    now,
		fireTime: now.Add(d),
		alertCh:  make(chan time.Time, 1),
	}
	mc.m.Lock()
	mc.timers = append(mc.timers, timer)
	mc.m.Unlock()

	return timer
}

type mockTimer struct {
	m        sync.Mutex
	clock    time.Time
	fireTime time.Time
	active   bool
	alertCh  chan time.Time
}

func (m *mockTimer) Fire(t time.Time) {
	select {
	case m.alertCh <- t:
	default:
	}
}

func (m *mockTimer) Alert() <-chan time.Time {
	defer func() {
		m.m.Lock()
		m.active = false
		m.m.Unlock()
	}()
	return m.alertCh
}

func (m *mockTimer) Reset(d time.Duration) bool {
	m.m.Lock()
	defer m.m.Unlock()

	m.fireTime = m.clock.Add(d)
	active := m.active
	m.active = true
	return active
}

func (m *mockTimer) Stop() bool {
	m.m.Lock()
	defer m.m.Unlock()

	active := m.active
	m.active = false
	return active
}

type mockTicker struct {
	m sync.Mutex

	clock    time.Time
	fireTime time.Time
	active   bool
	alertCh  chan time.Time
	duration time.Duration
}

// Ticker returns a ticker given duration
func (mc *MockClock) Ticker(d time.Duration) Ticker {
	now := mc.Now()

	ticker := &mockTicker{
		clock:    now,
		fireTime: now.Add(d),
		alertCh:  make(chan time.Time, 1),
		duration: d,
	}
	mc.m.Lock()
	mc.tickers = append(mc.tickers, ticker)
	mc.m.Unlock()
	return ticker
}

func (mt *mockTicker) Fire(t time.Time) {
	select {
	case mt.alertCh <- t:
	default:
	}
}

func (mt *mockTicker) Alert() <-chan time.Time {
	return mt.alertCh
}

func (mt *mockTicker) Reset(d time.Duration) {
	mt.m.Lock()
	defer mt.m.Unlock()

	mt.fireTime = mt.clock.Add(d)
	mt.duration = d
	mt.active = true
}

func (mt *mockTicker) Stop() {
	mt.m.Lock()
	defer mt.m.Unlock()

	mt.active = false
}
