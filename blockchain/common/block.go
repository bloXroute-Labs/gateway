package common

import (
	ethTypes "github.com/ethereum/go-ethereum/core/types"
)

// Block represents a wrapper around an ethereum execution layer block, providing additional functionality.
// It is a copy of the original go-ethereum Block struct, with the addition of the sidecars field to support BSC.
// Currently used only by BSC or Polygon chains.
type Block struct {
	ethTypes.Block

	// sidecars provides DA check
	sidecars BlobSidecars `rlp:"optional"` // Only used by BSC
}

// NewBlock creates a new block. The input data is copied,
func NewBlock(header *ethTypes.Header, body *ethTypes.Body, receipts []*ethTypes.Receipt, hasher ethTypes.TrieHasher) *Block {
	return &Block{Block: *ethTypes.NewBlock(header, body, receipts, hasher)}
}

// Sidecars returns the sidecars of the block.
func (b *Block) Sidecars() BlobSidecars {
	return b.sidecars
}

// NewBlockWithHeader creates a block with the given header data. The
// header data is copied, changes to header and to the field values
// will not affect the block.
func NewBlockWithHeader(header *ethTypes.Header) *Block {
	return &Block{Block: *ethTypes.NewBlockWithHeader(header)}
}

// WithBody returns a copy of the block with the given transaction and uncle contents.
func (b *Block) WithBody(body ethTypes.Body) *Block {
	block := &Block{Block: *b.Block.WithBody(body), sidecars: b.sidecars}
	return block
}

// SetBlobSidecars sets the sidecars of the block without copying.
func (b *Block) SetBlobSidecars(sidecars BlobSidecars) {
	b.sidecars = sidecars
}

// WithSidecars returns a block containing the given blobs.
func (b *Block) WithSidecars(sidecars BlobSidecars) *Block {
	block := &Block{Block: b.Block}
	if sidecars != nil {
		block.sidecars = make(BlobSidecars, len(sidecars))
		copy(block.sidecars, sidecars)
	}
	return block
}
