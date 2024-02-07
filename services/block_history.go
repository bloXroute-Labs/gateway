package services

import (
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

const (
	blockExpirationTime = 10 * time.Minute
	// SeenFromBoth Relay and node saw the block
	SeenFromBoth = "seen from relay and from node"
	// SeenFromRelay Relay saw the block
	SeenFromRelay = "seen from relay"
	// SeenFromNode Node saw the block
	SeenFromNode = "seen from node"
	// FirstTimeSeen first time saw the block
	FirstTimeSeen = "first time seen"
)

// SeenStatus represent seen status string
type SeenStatus string

type blockData struct {
	expirationTime int64
	status         SeenStatus
}

// BlockHistory holds block hashes that we have seen in the past and if block received by blockchain node
type BlockHistory struct {
	name        string // for logging
	clock       utils.Clock
	cleanupFreq time.Duration
	data        *syncmap.SyncMap[string, blockData]
	expiration  time.Duration
}

// NewBlockHistory create a new object
func NewBlockHistory(name string, cleanupFreq time.Duration, clock utils.Clock) BlockHistory {
	bh := BlockHistory{
		name:        name,
		clock:       clock,
		cleanupFreq: cleanupFreq,
		data:        syncmap.NewStringMapOf[blockData](),
		expiration:  blockExpirationTime,
	}
	go bh.cleanup()
	return bh
}

// Count provides the size of the history
func (bh *BlockHistory) Count() int {
	return bh.data.Size()
}

func (bh *BlockHistory) cleanup() {
	ticker := bh.clock.Ticker(bh.cleanupFreq)
	for range ticker.Alert() {
		itemsCleaned := bh.clean()
		log.Debugf("cleaned %v entries in HashHistory[%v], the remaining entries are %v", itemsCleaned, bh.name, bh.Count())
	}
}

func (bh *BlockHistory) clean() int {
	historyCleaned := 0
	timeNow := bh.clock.Now().UnixNano()

	bh.data.Range(func(key string, blockData blockData) bool {
		if timeNow > blockData.expirationTime {
			bh.data.Delete(key)
			historyCleaned++
		}
		return true
	})

	return historyCleaned
}

// AddOrUpdate check if block hash is in the map if exist change status if not add to map
func (bh *BlockHistory) AddOrUpdate(hash string, status SeenStatus) {
	if val, ok := bh.data.Load(hash); ok {
		newBd := blockData{expirationTime: val.expirationTime, status: createNewStatus(status, val.status)}
		bh.data.Store(hash, newBd)
	} else {
		bd := blockData{
			expirationTime: bh.clock.Now().Add(bh.expiration).UnixNano(),
			status:         status,
		}
		bh.data.Store(hash, bd)
	}
}

func createNewStatus(newStatus SeenStatus, oldStatus SeenStatus) SeenStatus {
	switch oldStatus {
	case SeenFromBoth:
		return SeenFromBoth
	case SeenFromRelay:
		if newStatus == SeenFromNode {
			return SeenFromBoth
		}
	case SeenFromNode:
		if newStatus == SeenFromRelay {
			return SeenFromBoth
		}
	}
	return newStatus
}

// Status return current block status
func (bh *BlockHistory) Status(hash string) string {
	if val, ok := bh.data.Load(hash); ok {
		if bh.clock.Now().UnixNano() < val.expirationTime {
			return string(val.status)
		}
	}
	return FirstTimeSeen
}
