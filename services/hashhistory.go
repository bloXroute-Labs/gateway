package services

import (
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	cmap "github.com/orcaman/concurrent-map"
	"time"
)

// HashHistory holds hashes that we have seen in the past
type HashHistory struct {
	name        string // for logging
	clock       utils.Clock
	cleanupFreq time.Duration
	data        cmap.ConcurrentMap
}

// NewHashHistory creates a new object
func NewHashHistory(name string, cleanupFreq time.Duration) HashHistory {
	return newHashHistory(name, utils.RealClock{}, cleanupFreq)
}

func newHashHistory(name string, clock utils.Clock, cleanupFreq time.Duration) HashHistory {
	hh := HashHistory{
		name:        name,
		clock:       clock,
		cleanupFreq: cleanupFreq,
		data:        cmap.New(),
	}
	go hh.cleanup()
	return hh
}

// Add adds the hash for the duration
func (hh HashHistory) Add(hash string, expiration time.Duration) {
	hh.data.Set(hash, hh.clock.Now().Add(expiration).UnixNano())
}

// Remove removes the hash from the data
func (hh HashHistory) Remove(hash string) {
	hh.data.Remove(hash)
}

// SetIfAbsent Sets the given value under the specified key if no value was associated with it.
func (hh HashHistory) SetIfAbsent(hash string, expiration time.Duration) bool {
	return hh.data.SetIfAbsent(hash, hh.clock.Now().Add(expiration).UnixNano())
}

// Exists checks if hash is in history
func (hh HashHistory) Exists(hash string) bool {
	if val, ok := hh.data.Get(hash); ok {
		expiration := val.(int64)
		if hh.clock.Now().UnixNano() < expiration {
			return true
		}
	}
	return false
}

// Count provides the size of the history
func (hh HashHistory) Count() int {
	return hh.data.Count()
}

func (hh HashHistory) cleanup() {
	ticker := hh.clock.Ticker(hh.cleanupFreq)
	for {
		select {
		case <-ticker.Alert():
			itemsCleaned := hh.clean()
			log.Debugf("cleaned %v entries in HashHistory[%v], the remaining entries are %v", itemsCleaned, hh.name, hh.Count())
		}
	}
}

func (hh HashHistory) clean() int {
	historyCleaned := 0
	timeNow := hh.clock.Now().UnixNano()
	for item := range hh.data.IterBuffered() {
		expiration := item.Val.(int64)
		if timeNow > expiration {
			hh.data.Remove(item.Key)
			historyCleaned++
		}
	}
	return historyCleaned
}
