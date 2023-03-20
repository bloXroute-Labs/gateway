package beacon

import (
	"errors"
	"fmt"
	"time"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	bxTypes "github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common/hexutil"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/prysmaticlabs/prysm/v3/beacon-chain/core/signing"
	p2ptypes "github.com/prysmaticlabs/prysm/v3/beacon-chain/p2p/types"
	"github.com/prysmaticlabs/prysm/v3/config/params"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
	types "github.com/prysmaticlabs/prysm/v3/consensus-types/primitives"
	"github.com/prysmaticlabs/prysm/v3/encoding/bytesutil"
)

// special error constant types
var (
	ErrInvalidRequest        = errors.New("invalid request")
	ErrBodyNotFound          = errors.New("block body not stored")
	ErrAlreadySeen           = errors.New("already seen")
	ErrAncientHeaders        = errors.New("headers requested are ancient")
	ErrFutureHeaders         = errors.New("headers requested are in the future")
	ErrQueryAmountIsNotValid = errors.New("query amount is not valid")
)

func logBlockConverterFailure(err error, bdnBlock *bxTypes.BxBlock) {
	var blockHex string
	if log.IsLevelEnabled(log.TraceLevel) {
		b, err := rlp.EncodeToBytes(bdnBlock)
		if err != nil {
			blockHex = fmt.Sprintf("bad block from BDN could not be encoded to RLP bytes: %v", err)
		} else {
			blockHex = hexutil.Encode(b)
		}
	}
	log.Errorf("could not convert block (hash: %v) from BDN to beacon block: %v. contents: %v", bdnBlock.Hash(), err, blockHex)
}
func currentSlot(genesisTime uint64) types.Slot {
	return types.Slot(uint64(time.Now().Unix()-int64(genesisTime)) / params.BeaconConfig().SecondsPerSlot)
}

func extractBlockDataType(digest []byte, vRoot []byte) (interfaces.ReadOnlySignedBeaconBlock, error) {
	if len(digest) == 0 {
		bFunc, ok := p2ptypes.BlockMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
		if !ok {
			return nil, errors.New("no block type exists for the genesis fork version")
		}
		return bFunc()
	}
	if len(digest) != forkDigestLength {
		return nil, fmt.Errorf("invalid digest returned, wanted a length of %d but received %d", forkDigestLength, len(digest))
	}
	for k, blkFunc := range p2ptypes.BlockMap {
		rDigest, err := signing.ComputeForkDigest(k[:], vRoot[:])
		if err != nil {
			return nil, err
		}
		if rDigest == bytesutil.ToBytes4(digest) {
			return blkFunc()
		}
	}
	return nil, errors.New("no valid digest matched")
}
