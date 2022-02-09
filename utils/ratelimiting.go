package utils

import (
	"fmt"
	"github.com/bloXroute-Labs/gateway/types"
	log "github.com/sirupsen/logrus"
	"time"
)

// RateLimiter represents any struct that can be used to limit the amount of calls per time period
type RateLimiter interface {
	Take() (bool, float32)
	Limit() int
	refill() float32
	String() string
}

// bucket keeps track of the calls made by an account during an interval
type bucket struct {
	limit   int
	counter float32
}

// leakyBucketRateLimiter enables rate limiting using the leaky bucket algorithm
type leakyBucketRateLimiter struct {
	clock      Clock
	bucket     bucket
	interval   time.Duration
	lastCall   time.Time
	refillRate float32 // per millisecond
}

// NewLeakyBucketRateLimiter creates a new leakyBucketRateLimiter
func NewLeakyBucketRateLimiter(clock Clock, limit int, interval time.Duration) RateLimiter {
	rateLimiter := &leakyBucketRateLimiter{
		clock:    clock,
		interval: interval,
		lastCall: clock.Now(),
		bucket: bucket{
			limit:   limit,
			counter: float32(limit),
		},
	}
	rateLimiter.refillRate = float32(rateLimiter.bucket.limit) / float32(rateLimiter.interval.Milliseconds())

	return rateLimiter
}

// Take returns true if the action is allowed and false if not and updates the bucket count as necessary
func (l *leakyBucketRateLimiter) Take() (bool, float32) {
	l.refill()

	if l.bucket.counter < 1 {
		return false, l.bucket.counter
	}
	l.bucket.counter--

	return true, l.bucket.counter
}

func (l *leakyBucketRateLimiter) Limit() int {
	return l.bucket.limit
}

// refill refills the bucket counter upto the limit based on the time since the last call and the refill rate
func (l *leakyBucketRateLimiter) refill() float32 {
	timePassed := l.clock.Now().Sub(l.lastCall).Milliseconds()
	refillAmount := float32(timePassed) * l.refillRate
	l.lastCall = l.clock.Now()

	if l.bucket.counter+refillAmount <= float32(l.bucket.limit) {
		l.bucket.counter += refillAmount
	} else {
		l.bucket.counter = float32(l.bucket.limit)
	}

	return l.bucket.counter
}

func (l *leakyBucketRateLimiter) String() string {
	return fmt.Sprintf("Limit: %v | Counter: %v | Interval: %v | Last Call: %v | Refill Rate: %v",
		l.bucket.limit, l.bucket.counter, l.interval, l.lastCall.Format(time.RFC3339Nano), l.refillRate)
}

// rateLimitType specifies the time period the txTraceLeakyBucketRateLimiter manages the rate over (should match the `interval` in rateLimiter)
type rateLimitType string

// Daily means that the rate limiter manages the rate over a day
// PerSecond means that the rate limiter manages the rate over a second
// PerMillisecond means that the rate limiter manages the rate over a millisecond
const (
	Daily          rateLimitType = "day"
	PerSecond      rateLimitType = "second"
	PerMillisecond rateLimitType = "millisecond"
)

var rateLimitTypeToIntervalDuration = map[rateLimitType]time.Duration{
	Daily:          time.Hour * 24,
	PerSecond:      time.Second,
	PerMillisecond: time.Millisecond,
}

// txTraceLeakyBucketRateLimiter adds extra logging during Take as a sanity check when running the txtrace API
type txTraceLeakyBucketRateLimiter struct {
	*leakyBucketRateLimiter
	accountID     types.AccountID
	rateLimitType rateLimitType
}

// Take specifies if the call is allowed to be made, logs the counter, and returns the counter left in the bucket
func (t *txTraceLeakyBucketRateLimiter) Take() (bool, float32) {
	res, counter := t.leakyBucketRateLimiter.Take()

	log.Debugf("Account ID %v has %v / %v tx trace calls left per %s", t.accountID, t.leakyBucketRateLimiter.bucket.counter,
		t.leakyBucketRateLimiter.bucket.limit, t.rateLimitType)

	return res, counter
}

// NewTxTraceLeakyBucketRateLimiter creates a RateLimiter using the leaky bucket rate algorithm; it has logging during `Take()` compared to the leakyBucketRateLimiter
func NewTxTraceLeakyBucketRateLimiter(clock Clock, limit int, rateLimitType rateLimitType, accountID types.AccountID) RateLimiter {
	interval := rateLimitTypeToIntervalDuration[rateLimitType]
	rateLimiter := NewLeakyBucketRateLimiter(clock, limit, interval).(*leakyBucketRateLimiter)

	return &txTraceLeakyBucketRateLimiter{
		leakyBucketRateLimiter: rateLimiter,
		accountID:              accountID,
		rateLimitType:          rateLimitType,
	}
}
