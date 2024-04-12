package beacon

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/prysmaticlabs/prysm/v5/config/params"
	prysmTypes "github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
)

const cleaningFrequency = 12 * time.Second
const ignoreSlot = 5

// BlobCacheValue is a struct that holds the slot and the channel for the blob sidecar
type BlobCacheValue struct {
	slot      prysmTypes.Slot
	ch        chan *ethpb.BlobSidecar
	closeLock sync.RWMutex
}

// BlobSidecarCacheManager manages the blob sidecar subscription
type BlobSidecarCacheManager struct {
	blobSidecars      *syncmap.SyncMap[string, *BlobCacheValue]
	beaconGenesisTime uint64
	log               *log.Entry
}

// AddBlobSidecar adds a blob sidecar to the cache
func (m *BlobSidecarCacheManager) AddBlobSidecar(blobSidecar *ethpb.BlobSidecar) error {
	blockHash, err := blobSidecar.SignedBlockHeader.Header.HashTreeRoot()
	if err != nil {
		return fmt.Errorf("failed to calculate block hash: %v", err)
	}

	m.log.Tracef("adding blob sidecar to cache, slot %v, block hash: %s index: %d", blobSidecar.SignedBlockHeader.Header.Slot, hex.EncodeToString(blockHash[:]), blobSidecar.Index)

	value, _ := m.blobSidecars.LoadOrStore(hex.EncodeToString(blockHash[:]), &BlobCacheValue{
		slot: blobSidecar.SignedBlockHeader.Header.Slot,
		// BlobsidecarSubnetCount is the number of subnets that the blob sidecar will be sent to
		// currently it's equal to the max number of blobs inside a block
		ch: make(chan *ethpb.BlobSidecar, params.BeaconConfig().BlobsidecarSubnetCount),
	})

	value.closeLock.RLock()
	defer value.closeLock.RUnlock()

	if value.ch == nil {
		return fmt.Errorf("blob sidecar channel is closed for block hash %s", blockHash)
	}

	select {
	case value.ch <- blobSidecar:
	default:
		return fmt.Errorf("blob sidecar channel is full for block hash %s", blockHash)
	}

	return nil
}

// SubscribeToBlobByBlockHash subscribes to a blob by block hash
func (m *BlobSidecarCacheManager) SubscribeToBlobByBlockHash(blockHash string, slot prysmTypes.Slot) chan *ethpb.BlobSidecar {
	blockHash = strings.TrimPrefix(blockHash, "0x")
	value, _ := m.blobSidecars.LoadOrStore(blockHash, &BlobCacheValue{
		slot: slot,
		// BlobsidecarSubnetCount is the number of subnets that the blob sidecar will be sent to
		// currently it's equal to the max number of blobs inside a block
		ch: make(chan *ethpb.BlobSidecar, params.BeaconConfig().BlobsidecarSubnetCount),
	})

	value.closeLock.RLock()
	defer value.closeLock.RUnlock()

	return value.ch
}

// UnsubscribeFromBlobByBlockHash unsubscribes from a blob by block hash
func (m *BlobSidecarCacheManager) UnsubscribeFromBlobByBlockHash(blockHash string) {
	value, ok := m.blobSidecars.Load(blockHash)
	if !ok {
		return
	}

	value.closeLock.Lock()
	defer value.closeLock.Unlock()

	if value.ch != nil {
		close(value.ch)
		value.ch = nil
	}
	m.blobSidecars.Delete(blockHash)
}

func (m *BlobSidecarCacheManager) isOldBlobSidecar(slot prysmTypes.Slot) bool {
	return currentSlot(m.beaconGenesisTime)-ignoreSlot >= slot
}

// CleaningRoutine cleans the old blob sidecars
func (m *BlobSidecarCacheManager) CleaningRoutine() {
	ticker := time.NewTicker(cleaningFrequency)

	for range ticker.C {
		cleaned := 0
		m.blobSidecars.Range(func(key string, value *BlobCacheValue) bool {
			if m.isOldBlobSidecar(value.slot) {
				m.log.Tracef("Cleaning old blob sidecars for block hash: %s, slot %d", key, value.slot)
				m.UnsubscribeFromBlobByBlockHash(key)
				cleaned++
			}
			return true
		})
		m.log.Tracef("Cleaning old blob sidecars, size: %d, cleaned: %v", m.blobSidecars.Size(), cleaned)
	}
}

// NewBlobSidecarCacheManager creates a new blob sidecar cache manager
func NewBlobSidecarCacheManager(beaconGenesisTime uint64) *BlobSidecarCacheManager {
	logCtx := log.WithField("component", "blob-sidecar-cache")

	m := &BlobSidecarCacheManager{
		blobSidecars:      syncmap.NewStringMapOf[*BlobCacheValue](),
		beaconGenesisTime: beaconGenesisTime,
		log:               logCtx,
	}

	go m.CleaningRoutine()
	return m
}
