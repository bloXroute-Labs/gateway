package beacon

import (
	"context"
	"fmt"
	"sync"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/blocks"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	ethpb "github.com/OffchainLabs/prysm/v6/proto/prysm/v1alpha1"
	"github.com/OffchainLabs/prysm/v6/time/slots"
)

type checkpoint struct {
	root []byte
	slot uint64
}

// Chain represents a beacon chain
type Chain struct {
	genesisState        state.BeaconState
	slotsPerEpoch       uint64
	justifiedCheckpoint *checkpoint
	finalizedCheckpoint *checkpoint
	checkpoint          *checkpoint
	status              *ethpb.Status

	lock *sync.RWMutex
}

// NewChain creates a new beacon chain
func NewChain(genesisState state.BeaconState, slotsPerEpoch uint64) (*Chain, error) {
	m := &Chain{
		genesisState:  genesisState,
		slotsPerEpoch: slotsPerEpoch,
		lock:          &sync.RWMutex{},
	}

	if err := m.initStatus(); err != nil {
		return nil, fmt.Errorf("could not initialize status: %v", err)
	}

	return m, nil
}

// AddBlock adds a block to the chain
func (c *Chain) AddBlock(block interfaces.ReadOnlySignedBeaconBlock) error {
	// need to make sure checkpoints and status updated atomically
	c.lock.Lock()
	defer c.lock.Unlock()

	if block.Block().Slot() <= c.status.HeadSlot {
		return nil
	}

	digest, err := currentForkDigest(c.genesisState)
	if err != nil {
		return fmt.Errorf("could not get current fork digest: %v", err)
	}

	root, err := block.Block().HashTreeRoot()
	if err != nil {
		return fmt.Errorf("could not get block hash tree root: %v", err)
	}

	// start of the epoch
	blockSlot := block.Block().Slot()
	isEpochStart := uint64(blockSlot)%c.slotsPerEpoch == 0
	if isEpochStart {
		if c.justifiedCheckpoint != nil && (c.finalizedCheckpoint == nil || c.justifiedCheckpoint.slot-c.finalizedCheckpoint.slot == c.slotsPerEpoch) {
			c.finalizedCheckpoint = c.justifiedCheckpoint

			c.status = &ethpb.Status{
				ForkDigest:     digest[:],
				FinalizedRoot:  c.finalizedCheckpoint.root,
				FinalizedEpoch: slots.ToEpoch(primitives.Slot(c.finalizedCheckpoint.slot)),
				HeadRoot:       root[:],
				HeadSlot:       blockSlot,
			}
		}

		if c.checkpoint != nil && (c.justifiedCheckpoint == nil || c.checkpoint.slot-c.justifiedCheckpoint.slot == c.slotsPerEpoch) {
			c.justifiedCheckpoint = c.checkpoint
		}

		c.checkpoint = &checkpoint{
			root: root[:],
			slot: uint64(blockSlot),
		}
	} else if c.finalizedCheckpoint != nil && uint64(blockSlot)-c.finalizedCheckpoint.slot > c.slotsPerEpoch*2 && uint64(blockSlot)-c.finalizedCheckpoint.slot < c.slotsPerEpoch*3 {
		// Just update status if same epoch yet
		c.status = &ethpb.Status{
			ForkDigest:     digest[:],
			FinalizedRoot:  c.finalizedCheckpoint.root,
			FinalizedEpoch: slots.ToEpoch(primitives.Slot(c.finalizedCheckpoint.slot)),
			HeadRoot:       root[:],
			HeadSlot:       blockSlot,
		}
	}

	return nil
}

// Status returns the current status of the chain
func (c *Chain) Status(peer *peer) *ethpb.Status {
	c.lock.RLock()
	defer c.lock.RUnlock()

	// Take the peer's status if it's ahead of our own
	if peerStatus := peer.getStatus(); peerStatus != nil && peerStatus.GetHeadSlot() > c.status.GetHeadSlot() {
		return peerStatus
	}

	return c.status
}

func (c *Chain) initStatus() error {
	c.lock.Lock()
	defer c.lock.Unlock()

	digest, err := currentForkDigest(c.genesisState)
	if err != nil {
		return err
	}

	stateRoot, err := c.genesisState.HashTreeRoot(context.Background())
	if err != nil {
		return err
	}
	genesisBlk := blocks.NewGenesisBlock(stateRoot[:])
	genesisBlkRoot, err := genesisBlk.Block.HashTreeRoot()
	if err != nil {
		return err
	}

	c.status = &ethpb.Status{
		ForkDigest:     digest[:],
		FinalizedRoot:  params.BeaconConfig().ZeroHash[:],
		FinalizedEpoch: 0,
		HeadRoot:       genesisBlkRoot[:],
		HeadSlot:       0,
	}

	return nil
}
