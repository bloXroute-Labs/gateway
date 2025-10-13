package services

import (
	"fmt"
	"sync"
	"time"

	log "github.com/bloXroute-Labs/bxcommon-go/logger"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const defaultSenderStoreCapacity = 1000

// SenderExtractor receives EthTransaction pointers and extracts their senders in a background goroutine
type SenderExtractor struct {
	ch      chan *types.EthTransaction
	senders *boundedSenderStore
}

type senderEntry struct {
	sender    types.Sender
	timestamp time.Time
}

// boundedSenderStore is a simple mutex-protected map + circular slice of keys
// that evicts the oldest entry when capacity is reached.
type boundedSenderStore struct {
	mu       sync.RWMutex
	data     map[types.SHA256Hash]senderEntry
	keys     []types.SHA256Hash
	writeIdx int
	size     int
}

func newBoundedSenderStore() *boundedSenderStore {
	return &boundedSenderStore{
		data: make(map[types.SHA256Hash]senderEntry, defaultSenderStoreCapacity),
		keys: make([]types.SHA256Hash, defaultSenderStoreCapacity),
	}
}

func (b *boundedSenderStore) Store(key types.SHA256Hash, val senderEntry) {
	b.mu.Lock()
	defer b.mu.Unlock()

	if _, ok := b.data[key]; ok {
		return
	}

	if b.size < defaultSenderStoreCapacity {
		b.data[key] = val
		b.keys[b.writeIdx] = key
		b.writeIdx = (b.writeIdx + 1) % defaultSenderStoreCapacity
		b.size++
		return
	}

	// full: evict oldest at writeIdx
	oldest := b.keys[b.writeIdx]
	delete(b.data, oldest)
	b.keys[b.writeIdx] = key
	b.data[key] = val
	b.writeIdx = (b.writeIdx + 1) % defaultSenderStoreCapacity
}

func (b *boundedSenderStore) Load(key types.SHA256Hash) (senderEntry, bool) {
	b.mu.RLock()
	defer b.mu.RUnlock()
	v, ok := b.data[key]
	return v, ok
}

func (b *boundedSenderStore) Size() int {
	b.mu.RLock()
	defer b.mu.RUnlock()
	return b.size
}

// NewSenderExtractor creates a new SenderExtractor
func NewSenderExtractor() *SenderExtractor {
	s := &SenderExtractor{
		ch:      make(chan *types.EthTransaction, 1000),
		senders: newBoundedSenderStore(),
	}
	return s
}

// Run runs the SenderExtractor
func (s *SenderExtractor) Run() {
	for ethTx := range s.ch {
		if ethTx == nil {
			continue
		}
		if sender, err := ethTx.Sender(); err == nil {
			h := ethTx.Hash()
			s.senders.Store(h, senderEntry{sender: sender, timestamp: time.Now()})
		} else {
			log.Errorf("failed to extract sender for ethTx: %v", ethTx.Hash().String())
		}
	}
}

func (s *SenderExtractor) submitEth(ethTx *types.EthTransaction) {
	select {
	case s.ch <- ethTx:
	default:
	}

}

// GetSender returns the extracted sender for the given tx hash if available.
func (s *SenderExtractor) GetSender(hash types.SHA256Hash) (types.Sender, bool) {
	v, ok := s.senders.Load(hash)
	if !ok {
		return types.EmptySender, false
	}
	return v.sender, true
}

// GetSendersFromBlockTxs returns the senders for the transactions in the block
func (s *SenderExtractor) GetSendersFromBlockTxs(block *common.Block) map[string]types.Sender {
	var foundSender, notFoundSender int
	senders := make(map[string]types.Sender)
	txs := block.Transactions()
	for _, tx := range txs {
		sender, ok := s.GetSender(types.SHA256Hash(tx.Hash()))
		if ok {
			senders[tx.Hash().String()] = sender
			foundSender++
		} else {
			notFoundSender++
		}
	}
	if log.IsLevelEnabled(log.DebugLevel) {
		total := foundSender + notFoundSender
		pctStr := "0%"
		if total > 0 {
			pct := (float64(foundSender) / float64(total)) * 100
			pctStr = fmt.Sprintf("%.0f%%", pct)
		}
		log.Debugf("found %s of senders for block %v", pctStr, block.Number())
	}
	return senders
}
