package beacon

import (
	"fmt"
	"sync"
)

// APISharedSync represents all the shared resource api need to detect blocks and blobs duplication
type APISharedSync struct {
	lastSlot                         uint64
	lastBlobsByBeaconHashSuccessSlot uint64
	lastBlobsMutex                   sync.Mutex
	lastSlotMutex                    sync.Mutex
	processedBlobIndexesBitfield     uint64
}

// NewAPISharedSync create a new APISharedSync object
func NewAPISharedSync() *APISharedSync {
	return &APISharedSync{lastSlotMutex: sync.Mutex{}, lastBlobsMutex: sync.Mutex{}}
}

func (a *APISharedSync) setProcessedBlobIndex(index uint64) error {
	if index > 63 {
		return fmt.Errorf("blob sidecar index %d is out of range", index)
	}
	a.processedBlobIndexesBitfield = a.processedBlobIndexesBitfield | (1 << index)
	return nil
}

func (a *APISharedSync) isProcessedBlobIndex(index uint64) bool {
	return a.processedBlobIndexesBitfield&(1<<index) != 0
}

func (a *APISharedSync) resetProcessedBlobIndexes() {
	a.processedBlobIndexesBitfield = 0
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

func (a *APISharedSync) needToRequestBlobSidecar(slot uint64, index uint64) (bool, error) {
	a.lastBlobsMutex.Lock()
	defer a.lastBlobsMutex.Unlock()

	if a.lastBlobsByBeaconHashSuccessSlot > slot {
		return false, nil
	}

	if a.lastBlobsByBeaconHashSuccessSlot < slot {
		// reset processed blob indexes for each new slot
		a.resetProcessedBlobIndexes()
		a.lastBlobsByBeaconHashSuccessSlot = slot
	}

	// if the blob sidecar index is already processed, ignore it
	if a.isProcessedBlobIndex(index) {
		return false, nil
	}

	return true, a.setProcessedBlobIndex(index)
}
