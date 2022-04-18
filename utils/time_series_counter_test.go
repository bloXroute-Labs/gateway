package utils

import (
	"github.com/stretchr/testify/assert"
	"testing"
	"time"
)

func TestTimeSeriesCounter_Track(t *testing.T) {
	mc := MockClock{}
	tsc := NewTimeSeriesCounter(&mc, time.Hour, time.Minute)

	tsc.Track()
	tsc.Track()
	tsc.Track()
	assert.Equal(t, 3, tsc.Count())

	// t=20: 3 entries in last hour
	mc.IncTime(20 * time.Minute)

	assert.Equal(t, 3, tsc.Count())
	tsc.Track()
	tsc.Track()
	assert.Equal(t, 5, tsc.Count())

	// t=59: 9 entries in last hour
	mc.IncTime(39 * time.Minute)

	assert.Equal(t, 5, tsc.Count())
	tsc.Track()
	tsc.Track()
	tsc.Track()
	tsc.Track()
	assert.Equal(t, 9, tsc.Count())

	// t=60: 9 entries in last hour
	mc.IncTime(time.Minute)
	assert.Equal(t, 9, tsc.Count())

	// t=61: 6 entries in last hour
	mc.IncTime(time.Minute)
	assert.Equal(t, 6, tsc.Count())
	tsc.Track()

	// t=81: 5 entries in last hour
	mc.IncTime(20 * time.Minute)
	assert.Equal(t, 5, tsc.Count())

	mc.IncTime(24 * time.Hour)
	assert.Equal(t, 0, tsc.Count())
}

func TestTimeSeriesCounter_FractionalIntervals(t *testing.T) {
	mc := MockClock{}
	assert.Panics(t, func() { NewTimeSeriesCounter(&mc, 10*time.Minute, 3*time.Minute) })
}
