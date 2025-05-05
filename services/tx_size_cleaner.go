package services

import (
	"sort"
	"sync/atomic"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/bxcommon-go/syncmap"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// TxSizeCleaner is a utility that keeps track of the total size of transactions that are currently in the mempool.
type TxSizeCleaner struct {
	txHashMap *syncmap.SyncMap[string, *txSizeCleanerEntry]

	name         string
	totalSize    atomic.Uint64
	totalMaxSize uint64
	cleanSize    uint64
	removeFunc   func(hashes *types.SHA256HashList)
	cleanChan    chan struct{}
}

// NewTxSizeCleaner creates a new TxSizeCleaner.
func NewTxSizeCleaner(name string, totalMaxSize uint64, cleanCoef float64, removeFunc func(hashes *types.SHA256HashList)) *TxSizeCleaner {
	return &TxSizeCleaner{
		name:         name,
		txHashMap:    syncmap.NewStringMapOf[*txSizeCleanerEntry](),
		totalMaxSize: totalMaxSize,
		cleanSize:    uint64(float64(totalMaxSize) * cleanCoef),
		removeFunc:   removeFunc,
		cleanChan:    make(chan struct{}, 1),
	}
}

type txSizeCleanerEntry struct {
	hash    string
	size    int
	addTime time.Time
}

// AddTx adds a transaction to the cleaner.
func (c *TxSizeCleaner) AddTx(hash string, size int, addTime time.Time) {
	_, exists := c.txHashMap.LoadOrStore(hash, &txSizeCleanerEntry{
		hash:    hash,
		size:    size,
		addTime: addTime,
	})
	if exists {
		return
	}

	c.totalSize.Add(uint64(size))

	c.cleanIfNeeded()
}

// Remove removes a transaction from the cleaner.
func (c *TxSizeCleaner) Remove(hash string) {
	if e, exists := c.txHashMap.LoadAndDelete(hash); exists {
		c.totalSize.Add(-uint64(e.size))
	}
}

func (c *TxSizeCleaner) cleanIfNeeded() {
	if c.totalSize.Load() < c.totalMaxSize {
		return
	}

	select {
	case c.cleanChan <- struct{}{}:
	default:
		return
	}

	startTime := time.Now()

	sizeToClean := c.totalSize.Load() - c.cleanSize

	txs := make([]*txSizeCleanerEntry, 0, c.txHashMap.Size())
	c.txHashMap.Range(func(_ string, e *txSizeCleanerEntry) bool {
		txs = append(txs, e)

		return true
	})

	sort.Slice(txs, func(i, j int) bool {
		return txs[i].addTime.Before(txs[j].addTime)
	})

	sizeCleaned := uint64(0)
	txHashesToRemove := make(types.SHA256HashList, 0)
	for _, tx := range txs {
		sizeCleaned += uint64(tx.size)
		txHashesToRemove = append(txHashesToRemove, types.SHA256Hash([]byte(tx.hash)))
		if sizeCleaned >= sizeToClean {
			break
		}
	}

	c.removeFunc(&txHashesToRemove)

	<-c.cleanChan

	log.Debugf("%s cleaned %d(%d bytes) entries in %s", c.name, len(txHashesToRemove), sizeCleaned, time.Since(startTime))
}
