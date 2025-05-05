package beacon

import (
	"errors"
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v6/beacon-chain/core/signing"
	p2ptypes "github.com/OffchainLabs/prysm/v6/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v6/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v6/config/params"
	"github.com/OffchainLabs/prysm/v6/consensus-types/interfaces"
	prysmTypes "github.com/OffchainLabs/prysm/v6/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v6/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v6/network/forks"
	"github.com/OffchainLabs/prysm/v6/time/slots"
	"github.com/ethereum/go-ethereum/common"

	"github.com/bloXroute-Labs/bxcommon-go/clock"
	"github.com/bloXroute-Labs/bxcommon-go/logger"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

const confirmationDelay = 4 * time.Second

// WrappedReadOnlySignedBeaconBlock represent wrapper for ReadOnlySignedBeaconBlock
type WrappedReadOnlySignedBeaconBlock struct {
	Block interfaces.ReadOnlySignedBeaconBlock
	hash  *common.Hash
	lock  *sync.RWMutex
}

// NewWrappedReadOnlySignedBeaconBlock create new WrappedReadOnlySignedBeaconBlock
func NewWrappedReadOnlySignedBeaconBlock(beaconBlock interfaces.ReadOnlySignedBeaconBlock) WrappedReadOnlySignedBeaconBlock {
	return WrappedReadOnlySignedBeaconBlock{Block: beaconBlock, lock: &sync.RWMutex{}}
}

// HashTreeRoot we use it when we want to extract hash, now it will extract only once
func (b *WrappedReadOnlySignedBeaconBlock) HashTreeRoot() (common.Hash, error) {
	b.lock.Lock()
	defer b.lock.Unlock()
	if b.hash == nil {
		rawHash, err := b.Block.Block().HashTreeRoot()
		if err != nil {
			logger.Errorf("error extracting block hash from block: %v", err)
			return common.Hash{}, err
		}
		blockHash := common.Hash(rawHash)
		b.hash = &blockHash
	}
	return *b.hash, nil
}

// SendBlockToBDN converts block to BxBlock, sends it to the bridge and starts a timer to send a confirmation after 4-seconds
// confirmation is needed to clean up the hash history.
func SendBlockToBDN(clock clock.Clock, log *logger.Entry, block WrappedReadOnlySignedBeaconBlock, bridge blockchain.Bridge, endpoint types.NodeEndpoint) error {
	bdnBeaconBlock, err := bridge.BlockBlockchainToBDN(block)
	if err != nil {
		return fmt.Errorf("could not convert beacon block: %v", err)
	}

	if err := bridge.SendBlockToBDN(bdnBeaconBlock, endpoint); err != nil {
		return fmt.Errorf("could not send block to gateway: %v", err)
	}

	clock.AfterFunc(confirmationDelay, func() {
		if err := bridge.SendConfirmedBlockToGateway(bdnBeaconBlock, endpoint); err != nil {
			log.Errorf("could not send beacon block confirmation to gateway: %v", err)
		}
	})

	return nil
}

func currentSlot(genesisTime uint64) prysmTypes.Slot {
	return prysmTypes.Slot(uint64(time.Now().Unix()-int64(genesisTime)) / params.BeaconConfig().SecondsPerSlot)
}

func epochStartTime(genesisTime uint64, epoch prysmTypes.Epoch) (time.Time, error) {
	slot, err := slots.EpochStart(epoch)
	if err != nil {
		return time.Time{}, err
	}

	return slots.ToTime(genesisTime, slot)
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

func currentForkDigest(genesisState state.BeaconState) ([4]byte, error) {
	genesisTime := time.Unix(int64(genesisState.GenesisTime()), 0)
	genesisValidatorsRoot := genesisState.GenesisValidatorsRoot()

	return forks.CreateForkDigest(genesisTime, genesisValidatorsRoot)
}
