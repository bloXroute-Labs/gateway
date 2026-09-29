package utils

import (
	"fmt"
	"testing"
	"time"

	bxclock "github.com/bloXroute-Labs/bxcommon-go/v2/clock"
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
			clock := bxclock.MockClock{}
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
			l := NewLeakyBucketRateLimiter(&bxclock.MockClock{}, testCase.limit, testCase.interval)

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
				clock: &bxclock.MockClock{},
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
				clock: &bxclock.MockClock{},
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

