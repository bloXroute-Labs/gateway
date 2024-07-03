package logger

import (
	"io"
	"sync"

	"github.com/rs/zerolog"
	"github.com/rs/zerolog/log"
)

// Discard creates an entry for testing
func Discard() *Entry {
	return &Entry{ee: zerolog.New(io.Discard)}
}

// GlobalTest is a global logger for testing
type GlobalTest struct {
	mu      sync.RWMutex
	Entries []TestEntry
}

// NewGlobal creates a new global logger for testing
func NewGlobal() *GlobalTest {
	gt := &GlobalTest{}

	w := newWriter(io.Discard, true)
	w.FormatPrepare = func(m map[string]interface{}) error {
		gt.addEntry(TestEntry{Message: m["message"].(string)})
		return nil
	}

	lw := &levelWriter{
		WriteCloser: w,
		minLevel:    zerolog.TraceLevel,
		maxLevel:    zerolog.FatalLevel,
		systemLevel: zerolog.TraceLevel,
	}

	// Lock configuration to avoid race conditions
	initConfigMutex.Lock()
	defer initConfigMutex.Unlock()

	log.Logger = log.Output(lw)

	return gt
}

// TestEntry is a test entry
type TestEntry struct {
	Message string
}

func (e *GlobalTest) addEntry(entry TestEntry) {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Entries = append(e.Entries, entry)
}

// LastEntry returns the last entry that was logged or nil.
func (e *GlobalTest) LastEntry() *TestEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	i := len(e.Entries) - 1
	if i < 0 {
		return nil
	}
	return &e.Entries[i]
}

// AllEntries returns all entries that were logged.
func (e *GlobalTest) AllEntries() []TestEntry {
	e.mu.RLock()
	defer e.mu.RUnlock()
	// Make a copy so the returned value won't race with future log requests
	entries := make([]TestEntry, len(e.Entries))
	for i := 0; i < len(e.Entries); i++ {
		// Make a copy, for safety
		entries[i] = e.Entries[i]
	}
	return entries
}

// Reset removes all entries from the logger.
func (e *GlobalTest) Reset() {
	e.mu.Lock()
	defer e.mu.Unlock()
	e.Entries = nil
}
