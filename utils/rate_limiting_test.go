package utils

import (
	"fmt"
	"testing"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/stretchr/testify/assert"
)

func TestLeakyBucketRateLimiter_refill(t *testing.T) {
	testTable := []struct {
		counter    float32
		limit      uint64
		refillRate float32
		timePassed time.Duration
		endCount   float32
	}{
		{3, 5, 1, time.Millisecond, 4},
		{3, 5, 2, time.Millisecond, 5},
		{3, 5, 1, time.Millisecond * 2, 5},
		{3, 5, 1, time.Millisecond * 3, 5},
		{3, 5, 1, time.Millisecond * 4, 5},
		{1, 3, 3, time.Millisecond, 3},
		{1, 3, 0.5, time.Millisecond * 3, 2.5},
		{1.2, 6, 0.4, time.Millisecond * 3, 2.4},
	}

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase), func(t *testing.T) {
			clock := MockClock{}
			clock.SetTime(time.Unix(0, 0))
			l := leakyBucketRateLimiter{
				clock: &clock,
				bucket: bucket{
					counter: testCase.counter,
					limit:   testCase.limit,
				},
				lastCall:   clock.Now(),
				interval:   time.Millisecond,
				refillRate: testCase.refillRate,
			}

			clock.IncTime(testCase.timePassed)
			endCount := l.refill()

			assert.Equal(t, testCase.endCount, endCount)
		})
	}
}

func TestLeakyBucketRateLimiter_Take_RefillRateCalculatedCorrectly(t *testing.T) {
	testTable := []struct {
		limit              uint64
		interval           time.Duration
		expectedRefillRate float32
	}{
		{5, time.Second, 0.005},
		{5, time.Millisecond, 5},
		{10, time.Millisecond * 5, 2},
		{10, time.Millisecond * 500, 0.02},
	}

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase), func(t *testing.T) {
			l := NewLeakyBucketRateLimiter(&MockClock{}, testCase.limit, testCase.interval)

			l.Take()
			rateLimiter := l.(*leakyBucketRateLimiter)

			assert.Equal(t, testCase.expectedRefillRate, rateLimiter.refillRate)
		})
	}
}

func TestLeakyBucketRateLimiter_Take_NotSuccessfulWhenOutOfCalls(t *testing.T) {
	testTable := []struct {
		startingCounter float32
		result          bool
	}{
		{0.5, false},
		{1, true},
		{2.5, true},
	}

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase), func(t *testing.T) {
			l := leakyBucketRateLimiter{
				clock: &MockClock{},
				bucket: bucket{
					counter: testCase.startingCounter,
					limit:   5,
				},
			}

			result, _ := l.Take()

			assert.Equal(t, testCase.result, result)
		})
	}
}

func TestLeakyBucketRateLimiter_Take_BucketCounterSameWhenOutOfCalls(t *testing.T) {
	testTable := []struct {
		startingCounter float32
		endingCounter   float32
	}{
		{0.5, 0.5},
		{0.9, 0.9},
		{1, 0},
		{2.5, 1.5},
	}

	for _, testCase := range testTable {
		t.Run(fmt.Sprint(testCase), func(t *testing.T) {
			l := leakyBucketRateLimiter{
				clock: &MockClock{},
				bucket: bucket{
					counter: testCase.startingCounter,
					limit:   5,
				},
			}

			_, counter := l.Take()

			assert.Equal(t, testCase.endingCounter, counter)
		})
	}
}

func TestTxTraceLeakyBucketRateLimiter_Take_HasCorrectLogging(t *testing.T) {
	mockClock := &MockClock{}
	startTime := time.Unix(0, 0)
	mockClock.SetTime(startTime)
	l := NewTxToolsLeakyBucketRateLimiter(mockClock, 2, PerSecond, "abc")

	hook := log.NewGlobal()

	res, counter := l.Take()
	if len(hook.AllEntries()) != 1 {
		t.FailNow()
	}
	assert.True(t, res)
	assert.Equal(t, "Account ID abc has 1 / 2 tx trace calls left per second", hook.LastEntry().Message)
	assert.Equal(t, float32(1), counter)

	res, counter = l.Take()
	if len(hook.AllEntries()) != 2 {
		t.FailNow()
	}
	assert.True(t, res)
	assert.Equal(t, "Account ID abc has 0 / 2 tx trace calls left per second", hook.LastEntry().Message)
	assert.Equal(t, float32(0), counter)

	mockClock.SetTime(startTime.Add(time.Millisecond * 300))
	res, counter = l.Take()
	if len(hook.AllEntries()) != 3 {
		t.FailNow()
	}
	assert.False(t, res)
	assert.Equal(t, "Account ID abc has 0.6 / 2 tx trace calls left per second", hook.LastEntry().Message)
	assert.Equal(t, float32(0.6), counter)

	mockClock.SetTime(startTime.Add(time.Millisecond * 450))
	res, counter = l.Take()
	if len(hook.AllEntries()) != 4 {
		t.FailNow()
	}
	assert.False(t, res)
	assert.Equal(t, "Account ID abc has 0.90000004 / 2 tx trace calls left per second", hook.LastEntry().Message)
	assert.Equal(t, float32(0.90000004), counter)

	mockClock.SetTime(startTime.Add(time.Millisecond * 500))
	res, counter = l.Take()
	if len(hook.AllEntries()) != 5 {
		t.FailNow()
	}
	assert.True(t, res)
	assert.Equal(t, "Account ID abc has 0 / 2 tx trace calls left per second", hook.LastEntry().Message)
	assert.Equal(t, float32(0), counter)

	mockClock.SetTime(startTime.Add(time.Second * 2))
	res, counter = l.Take()
	if len(hook.AllEntries()) != 6 {
		t.FailNow()
	}
	assert.True(t, res)
	assert.Equal(t, "Account ID abc has 1 / 2 tx trace calls left per second", hook.LastEntry().Message)
	assert.Equal(t, float32(1), counter)
}
