package core

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"
)

// special error constant types
var (
	ErrInvalidRequest = errors.New("invalid request")

	ErrBodyNotFound          = errors.New("block body not stored")
	ErrBlobSidecarNotFound   = errors.New("blob sidecar not stored")
	ErrAlreadySeen           = errors.New("already seen")
	ErrAncientHeaders        = errors.New("headers requested are ancient")
	ErrFutureHeaders         = errors.New("headers requested are in the future")
	ErrQueryAmountIsNotValid = errors.New("query amount is not valid")
)

// BlockRef represents block info used for storing the best block
type BlockRef struct {
	Height uint64
	Hash   common.Hash
}

// String formats BlockRef for concise printing
func (b BlockRef) String() string {
	return fmt.Sprintf("%v[%v]", b.Height, b.Hash.TerminalString())
}

// BlockRefChain is a slice of BlockRef that represents a chain of blocks
type BlockRefChain []BlockRef

func (bc BlockRefChain) head() *BlockRef {
	if len(bc) == 0 {
		return &BlockRef{}
	}
	return &bc[0]
}

func (bc BlockRefChain) tail() *BlockRef {
	if len(bc) == 0 {
		return nil
	}
	return &bc[len(bc)-1]
}

// String formats BlockRefChain for concise printing
func (bc BlockRefChain) String() string {
	return fmt.Sprintf("chainstate(best: %v, oldest: %v)", bc.head(), bc.tail())
}
