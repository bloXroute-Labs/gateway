package eth

import (
	"context"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/eth/protocols/eth"
	"github.com/stretchr/testify/assert"
	"math/big"
	"testing"
	"time"
)

func TestChain_AddBlock(t *testing.T) {
	c := newChain(context.Background(), 5, 5, time.Hour, 1000)

	block1 := bxmock.NewEthBlock(1, common.Hash{})
	block2 := bxmock.NewEthBlock(2, block1.Hash())
	block3a := bxmock.NewEthBlock(3, block2.Hash())
	block3b := bxmock.NewEthBlock(3, block2.Hash())
	block4a := bxmock.NewEthBlock(4, block3a.Hash())
	block4b := bxmock.NewEthBlock(4, block3b.Hash())
	block5a := bxmock.NewEthBlock(5, block4a.Hash())
	block5b := bxmock.NewEthBlock(5, block4b.Hash())
	block6 := bxmock.NewEthBlock(6, block5b.Hash())

	newHeads := addBlockWithTD(c, block1, block1.Difficulty())
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block1, 0, 1)

	newHeads = addBlock(c, block2)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block2, 0, 2)
	assertChainState(t, c, block1, 1, 2)

	newHeads = addBlock(c, block3a)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block3a, 0, 3)
	assertChainState(t, c, block2, 1, 3)
	assertChainState(t, c, block1, 2, 3)

	newHeads = addBlock(c, block3b)
	assert.Equal(t, 0, newHeads)
	assertChainState(t, c, block3a, 0, 3)
	assertChainState(t, c, block2, 1, 3)
	assertChainState(t, c, block1, 2, 3)

	newHeads = addBlock(c, block4a)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block4a, 0, 4)
	assertChainState(t, c, block3a, 1, 4)
	assertChainState(t, c, block2, 2, 4)
	assertChainState(t, c, block1, 3, 4)

	newHeads = addBlock(c, block4b)
	assert.Equal(t, 0, newHeads)
	assertChainState(t, c, block4a, 0, 4)
	assertChainState(t, c, block3a, 1, 4)
	assertChainState(t, c, block2, 2, 4)
	assertChainState(t, c, block1, 3, 4)

	newHeads = addBlock(c, block5a)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block5a, 0, 5)
	assertChainState(t, c, block4a, 1, 5)
	assertChainState(t, c, block3a, 2, 5)
	assertChainState(t, c, block2, 3, 5)
	assertChainState(t, c, block1, 4, 5)

	newHeads = addBlock(c, block5b)
	assert.Equal(t, 0, newHeads)
	assertChainState(t, c, block5a, 0, 5)
	assertChainState(t, c, block4a, 1, 5)
	assertChainState(t, c, block3a, 2, 5)
	assertChainState(t, c, block2, 3, 5)
	assertChainState(t, c, block1, 4, 5)

	newHeads = addBlock(c, block6)
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

	block1 := bxmock.NewEthBlock(1, common.Hash{})
	block2 := bxmock.NewEthBlock(2, block1.Hash())
	block3 := bxmock.NewEthBlock(3, block2.Hash())
	block4a := bxmock.NewEthBlock(4, block3.Hash())
	block4b := bxmock.NewEthBlock(4, block3.Hash())
	block5 := bxmock.NewEthBlock(5, block4b.Hash())
	block6 := bxmock.NewEthBlock(6, block5.Hash())
	block7 := bxmock.NewEthBlock(7, block6.Hash())

	addBlockWithTD(c, block1, block1.Difficulty())

	newHeads := addBlock(c, block3)
	assert.Equal(t, 0, newHeads)
	assertChainState(t, c, block1, 0, 1)

	newHeads = addBlock(c, block2)
	assert.Equal(t, 1, newHeads)
	assertChainState(t, c, block2, 0, 2)
	assertChainState(t, c, block1, 1, 2)

	// found block 3 and filled it in
	newHeads = addBlock(c, block4a)
	assert.Equal(t, 2, newHeads)
	assertChainState(t, c, block4a, 0, 4)

	// add a bunch of entries that can't be added to chain (for some reason 4b is missing)
	newHeads = addBlock(c, block5)
	assert.Equal(t, 0, newHeads)
	newHeads = addBlock(c, block6)
	assert.Equal(t, 0, newHeads)

	// chain is long enough, don't care about 4b anymore
	newHeads = addBlock(c, block7)
	assert.Equal(t, 3, newHeads)
	assertChainState(t, c, block7, 0, 3)
	assertChainState(t, c, block6, 1, 3)
	assertChainState(t, c, block5, 2, 3)
}

func TestChain_AddBlock_LongFork(t *testing.T) {
	c := newChain(context.Background(), 2, 5, time.Hour, 1000)

	block1 := bxmock.NewEthBlock(1, common.Hash{})
	block2a := bxmock.NewEthBlock(2, block1.Hash())
	block2b := bxmock.NewEthBlock(2, block1.Hash())
	block3a := bxmock.NewEthBlock(3, block2a.Hash())
	block3b := bxmock.NewEthBlock(3, block2b.Hash())
	block4a := bxmock.NewEthBlock(4, block3a.Hash())
	block4b := bxmock.NewEthBlock(4, block3b.Hash())
	block5a := bxmock.NewEthBlock(5, block4a.Hash())
	block5b := bxmock.NewEthBlock(5, block4b.Hash())
	block6 := bxmock.NewEthBlock(6, block5b.Hash())

	addBlockWithTD(c, block1, block1.Difficulty())
	addBlock(c, block2a)
	addBlock(c, block2b)
	addBlock(c, block3a)
	addBlock(c, block3b)
	addBlock(c, block4a)
	addBlock(c, block4b)
	addBlock(c, block5a)
	addBlock(c, block5b)

	assertChainState(t, c, block5a, 0, 5)
	assertChainState(t, c, block4a, 1, 5)
	assertChainState(t, c, block3a, 2, 5)
	assertChainState(t, c, block2a, 3, 5)
	assertChainState(t, c, block1, 4, 5)

	newHeads := addBlock(c, block6)
	assert.Equal(t, 3, newHeads)

	assertChainState(t, c, block6, 0, 3)
	assertChainState(t, c, block5b, 1, 3)
	assertChainState(t, c, block4b, 2, 3)
}

func TestChain_GetHeaders_ByNumber(t *testing.T) {
	c := NewChain(context.Background())

	// true chain: 1, 2, 3b, 4
	block1 := bxmock.NewEthBlock(1, common.Hash{})
	block2 := bxmock.NewEthBlock(2, block1.Hash())
	block3a := bxmock.NewEthBlock(3, block2.Hash())
	block3b := bxmock.NewEthBlock(3, block2.Hash())
	block4 := bxmock.NewEthBlock(4, block3b.Hash())

	addBlockWithTD(c, block1, block1.Difficulty())
	addBlock(c, block2)
	addBlock(c, block3a)
	addBlock(c, block3b)
	addBlock(c, block4)

	var (
		headers []*ethtypes.Header
		err     error
	)

	// expected: err (neither header or hash provided)
	headers, err = c.GetHeaders(eth.HashOrNumber{}, 1, 0, false)
	assert.Equal(t, ErrInvalidRequest, err)

	// expected: err (headers in future)
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 10}, 1, 0, false)
	assert.Equal(t, ErrFutureHeaders, err)

	// expected: 1
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, block1.Header(), headers[0])

	// fork point, expected: 3b
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 3}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, block3b.Header(), headers[0])

	// expected: 1, 2, 3b, 4
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 4, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, block1.Header(), headers[0])
	assert.Equal(t, block2.Header(), headers[1])
	assert.Equal(t, block3b.Header(), headers[2])
	assert.Equal(t, block4.Header(), headers[3])

	// expected: 1, 3b
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 2, 1, false)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, block1.Header(), headers[0])
	assert.Equal(t, block3b.Header(), headers[1])

	// expected: 4, 2
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 4}, 2, 1, true)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, block4.Header(), headers[0])
	assert.Equal(t, block2.Header(), headers[1])

	// expected: 1, 2, 3b, 4 (found all that was possible)
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 100, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, block1.Header(), headers[0])
	assert.Equal(t, block2.Header(), headers[1])
	assert.Equal(t, block3b.Header(), headers[2])
	assert.Equal(t, block4.Header(), headers[3])

	// expected: err (header couldn't be located at the requested height in the past, so most create error)
	headers, err = c.GetHeaders(eth.HashOrNumber{Number: 1}, 100, 0, true)
	assert.NotNil(t, err)
}

func TestChain_GetHeaders_ByHash(t *testing.T) {
	c := NewChain(context.Background())

	// true chain: 1, 2, 3b, 4
	block1 := bxmock.NewEthBlock(1, common.Hash{})
	block2 := bxmock.NewEthBlock(2, block1.Hash())
	block3a := bxmock.NewEthBlock(3, block2.Hash())
	block3b := bxmock.NewEthBlock(3, block2.Hash())
	block4 := bxmock.NewEthBlock(4, block3b.Hash())

	addBlockWithTD(c, block1, block1.Difficulty())
	addBlock(c, block2)
	addBlock(c, block3a)
	addBlock(c, block3b)
	addBlock(c, block4)

	var (
		headers []*ethtypes.Header
		err     error
	)

	// expected: 1
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: block1.Hash()}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, block1.Header(), headers[0])

	// fork point, expected: 3a
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: block3a.Hash()}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, block3a.Header(), headers[0])

	// fork point, expected: 3b (even though it's not part of chain, still return it if requested)
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: block3b.Hash()}, 1, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 1, len(headers))
	assert.Equal(t, block3b.Header(), headers[0])

	// expected: 1, 2, 3b, 4
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: block1.Hash()}, 4, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, block1.Header(), headers[0])
	assert.Equal(t, block2.Header(), headers[1])
	assert.Equal(t, block3b.Header(), headers[2])
	assert.Equal(t, block4.Header(), headers[3])

	// expected: 1, 3b
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: block1.Hash()}, 2, 1, false)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, block1.Header(), headers[0])
	assert.Equal(t, block3b.Header(), headers[1])

	// expected: 4, 2
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: block4.Hash()}, 2, 1, true)
	assert.Nil(t, err)
	assert.Equal(t, 2, len(headers))
	assert.Equal(t, block4.Header(), headers[0])
	assert.Equal(t, block2.Header(), headers[1])

	// expected: 1, 2, 3b, 4 (found all that was possible)
	headers, err = c.GetHeaders(eth.HashOrNumber{Hash: block1.Hash()}, 100, 0, false)
	assert.Nil(t, err)
	assert.Equal(t, 4, len(headers))
	assert.Equal(t, block1.Header(), headers[0])
	assert.Equal(t, block2.Header(), headers[1])
	assert.Equal(t, block3b.Header(), headers[2])
	assert.Equal(t, block4.Header(), headers[3])
}

func TestChain_InitializeStatus(t *testing.T) {
	var ok bool
	c := NewChain(context.Background())

	initialHash1 := common.Hash{1, 2, 3}
	initialHash2 := common.Hash{2, 3, 4}
	initialDifficulty := big.NewInt(100)

	// initialize difficulty creates entries, but doesn't modify any other state
	c.InitializeDifficulty(initialHash1, initialDifficulty)
	assert.Equal(t, 1, c.heightToBlockHeaders.Count())
	assert.Equal(t, 0, len(c.chainState))

	addBlock(c, bxmock.NewEthBlock(100, common.Hash{}))
	assert.Equal(t, 2, c.heightToBlockHeaders.Count())
	assert.Equal(t, 1, len(c.chainState))

	c.InitializeDifficulty(initialHash2, initialDifficulty)
	assert.Equal(t, 2, c.heightToBlockHeaders.Count())
	assert.Equal(t, 1, len(c.chainState))

	_, ok = c.getBlockDifficulty(initialHash1)
	assert.True(t, ok)
	_, ok = c.getBlockDifficulty(initialHash2)
	assert.True(t, ok)

	// after any cleanup call, initialized entries status will be ejected
	c.clean(1)
	assert.Equal(t, 1, c.heightToBlockHeaders.Count())
	assert.Equal(t, 1, len(c.chainState))

	_, ok = c.getBlockDifficulty(initialHash1)
	assert.False(t, ok)
	_, ok = c.getBlockDifficulty(initialHash2)
	assert.False(t, ok)
}

func TestChain_GetNewHeadsForBDN(t *testing.T) {
	c := newChain(context.Background(), 5, 5, time.Hour, 1000)

	block1 := bxmock.NewEthBlock(1, common.Hash{})
	block2 := bxmock.NewEthBlock(2, block1.Hash())
	block3 := bxmock.NewEthBlock(3, block2.Hash())

	addBlockWithTD(c, block1, block1.Difficulty())
	addBlock(c, block2)

	blocks, err := c.GetNewHeadsForBDN(2)
	assert.Nil(t, err)
	assert.Equal(t, block2.Hash(), blocks[0].Block.Hash())
	assert.Equal(t, block1.Hash(), blocks[1].Block.Hash())

	_, err = c.GetNewHeadsForBDN(3)
	assert.NotNil(t, err)

	c.MarkSentToBDN(block2.Hash())
	addBlock(c, block3)

	blocks, err = c.GetNewHeadsForBDN(2)
	assert.Nil(t, err)
	assert.Equal(t, block3.Hash(), blocks[0].Block.Hash())
}

func addBlock(c *Chain, block *ethtypes.Block) int {
	return addBlockWithTD(c, block, nil)
}

func addBlockWithTD(c *Chain, block *ethtypes.Block, td *big.Int) int {
	bi := NewBlockInfo(block, td)
	_ = c.SetTotalDifficulty(bi)
	return c.AddBlock(bi, BSBlockchain)
}

func assertChainState(t *testing.T, c *Chain, block *ethtypes.Block, index int, length int) {
	assert.Equal(t, length, len(c.chainState))
	assert.Equal(t, block.NumberU64(), c.chainState[index].height)
	assert.Equal(t, block.Hash(), c.chainState[index].hash)
}
