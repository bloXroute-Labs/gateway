package beacon

import (
	"sync"
)

// APISharedSync represents all the shared resource api need to detect blocks and blobs duplication
type APISharedSync struct {
	lastSlot      uint64
	lastSlotMutex sync.Mutex
}

// NewAPISharedSync create a new APISharedSync object
func NewAPISharedSync() *APISharedSync {
	return &APISharedSync{lastSlotMutex: sync.Mutex{}}
}

func (a *APISharedSync) isKnownSlot(slot uint64, setSlot bool) bool {
	a.lastSlotMutex.Lock()
	defer a.lastSlotMutex.Unlock()
	if slot <= a.lastSlot {
		return true
	}
	if setSlot {
		a.lastSlot = slot
	}
	return false
}
