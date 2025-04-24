package utils

import (
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"

	"github.com/bloXroute-Labs/gateway/v2/utils/hasher"
)

// TimeSeriesCounterPool is a pool of time series counters based on peer id
type TimeSeriesCounterPool struct {
	duration          time.Duration
	fidelity          time.Duration
	clock             clock.Clock
	timeSeriesCounter *syncmap.SyncMap[bxtypes.NodeID, *TimeSeriesCounter]
}

// NewTimeSeriesCounterPool creates new time series counter pool
func NewTimeSeriesCounterPool(clock clock.Clock, duration, fidelity time.Duration) TimeSeriesCounterPool {
	return TimeSeriesCounterPool{
		duration:          duration,
		fidelity:          fidelity,
		clock:             clock,
		timeSeriesCounter: syncmap.NewTypedMapOf[bxtypes.NodeID, *TimeSeriesCounter](hasher.NodeIDHasher),
	}
}

// AddPeer adds new peer to the pool
func (tsc TimeSeriesCounterPool) AddPeer(peerID bxtypes.NodeID) {
	if tsc.timeSeriesCounter.Has(peerID) {
		log.Errorf("counter for %s peer already exists, can not add new one", peerID)
		return
	}

	tsc.timeSeriesCounter.Store(peerID, NewTimeSeriesCounter(tsc.clock, tsc.duration, tsc.fidelity))
}

// RemovePeer removes peer from the pool
func (tsc TimeSeriesCounterPool) RemovePeer(peerID bxtypes.NodeID) {
	if !tsc.timeSeriesCounter.Has(peerID) {
		return
	}

	tsc.timeSeriesCounter.Delete(peerID)
}

// Track increment counter for the peer
func (tsc TimeSeriesCounterPool) Track(peerID bxtypes.NodeID) {
	counter, ok := tsc.timeSeriesCounter.Load(peerID)
	if !ok {
		log.Errorf("counter for %s peer does not exists, can not track", peerID)
		return
	}

	counter.Track()
}

// GetCount returns current count for the peer
func (tsc TimeSeriesCounterPool) GetCount(peerID bxtypes.NodeID) int {
	counter, ok := tsc.timeSeriesCounter.Load(peerID)
	if !ok {
		log.Errorf("counter for %s peer does not exists, can not track", peerID)
		return 0
	}

	return counter.Count()
}
