package utils

import (
	"sync"
	"time"
)

type entry struct {
	counter int
	ts      time.Time
}

// TimeSeriesCounter tracks the number of occurrences of an event in the last amount of time with the provided fidelity.
// For example, with duration=60m and fidelity=1m, TimeSeriesCounter will guarantee a count of the number of events
// that happened between 60-61m ago.
type TimeSeriesCounter struct {
	mu           sync.RWMutex
	clock        Clock
	duration     time.Duration
	fidelity     time.Duration
	entriesCount int
	entries      []entry
	total        int
}

// NewTimeSeriesCounter initializes a new time series tracker for the previous duration with the provided fidelity.
func NewTimeSeriesCounter(clock Clock, duration time.Duration, fidelity time.Duration) *TimeSeriesCounter {
	entriesCount := duration / fidelity
	if entriesCount*fidelity != duration {
		panic("cannot create a time series counter with a fractional entry count")
	}

	entries := make([]entry, 0, entriesCount)
	entries = append(entries, entry{0, clock.Now()})

	return &TimeSeriesCounter{
		clock:        clock,
		duration:     duration,
		fidelity:     fidelity,
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
	emptyIntervalCount := int(elapsed / ts.fidelity)

	for i := 1; i <= emptyIntervalCount; i++ {
		ts.entries = append(ts.entries, entry{0, lastEntry.ts.Add(time.Duration(i) * ts.fidelity)})
	}

	// check if need to add one more bucket for rounding
	lastEntry, _ = ts.lastEntry()
	if now.Sub(lastEntry.ts) > ts.fidelity {
		ts.entries = append(ts.entries, entry{0, lastEntry.ts.Add(ts.fidelity)})
	}
}

func (ts *TimeSeriesCounter) lastEntry() (*entry, bool) {
	if len(ts.entries) == 0 {
		return nil, false
	}
	return &ts.entries[len(ts.entries)-1], true
}
