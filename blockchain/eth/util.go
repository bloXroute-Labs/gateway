package eth

import (
	"errors"
	"fmt"

	"github.com/ethereum/go-ethereum/common"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// special error constant types
var (
	ErrInvalidRequest        = errors.New("invalid request")
	ErrInvalidPacketType     = errors.New("invalid packet type")
	ErrBodyNotFound          = errors.New("block body not stored")
	ErrBlobSidecarNotFound   = errors.New("blob sidecar not stored")
	ErrAlreadySeen           = errors.New("already seen")
	ErrAncientHeaders        = errors.New("headers requested are ancient")
	ErrFutureHeaders         = errors.New("headers requested are in the future")
	ErrQueryAmountIsNotValid = errors.New("query amount is not valid")
)

// blockRef represents block info used for storing best block
type blockRef struct {
	height uint64
	hash   common.Hash
}

// String formats blockRef for concise printing
func (b blockRef) String() string {
	return fmt.Sprintf("%v[%v]", b.height, b.hash.TerminalString())
}

type blockRefChain []blockRef

func (bc blockRefChain) head() *blockRef {
	if len(bc) == 0 {
		return &blockRef{}
	}
	return &bc[0]
}

func (bc blockRefChain) tail() *blockRef {
	if len(bc) == 0 {
		return nil
	}
	return &bc[len(bc)-1]
}

// String formats blockRefChain for concise printing
func (bc blockRefChain) String() string {
	return fmt.Sprintf("chainstate(best: %v, oldest: %v)", bc.head(), bc.tail())
}

type keyWriteError struct {
	error
}

// NewSHA256Hash is a utility function for converting between Ethereum common hashes and bloxroute hashes
func NewSHA256Hash(hash common.Hash) types.SHA256Hash {
	var sha256Hash types.SHA256Hash
	copy(sha256Hash[:], hash.Bytes())
	return sha256Hash
}
