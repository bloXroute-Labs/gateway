package utils

import (
	"testing"
	"time"

	bxclock "github.com/bloXroute-Labs/bxcommon-go/clock"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestNewTimeSeriesCounterPoolPeerDoesNotExists(t *testing.T) {
	id := bxtypes.NodeID("id")
	clock := bxclock.MockClock{}
	now := time.Now()
	clock.SetTime(now)

	seriesCounterPool := NewTimeSeriesCounterPool(&clock, time.Minute, time.Minute)
	assert.Equal(t, 0, seriesCounterPool.GetCount(id))
	seriesCounterPool.Track(id)
	assert.Equal(t, 0, seriesCounterPool.GetCount(id))
}

func TestNewTimeSeriesCounterPoolPeerTrack(t *testing.T) {
	id := bxtypes.NodeID("id")
	clock := bxclock.MockClock{}
	now := time.Now()
	clock.SetTime(now)

	seriesCounterPool := NewTimeSeriesCounterPool(&clock, time.Minute, time.Minute)
	seriesCounterPool.AddPeer(id)
	seriesCounterPool.Track(id)
	assert.Equal(t, 1, seriesCounterPool.GetCount(id))
}

func TestNewTimeSeriesCounterPoolPeerRemovedPeer(t *testing.T) {
	id := bxtypes.NodeID("id")
	clock := bxclock.MockClock{}
	now := time.Now()
	clock.SetTime(now)

	seriesCounterPool := NewTimeSeriesCounterPool(&clock, time.Minute, time.Minute)
	seriesCounterPool.AddPeer(id)
	seriesCounterPool.Track(id)
	assert.Equal(t, 1, seriesCounterPool.GetCount(id))
	seriesCounterPool.RemovePeer(id)
	assert.Equal(t, 0, seriesCounterPool.GetCount(id))
}

func TestNewTimeSeriesCounterPoolPeerExpiration(t *testing.T) {
	id := bxtypes.NodeID("id")
	clock := &bxclock.MockClock{}
	now := time.Now()
	clock.SetTime(now)

	seriesCounterPool := NewTimeSeriesCounterPool(clock, 5*time.Minute, time.Minute)
	seriesCounterPool.AddPeer(id)

	seriesCounterPool.Track(id)

	require.Equal(t, 1, seriesCounterPool.GetCount(id))

	clock.IncTime(5 * time.Minute)
	assert.Equal(t, 1, seriesCounterPool.GetCount(id))

	clock.IncTime(time.Millisecond)
	assert.Equal(t, 0, seriesCounterPool.GetCount(id))
}
