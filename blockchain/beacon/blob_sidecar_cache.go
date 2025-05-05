package beacon

import (
	"encoding/hex"
	"fmt"
	"strings"
	"sync"
	"time"

	prysmTypes "github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	gethparams "github.com/ethereum/go-ethereum/params"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
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

	blockHashStr := hex.EncodeToString(blockHash[:])

	m.log.Tracef("adding blob sidecar to cache, slot %v, block hash: %s index: %d", blobSidecar.SignedBlockHeader.Header.Slot, blockHashStr, blobSidecar.Index)

	value, _ := m.blobSidecars.LoadOrStore(blockHashStr, &BlobCacheValue{
		slot: blobSidecar.SignedBlockHeader.Header.Slot,
		ch:   make(chan *ethpb.BlobSidecar, gethparams.DefaultPragueBlobConfig.Max),
	})

	value.closeLock.RLock()
	defer value.closeLock.RUnlock()

	if value.ch == nil {
		return fmt.Errorf("blob sidecar channel is closed for block hash %s", blockHashStr)
	}

	select {
	case value.ch <- blobSidecar:
	default:
		return fmt.Errorf("blob sidecar channel is full for block hash %s", blockHashStr)
	}

	return nil
}

// SubscribeToBlobByBlockHash subscribes to a blob by block hash
func (m *BlobSidecarCacheManager) SubscribeToBlobByBlockHash(blockHash string, slot prysmTypes.Slot) chan *ethpb.BlobSidecar {
	blockHash = strings.TrimPrefix(blockHash, "0x")
	value, _ := m.blobSidecars.LoadOrStore(blockHash, &BlobCacheValue{
		slot: slot,
		ch:   make(chan *ethpb.BlobSidecar, gethparams.DefaultPragueBlobConfig.Max),
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
