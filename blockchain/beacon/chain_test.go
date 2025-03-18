package beacon

import (
	"testing"

	"github.com/prysmaticlabs/prysm/v5/beacon-chain/core/transition"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/prysmaticlabs/prysm/v5/testing/util"
	"github.com/prysmaticlabs/prysm/v5/time/slots"
	"github.com/stretchr/testify/require"
)

func TestChain(t *testing.T) {
	st, err := transition.EmptyGenesisState()
	require.NoError(t, err)

	c, err := NewChain(st, 3)
	require.NoError(t, err)

	emptyStatus := c.status

	assertCheckpoints := func(t *testing.T, finalized, justified, checkpoint *checkpoint) {
		require.EqualValues(t, finalized, c.finalizedCheckpoint)
		require.EqualValues(t, justified, c.justifiedCheckpoint)
		require.EqualValues(t, checkpoint, c.checkpoint)
	}

	assertStatusNotChanged := func(t *testing.T) {
		require.Equal(t, emptyStatus, c.status)
	}

	// Genesis block

	b, root := block(t, 0)
	err = c.AddBlock(b)
	require.NoError(t, err)

	// Epoch 0

	b, _ = block(t, 1)
	err = c.AddBlock(b)
	require.NoError(t, err)

	assertCheckpoints(t, nil, nil, nil)
	assertStatusNotChanged(t)

	b, _ = block(t, 2)
	err = c.AddBlock(b)
	require.NoError(t, err)

	assertCheckpoints(t, nil, nil, nil)
	assertStatusNotChanged(t)

	// Epoch 1

	b, root = block(t, 3)
	err = c.AddBlock(b)
	require.NoError(t, err)

	ch := &checkpoint{
		root: root,
		slot: 3,
	}

	assertCheckpoints(t, nil, nil, ch)
	assertStatusNotChanged(t)

	b, _ = block(t, 4)
	err = c.AddBlock(b)
	require.NoError(t, err)

	assertCheckpoints(t, nil, nil, ch)

	b, _ = block(t, 5)
	err = c.AddBlock(b)
	require.NoError(t, err)

	assertCheckpoints(t, nil, nil, ch)

	// Epoch 2

	b, root = block(t, 6)
	err = c.AddBlock(b)
	require.NoError(t, err)

	justified := ch
	ch = &checkpoint{
		root: root,
		slot: 6,
	}

	assertCheckpoints(t, nil, justified, ch)
	assertStatusNotChanged(t)

	b, _ = block(t, 7)
	err = c.AddBlock(b)
	require.NoError(t, err)

	assertCheckpoints(t, nil, justified, ch)
	assertStatusNotChanged(t)

	b, _ = block(t, 8)
	err = c.AddBlock(b)
	require.NoError(t, err)

	assertCheckpoints(t, nil, justified, ch)
	assertStatusNotChanged(t)

	// Epoch 3

	b, root = block(t, 9)
	err = c.AddBlock(b)
	require.NoError(t, err)

	finalized := justified
	justified = ch
	ch = &checkpoint{
		root: root,
		slot: 9,
	}

	assertCheckpoints(t, finalized, justified, ch)
	require.EqualValues(t, &ethpb.Status{
		ForkDigest:     emptyStatus.ForkDigest,
		FinalizedRoot:  finalized.root,
		FinalizedEpoch: slots.ToEpoch(9),
		HeadRoot:       root,
		HeadSlot:       9,
	}, c.status)

	b, root = block(t, 10)
	err = c.AddBlock(b)
	require.NoError(t, err)

	assertCheckpoints(t, finalized, justified, ch)
	require.EqualValues(t, &ethpb.Status{
		ForkDigest:     emptyStatus.ForkDigest,
		FinalizedRoot:  finalized.root,
		FinalizedEpoch: slots.ToEpoch(10),
		HeadRoot:       root,
		HeadSlot:       10,
	}, c.status)
}

func block(t *testing.T, i uint64) (interfaces.ReadOnlySignedBeaconBlock, []byte) {
	denebBlock := util.HydrateSignedBeaconBlockDeneb(&ethpb.SignedBeaconBlockDeneb{
		Block: &ethpb.BeaconBlockDeneb{
			Slot: primitives.Slot(i),
		},
	})

	blk, err := blocks.NewSignedBeaconBlock(denebBlock)
	require.NoError(t, err)

	root, err := denebBlock.Block.HashTreeRoot()
	require.NoError(t, err)

	return blk, root[:]
}
