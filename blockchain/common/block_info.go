package common

import (
	"math/big"

	ethcommon "github.com/ethereum/go-ethereum/common"
)

// BlockInfo wraps an Ethereum block with its total difficulty.
type BlockInfo struct {
	Block *Block
	TD    *big.Int
}

// NewBlockInfo composes a new BlockInfo. nil is considered a valid total difficulty for constructing this struct.
func NewBlockInfo(block *Block, totalDifficulty *big.Int) *BlockInfo {
	info := &BlockInfo{
		Block: block,
	}
	info.SetTotalDifficulty(totalDifficulty)
	return info
}

// SetTotalDifficulty sets the total difficulty, filtering nil arguments.
func (e *BlockInfo) SetTotalDifficulty(td *big.Int) {
	if td == nil {
		e.TD = big.NewInt(0)
	} else {
		e.TD = td
	}
}

// TotalDifficulty validates and returns the block's total difficulty. Any value <=0 may be encoded in types.BxBlock, and are considered invalid difficulties that should be treated as "unknown difficulty".
func (e *BlockInfo) TotalDifficulty() *big.Int {
	if e.TD.Int64() <= 0 {
		return nil
	}
	return e.TD
}

// Hash returns the hash of the block.
func (e *BlockInfo) Hash() ethcommon.Hash {
	return e.Block.Hash()
}

// Number returns the number of the block.
func (e *BlockInfo) Number() *big.Int {
	return e.Block.Number()
}
