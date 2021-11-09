package bxmock

import (
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/utils"
	"sync"
	"time"
)

// MockClock represents a fake time service for testing purposes
type MockClock struct {
	m sync.Mutex

	currentTime time.Time
	timers      []*mockTimer
}

// SetTime sets the time that this mock clock will always return
func (mc *MockClock) SetTime(t time.Time) {
	mc.m.Lock()
	defer mc.m.Unlock()

	mc.currentTime = t

	unfiredTimers := make([]*mockTimer, 0)
	for _, timer := range mc.timers {
		if timer.fireTime.Before(t) || timer.fireTime.Equal(t) {
			timer.Fire(timer.fireTime)
		} else {
			unfiredTimers = append(unfiredTimers, timer)
		}
	}

	mc.timers = unfiredTimers
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

// Timer returns a mock timer that will fire when mock clock ticks past its fire time
func (mc *MockClock) Timer(d time.Duration) utils.Timer {
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
