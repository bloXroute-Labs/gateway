package bscmev

import (
	"context"
	"math/big"
	"time"

	"go.uber.org/atomic"

	"github.com/bloXroute-Labs/gateway/v2/services"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/bloXroute-Labs/gateway/v2/utils/ptr"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

type blockLifecycle uint8

const (
	startOfBlock blockLifecycle = iota
	middleOfBlock
	endOfBlock
)

const (
	txBacklog    = 500
	blockBacklog = 100

	defaultBlockDuration = 3 * time.Second
)

// ProviderGwState is the delayer for BSC MEV with a mode that determines how it behaves
type ProviderGwState struct {
	state              *atomic.Pointer[blockLifecycle]
	currentBlockHeight *atomic.Pointer[big.Int]
	pendingTxs         *syncmap.SyncMap[types.SHA256Hash, *services.TransactionData]

	blockChan               chan *big.Int
	pendingTxsToProcessChan chan *services.TransactionData

	// --- dependency ---
	clock               utils.Clock
	txSlotStartDuration time.Duration
	txSlotEndDuration   time.Duration

	// --- managed during Run() ---
	tillMiddleOfBlockTimer utils.Timer
	tillEndOfBlockTimer    utils.Timer
}

// NewProviderGwState creates a new ProviderGwState
func NewProviderGwState(clock utils.Clock, txSlotStartDuration time.Duration, txSlotEndDuration time.Duration) *ProviderGwState {
	return &ProviderGwState{
		state:              atomic.NewPointer(ptr.New(startOfBlock)),
		currentBlockHeight: atomic.NewPointer(big.NewInt(0)),
		pendingTxs:         syncmap.NewTypedMapOf[types.SHA256Hash, *services.TransactionData](syncmap.SHA256HashHasher),

		blockChan:               make(chan *big.Int, blockBacklog),
		pendingTxsToProcessChan: make(chan *services.TransactionData, txBacklog),

		// --- dependency ---
		clock:               clock,
		txSlotStartDuration: txSlotStartDuration,
		txSlotEndDuration:   txSlotEndDuration,
	}
}

// Run starts the ProviderGwState
func (b *ProviderGwState) Run(ctx context.Context) error {
	b.tillMiddleOfBlockTimer = b.clock.Timer(0)
	b.tillEndOfBlockTimer = b.clock.Timer(0)

	<-b.tillMiddleOfBlockTimer.Alert()
	<-b.tillEndOfBlockTimer.Alert()

	go func() {
		for {
			select {
			case <-ctx.Done():
				b.tillMiddleOfBlockTimer.Stop()
				b.tillEndOfBlockTimer.Stop()
				return
			case number := <-b.blockChan:
				b.onStartOfBlock(number)
			case <-b.tillMiddleOfBlockTimer.Alert():
				b.onMiddleOfBlock()
			case <-b.tillEndOfBlockTimer.Alert():
				b.onEndOfBlock()
			}
		}
	}()

	return nil
}

// DelayedTransactions returns a channel of transactions that was delayed and should be processed
func (b *ProviderGwState) DelayedTransactions() <-chan *services.TransactionData {
	return b.pendingTxsToProcessChan
}

// StoreTx stores a transaction in the pendingTxs map if needed and returns true if the transaction should be sent to the validator
//
//  1. (startOfBlock) newBlock - b.skipDelayDuration before block
//     If: transaction received by bsc_mev gw is bsc_mev then send it to validator
//     Else: ignore
//
//  2. (middleOfBlock) b.skipDelayDuration - (defaultBlockDuration-defaultEndUnaccessibleDuration) before block
//     If: transaction received by bsc_mev gw is bsc_mev then:
//     Remove the tx from PendingTxs map and send it to validator
//     Else: add it to PendingTxs map
//
//  3. (endOfBlock) (defaultBlockDuration-defaultEndUnaccessibleDuration) - newBlock
//     Send all transactions to validator
//     Clear PendingTxs after 3.0s
func (b *ProviderGwState) StoreTx(data *services.TransactionData) bool {
	switch *b.state.Load() {
	case startOfBlock:
		if data.Tx.Flags().IsMevBundleTx() {
			return true
		}
	case middleOfBlock:
		if data.Tx.Flags().IsMevBundleTx() {
			b.pendingTxs.Delete(data.Tx.Hash())
			return true
		}

		data.DelayStartTime = b.clock.Now()
		b.pendingTxs.Store(data.Tx.Hash(), data)
	case endOfBlock:
		return true
	}

	return false
}

// OnBlockReceived is called when a block is received from the network
// to change the state of the delayer to startOfBlock
func (b *ProviderGwState) OnBlockReceived(blockHeight *big.Int) { b.blockChan <- blockHeight }

func (b *ProviderGwState) onStartOfBlock(blockHeight *big.Int) {
	if blockHeight == nil || b.currentBlockHeight.Load().Cmp(blockHeight) == 0 {
		return
	}

	b.currentBlockHeight.Store(blockHeight)
	b.state.Store(ptr.New(startOfBlock))
	b.pendingTxs.Clear()
	b.tillMiddleOfBlockTimer.Reset(b.txSlotStartDuration)
	b.tillEndOfBlockTimer.Reset(defaultBlockDuration - b.txSlotEndDuration)
}

func (b *ProviderGwState) onMiddleOfBlock() {
	b.state.Store(ptr.New(middleOfBlock))
}

func (b *ProviderGwState) onEndOfBlock() {
	b.state.Store(ptr.New(endOfBlock))
	b.pendingTxs.Range(func(_ types.SHA256Hash, data *services.TransactionData) bool {
		b.pendingTxsToProcessChan <- data
		return true
	})
}
