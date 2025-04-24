package services

import (
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
)

// Element represents an element stored in the history.
type Element[T any] struct {
	Expiration int64
	Value      T
}

// ExpiringMap holds hashes that we have seen in the past
type ExpiringMap[T any] struct {
	name        string // for logging
	clock       clock.Clock
	cleanupFreq time.Duration
	data        *syncmap.SyncMap[string, *Element[T]]
}

// NewExpiringMap creates a new object
func NewExpiringMap[T any](name string, cleanupFreq time.Duration) ExpiringMap[T] {
	return newExpiringMap[T](name, clock.RealClock{}, cleanupFreq)
}

func newExpiringMap[T any](name string, clock clock.Clock, cleanupFreq time.Duration) ExpiringMap[T] {
	em := ExpiringMap[T]{
		name:        name,
		clock:       clock,
		cleanupFreq: cleanupFreq,
		data:        syncmap.NewStringMapOf[*Element[T]](),
	}
	go em.cleanup()
	return em
}

// Add adds the hash for the duration
func (em ExpiringMap[T]) Add(hash string, value T, expiration time.Duration) {
	expirationTime := em.clock.Now().Add(expiration).UnixNano()
	em.data.Store(hash, &Element[T]{Expiration: expirationTime, Value: value})
}

// Remove removes the hash from the data
func (em ExpiringMap[T]) Remove(hash string) {
	em.data.Delete(hash)
}

// SetIfAbsent Sets the given value under the specified key if no value was associated with it.
func (em ExpiringMap[T]) SetIfAbsent(hash string, value T, expiration time.Duration) bool {
	expirationTime := em.clock.Now().Add(expiration).UnixNano()
	_, exists := em.data.LoadOrStore(hash, &Element[T]{Expiration: expirationTime, Value: value})

	return !exists
}

// Exists checks if hash is in history and if found returns the expiration status
func (em ExpiringMap[T]) Exists(hash string) (bool, bool) {
	expired := false
	if element, ok := em.data.Load(hash); ok {
		if em.clock.Now().UnixNano() > element.Expiration {
			expired = true
		}
		return true, expired
	}
	return false, expired
}

// Get checks if hash is in history and if found returns the value
func (em ExpiringMap[T]) Get(hash string) (*T, bool) {
	if element, ok := em.data.Load(hash); ok {
		return &element.Value, true
	}
	return nil, false
}

// All iterates over all elements and applies given function
func (em ExpiringMap[T]) All(process func(k string, v *Element[T]) bool) {
	em.data.Range(process)
}

// Count provides the size of the history
func (em ExpiringMap[T]) Count() int {
	return em.data.Size()
}

func (em ExpiringMap[T]) cleanup() {
	ticker := em.clock.Ticker(em.cleanupFreq)
	for range ticker.Alert() {
		itemsCleaned := em.clean()
		log.Debugf("cleaned %v entries in ExpiringMap[%v], the remaining entries are %v", itemsCleaned, em.name, em.Count())
	}
}

func (em ExpiringMap[T]) clean() int {
	historyCleaned := 0
	timeNow := em.clock.Now().UnixNano()

	em.data.Range(func(key string, element *Element[T]) bool {
		if timeNow > element.Expiration {
			em.data.Delete(key)
			historyCleaned++
		}
		return true
	})

	return historyCleaned
}
