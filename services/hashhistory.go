package services

import (
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/utils"
	cmap "github.com/orcaman/concurrent-map"
	log "github.com/sirupsen/logrus"
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
	hh.data.Set(hash, hh.clock.Now().Add(expiration))
}

// SetIfAbsent Sets the given value under the specified key if no value was associated with it.
func (hh HashHistory) SetIfAbsent(hash string, expiration time.Duration) bool {
	return hh.data.SetIfAbsent(hash, hh.clock.Now().Add(expiration))
}

// Exists checks if hash is in history
func (hh HashHistory) Exists(hash string) bool {
	if val, ok := hh.data.Get(hash); ok {
		expiration := val.(time.Time)
		if hh.clock.Now().Before(expiration) {
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
	ticker := time.NewTicker(hh.cleanupFreq)
	for {
		select {
		case <-ticker.C:
			itemsCleaned := hh.clean()
			log.Debugf("cleaned %v entries in HashHistory[%v]", itemsCleaned, hh.name)
		}
	}
}

func (hh HashHistory) clean() int {
	historyCleaned := 0
	timeNow := hh.clock.Now()
	for item := range hh.data.IterBuffered() {
		expiration := item.Val.(time.Time)
		if timeNow.After(expiration) {
			hh.data.Remove(item.Key)
			historyCleaned++
		}
	}
	return historyCleaned
}
