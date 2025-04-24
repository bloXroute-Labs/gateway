package utils

import (
	"sync"
	"time"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
)

type entry struct {
	counter int
	ts      time.Time
}

// TimeSeriesCounter tracks the number of occurrences of an event in the
// last duration of time with the provided timeFrame by keeping last
// duration/timeFrame entries. Each entry is just an infinite counter
// of the occurring event. Entry is being incremented during the timeFrame
// period of time, after the timeFrame passes it appends a new entry
// which counter is incremented.
//
// For example:
// With duration of 5m and timeFrame of 1m TimeSeriesCounter keeps
// 5m/1m = 5 (duration/timeFrame) entries.
//
// It starts from creating 1 entry which incremented by calling Track() func:
// |___|
//
// Which eventually gets to the point where max (5) entries exist:
// |_7_|_2_|_9_|_4_|_7_|
// At this point Count() is equal to 7+2+9+4+7 = 29
//
// After the time overflows the duration of 5m it removes the oldest entry and
// appends a new one which counter is now being incremented during the next
// timerFrame (1m):
// |_2_|_9_|_4_|_7_|___|
// At this point Count() is equal to 2+9+4+7+0 = 22
//
// This means that there is a point in time were two subsequent calls to Count()
// with very small delay return different results (29 and 22). So the smaller
// timerFrame relatively to duration the more entries it creates and the
// more precise results with less deviation it provides.
type TimeSeriesCounter struct {
	mu           sync.RWMutex
	clock        clock.Clock
	duration     time.Duration
	timeFrame    time.Duration
	entriesCount int
	entries      []entry
	total        int
}

// NewTimeSeriesCounter initializes a new time series tracker for the previous duration with the provided fidelity.
func NewTimeSeriesCounter(clock clock.Clock, duration time.Duration, timeFrame time.Duration) *TimeSeriesCounter {
	entriesCount := duration / timeFrame
	if entriesCount*timeFrame != duration {
		panic("cannot create a time series counter with a fractional entry count")
	}

	entries := make([]entry, 0, entriesCount)
	entries = append(entries, entry{0, clock.Now()})

	return &TimeSeriesCounter{
		clock:        clock,
		duration:     duration,
		timeFrame:    timeFrame,
		entriesCount: int(entriesCount),
		entries:      entries,
		total:        0,
	}
}

// Track counts one occurrence of an event.
func (ts *TimeSeriesCounter) Track() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	now := ts.clock.Now()
	ts.pruneFront(now)
	ts.fillSpaces(now)

	lastEntry, _ := ts.lastEntry()
	lastEntry.counter++

	ts.total++
}

// Count return the total occurrences in the last interval.
func (ts *TimeSeriesCounter) Count() int {
	ts.mu.RLock()
	defer ts.mu.RUnlock()

	now := ts.clock.Now()
	ts.pruneFront(now)
	return ts.total
}

// Recompute forces revaluation of the tracked total. Typically, this property is cached so the cost of calculating the total is amortized over the lifetime of each Track call. Recompute is usually not necessary.
func (ts *TimeSeriesCounter) Recompute() {
	ts.mu.Lock()
	defer ts.mu.Unlock()

	ts.pruneFront(ts.clock.Now())
	total := 0
	for _, e := range ts.entries {
		total += e.counter
	}
	ts.total = total
}

func (ts *TimeSeriesCounter) pruneFront(now time.Time) {
	if len(ts.entries) == 0 {
		return
	}

	// prune until the latest valid entry in the list
	for i, e := range ts.entries {
		if now.Sub(e.ts) <= ts.duration {
			ts.entries = ts.entries[i:]
			break
		} else {
			ts.total -= e.counter
		}
	}

	// check final entry: if needed, prune the entire list
	lastEntry, ok := ts.lastEntry()
	if !ok {
		return
	}
	if now.Sub(lastEntry.ts) > ts.duration {
		ts.entries = make([]entry, 0)
		ts.total = 0
		return
	}
}

func (ts *TimeSeriesCounter) fillSpaces(now time.Time) {
	lastEntry, ok := ts.lastEntry()

	// if entries is empty, then add an entry for the latest interval
	if !ok {
		ts.entries = append(ts.entries, entry{0, now})
		return
	}

	elapsed := now.Sub(lastEntry.ts)
	emptyIntervalCount := int(elapsed / ts.timeFrame)

	for i := 1; i <= emptyIntervalCount; i++ {
		ts.entries = append(ts.entries, entry{0, lastEntry.ts.Add(time.Duration(i) * ts.timeFrame)})
	}

	// check if need to add one more bucket for rounding
	lastEntry, _ = ts.lastEntry()
	if now.Sub(lastEntry.ts) > ts.timeFrame {
		ts.entries = append(ts.entries, entry{0, lastEntry.ts.Add(ts.timeFrame)})
	}
}

func (ts *TimeSeriesCounter) lastEntry() (*entry, bool) {
	if len(ts.entries) == 0 {
		return nil, false
	}
	return &ts.entries[len(ts.entries)-1], true
}
