package beacon

import (
	"context"
	"math/rand"
	"testing"
	"time"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/blocks"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	ethpb "github.com/prysmaticlabs/prysm/v3/proto/prysm/v1alpha1"
	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
)

func TestChain_AddBlock(t *testing.T) {
	c := newChain(context.Background(), 5, 5, time.Hour, 1000)

	block1 := newBeaconBlock(t, 1, nil)
	block2 := newBeaconBlock(t, 2, block1)
	block3a := newBeaconBlock(t, 3, block2)
	block3b := newBeaconBlock(t, 3, block2)
	block4a := newBeaconBlock(t, 4, block3a)
	block4b := newBeaconBlock(t, 4, block3b)
	block5a := newBeaconBlock(t, 5, block4a)
	block5b := newBeaconBlock(t, 5, block4b)
	block6 := newBeaconBlock(t, 6, block5b)

	newHeads := addBlock(t, c, block1)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block1, 0, 1)

	newHeads = addBlock(t, c, block2)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block2, 0, 2)
	assertChainState(t, c, block1, 1, 2)

	newHeads = addBlock(t, c, block3a)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block3a, 0, 3)
	assertChainState(t, c, block2, 1, 3)
	assertChainState(t, c, block1, 2, 3)

	newHeads = addBlock(t, c, block3b)
	assert.Equal(t, 0, newHeads)
	assertChainState(t, c, block3a, 0, 3)
	assertChainState(t, c, block2, 1, 3)
	assertChainState(t, c, block1, 2, 3)

	newHeads = addBlock(t, c, block4a)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block4a, 0, 4)
	assertChainState(t, c, block3a, 1, 4)
	assertChainState(t, c, block2, 2, 4)
	assertChainState(t, c, block1, 3, 4)

	newHeads = addBlock(t, c, block4b)
	assert.Equal(t, 0, newHeads)
	assertChainState(t, c, block4a, 0, 4)
	assertChainState(t, c, block3a, 1, 4)
	assertChainState(t, c, block2, 2, 4)
	assertChainState(t, c, block1, 3, 4)

	newHeads = addBlock(t, c, block5a)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block5a, 0, 5)
	assertChainState(t, c, block4a, 1, 5)
	assertChainState(t, c, block3a, 2, 5)
	assertChainState(t, c, block2, 3, 5)
	assertChainState(t, c, block1, 4, 5)

	newHeads = addBlock(t, c, block5b)
	assert.Equal(t, 0, newHeads)
	assertChainState(t, c, block5a, 0, 5)
	assertChainState(t, c, block4a, 1, 5)
	assertChainState(t, c, block3a, 2, 5)
	assertChainState(t, c, block2, 3, 5)
	assertChainState(t, c, block1, 4, 5)

	newHeads = addBlock(t, c, block6)
	assert.Equal(t, 4, newHeads)
	assertChainState(t, c, block6, 0, 6)
	assertChainState(t, c, block5b, 1, 6)
	assertChainState(t, c, block4b, 2, 6)
	assertChainState(t, c, block3b, 3, 6)
	assertChainState(t, c, block2, 4, 6)
	assertChainState(t, c, block1, 5, 6)
}

func TestChain_AddBlock_MissingBlocks(t *testing.T) {
	c := newChain(context.Background(), 5, 3, time.Hour, 1000)

	block1 := newBeaconBlock(t, 1, nil)
	block2 := newBeaconBlock(t, 2, block1)
	block3 := newBeaconBlock(t, 3, block2)
	block4a := newBeaconBlock(t, 4, block3)
	block4b := newBeaconBlock(t, 4, block3)
	block5 := newBeaconBlock(t, 5, block4b)
	block6 := newBeaconBlock(t, 6, block5)
	block7 := newBeaconBlock(t, 7, block6)

	addBlock(t, c, block1)

	newHeads := addBlock(t, c, block3)
	assert.Equal(t, 0, newHeads)
	assertChainState(t, c, block1, 0, 1)

	newHeads = addBlock(t, c, block2)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block2, 0, 2)
	assertChainState(t, c, block1, 1, 2)

	// found block 3 and filled it in
	newHeads = addBlock(t, c, block4a)
	assert.Equal(t, 2, newHeads)
	assertChainState(t, c, block4a, 0, 4)

	// add a bunch of entries that can't be added to chain (for some reason 4b is missing)
	newHeads = addBlock(t, c, block5)
	assert.Equal(t, 0, newHeads)
	newHeads = addBlock(t, c, block6)
	assert.Equal(t, 0, newHeads)

	// chain is long enough, don't care about 4b anymore
	newHeads = addBlock(t, c, block7)
	assert.Equal(t, 3, newHeads)
	assertChainState(t, c, block7, 0, 3)
	assertChainState(t, c, block6, 1, 3)
	assertChainState(t, c, block5, 2, 3)
}

func TestChain_AddBlock_LongFork(t *testing.T) {
	c := newChain(context.Background(), 2, 5, time.Hour, 1000)

	block1 := newBeaconBlock(t, 1, nil)
	block2a := newBeaconBlock(t, 2, block1)
	block2b := newBeaconBlock(t, 2, block1)
	block3a := newBeaconBlock(t, 3, block2a)
	block3b := newBeaconBlock(t, 3, block2b)
	block4a := newBeaconBlock(t, 4, block3a)
	block4b := newBeaconBlock(t, 4, block3b)
	block5a := newBeaconBlock(t, 5, block4a)
	block5b := newBeaconBlock(t, 5, block4b)
	block6 := newBeaconBlock(t, 6, block5b)

	addBlock(t, c, block1)
	addBlock(t, c, block2a)
	addBlock(t, c, block2b)
	addBlock(t, c, block3a)
	addBlock(t, c, block3b)
	addBlock(t, c, block4a)
	addBlock(t, c, block4b)
	addBlock(t, c, block5a)
	addBlock(t, c, block5b)

	assertChainState(t, c, block5a, 0, 5)
	assertChainState(t, c, block4a, 1, 5)
	assertChainState(t, c, block3a, 2, 5)
	assertChainState(t, c, block2a, 3, 5)
	assertChainState(t, c, block1, 4, 5)

	newHeads := addBlock(t, c, block6)
	assert.Equal(t, 3, newHeads)

	assertChainState(t, c, block6, 0, 3)
	assertChainState(t, c, block5b, 1, 3)
	assertChainState(t, c, block4b, 2, 3)
}

func TestChain_GetHeaders_ByNumber(t *testing.T) {
	c := NewChain(context.Background())

	// true chain: 1, 2, 3b, 4
	block1 := newBeaconBlock(t, 1, nil)
	block2 := newBeaconBlock(t, 2, block1)
	block3a := newBeaconBlock(t, 3, block2)
	block3b := newBeaconBlock(t, 3, block2)
	block4 := newBeaconBlock(t, 4, block3b)

	addBlock(t, c, block1)
	addBlock(t, c, block2)
	addBlock(t, c, block3a)
	addBlock(t, c, block3b)
	addBlock(t, c, block4)

	var (
		headers []*ethpb.SignedBeaconBlockHeader
		err     error
	)

	// expected: err (neither header or hash provided)
	headers, err = c.GetHeaders(eth.HashOrNumber{}, 1, 0, false)
	assert.Nil(t, headers)
	assert.Equal(t, ErrInvalidRequest, err)

	// expected: err (headers in future)
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 10}, 1, 0, false)
	assert.Nil(t, headers)
	assert.Equal(t, ErrFutureHeaders, err)

	// expected: 1
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, blockHeader(t, block1), headers[0])

	// fork point, expected: 3b
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 3}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, blockHeader(t, block3b), headers[0])

	// expected: 1, 2, 3b, 4
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 4, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, blockHeader(t, block1), headers[0])
	assert.Equal(t, blockHeader(t, block2), headers[1])
	assert.Equal(t, blockHeader(t, block3b), headers[2])
	assert.Equal(t, blockHeader(t, block4), headers[3])

	// expected: 1, 3b
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 2, 1, false)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, blockHeader(t, block1), headers[0])
	assert.Equal(t, blockHeader(t, block3b), headers[1])

	// expected: 4, 2
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 4}, 2, 1, true)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, blockHeader(t, block4), headers[0])
	assert.Equal(t, blockHeader(t, block2), headers[1])

	// expected: 1, 2, 3b, 4 (found all that was possible)
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 100, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, blockHeader(t, block1), headers[0])
	assert.Equal(t, blockHeader(t, block2), headers[1])
	assert.Equal(t, blockHeader(t, block3b), headers[2])
	assert.Equal(t, blockHeader(t, block4), headers[3])

	// expected: err (header couldn't be located at the requested height in the past, so most create error)
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 100, 0, true)
	assert.Nil(t, headers)
	assert.NotNil(t, err)
}

func TestChain_GetHeaders_ByHash(t *testing.T) {
	c := NewChain(context.Background())

	// true chain: 1, 2, 3b, 4
	block1 := newBeaconBlock(t, 1, nil)
	block2 := newBeaconBlock(t, 2, block1)
	block3a := newBeaconBlock(t, 3, block2)
	block3b := newBeaconBlock(t, 3, block2)
	block4 := newBeaconBlock(t, 4, block3b)

	addBlock(t, c, block1)
	addBlock(t, c, block2)
	addBlock(t, c, block3a)
	addBlock(t, c, block3b)
	addBlock(t, c, block4)

	var (
		headers []*ethpb.SignedBeaconBlockHeader
		err     error
	)

	// expected: 1
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: blockHash(t, block1)}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, blockHeader(t, block1), headers[0])

	// fork point, expected: 3a
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: blockHash(t, block3a)}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, blockHeader(t, block3a), headers[0])

	// fork point, expected: 3b (even though it's not part of chain, still return it if requested)
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: blockHash(t, block3b)}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, blockHeader(t, block3b), headers[0])

	// expected: 1, 2, 3b, 4
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: blockHash(t, block1)}, 4, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, blockHeader(t, block1), headers[0])
	assert.Equal(t, blockHeader(t, block2), headers[1])
	assert.Equal(t, blockHeader(t, block3b), headers[2])
	assert.Equal(t, blockHeader(t, block4), headers[3])

	// expected: 1, 3b
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: blockHash(t, block1)}, 2, 1, false)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, blockHeader(t, block1), headers[0])
	assert.Equal(t, blockHeader(t, block3b), headers[1])

	// expected: 4, 2
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: blockHash(t, block4)}, 2, 1, true)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, blockHeader(t, block4), headers[0])
	assert.Equal(t, blockHeader(t, block2), headers[1])

	// expected: 1, 2, 3b, 4 (found all that was possible)
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: blockHash(t, block1)}, 100, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, blockHeader(t, block1), headers[0])
	assert.Equal(t, blockHeader(t, block2), headers[1])
	assert.Equal(t, blockHeader(t, block3b), headers[2])
	assert.Equal(t, blockHeader(t, block4), headers[3])
}

func TestChain_GetNewHeadsForBDN(t *testing.T) {
	c := newChain(context.Background(), 5, 5, time.Hour, 1000)

	block1 := newBeaconBlock(t, 1, nil)
	block2 := newBeaconBlock(t, 2, block1)
	block3 := newBeaconBlock(t, 3, block2)

	addBlock(t, c, block1)
	addBlock(t, c, block2)

	blocks, err := c.GetNewHeadsForBDN(2)
	assert.Nil(t, err)
	assert.Equal(t, blockHash(t, block2), blockHash(t, blocks[0]))
	assert.Equal(t, blockHash(t, block1), blockHash(t, blocks[1]))

	_, err = c.GetNewHeadsForBDN(3)
	assert.NotNil(t, err)

	c.MarkSentToBDN(blockHash(t, block2))
	addBlock(t, c, block3)

	blocks, err = c.GetNewHeadsForBDN(2)
	assert.Nil(t, err)
	assert.Equal(t, blockHash(t, block3), blockHash(t, blocks[0]))
}

func newBeaconBlock(t *testing.T, slot int, prevBlock interfaces.SignedBeaconBlock) interfaces.SignedBeaconBlock {
	// Blocks should not be the same
	randaoReveal := make([]byte, 96)
	rand.Read(randaoReveal)

	var parentRoot [32]byte
	if prevBlock != nil {
		var err error
		parentRoot, err = prevBlock.Block().HashTreeRoot()
		require.NoError(t, err)
	}

	block := &ethpb.SignedBeaconBlock{
		Block: &ethpb.BeaconBlock{
			Slot:       types.Slot(slot),
			ParentRoot: parentRoot[:],
			StateRoot:  make([]byte, 32),
			Body: &ethpb.BeaconBlockBody{
				RandaoReveal: randaoReveal,
				Eth1Data: &ethpb.Eth1Data{
					DepositRoot: make([]byte, 32),
					BlockHash:   make([]byte, 32),
				},
				Graffiti:          make([]byte, 32),
				Attestations:      []*ethpb.Attestation{},
				AttesterSlashings: []*ethpb.AttesterSlashing{},
				Deposits:          []*ethpb.Deposit{},
				ProposerSlashings: []*ethpb.ProposerSlashing{},
				VoluntaryExits:    []*ethpb.SignedVoluntaryExit{},
			},
		},
		Signature: make([]byte, 96),
	}

	blk, err := blocks.NewSignedBeaconBlock(block)
	assert.NoError(t, err)

	return blk
}

func addBlock(t *testing.T, c *Chain, block interfaces.SignedBeaconBlock) int {
	newHeads, err := c.AddBlock(block, BSBlockchain)
	assert.NoError(t, err)

	return newHeads
}

func assertChainState(t *testing.T, c *Chain, block interfaces.SignedBeaconBlock, index int, length int) {
	assert.Equal(t, length, len(c.chainState))
	assert.Equal(t, uint64(block.Block().Slot()), c.chainState[index].height)
	assert.Equal(t, blockHash(t, block), c.chainState[index].hash)
}

func blockHeader(t *testing.T, block interfaces.SignedBeaconBlock) *ethpb.SignedBeaconBlockHeader {
	header, err := block.Header()
	assert.NoError(t, err)

	return header
}

func blockHash(t *testing.T, block interfaces.SignedBeaconBlock) ethcommon.Hash {
	hash, err := block.Block().HashTreeRoot()
	assert.NoError(t, err)

	return hash
}
