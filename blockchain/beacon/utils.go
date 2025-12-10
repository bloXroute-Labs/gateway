package beacon

import (
	"fmt"
	"sync"
	"time"

	"github.com/OffchainLabs/prysm/v7/beacon-chain/p2p"
	prysmTypes "github.com/OffchainLabs/prysm/v7/beacon-chain/p2p/types"
	"github.com/OffchainLabs/prysm/v7/beacon-chain/state"
	"github.com/OffchainLabs/prysm/v7/config/params"
	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	"github.com/OffchainLabs/prysm/v7/consensus-types/primitives"
	"github.com/OffchainLabs/prysm/v7/encoding/bytesutil"
	"github.com/OffchainLabs/prysm/v7/time/slots"
	"github.com/ethereum/go-ethereum/common"
	ssz "github.com/prysmaticlabs/fastssz"

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

func currentSlot(genesisTime uint64) primitives.Slot {
	//nolint:gosec
	// G115: unix time cannot be negative
	return primitives.Slot((uint64(time.Now().Unix()) - genesisTime) / params.BeaconConfig().SecondsPerSlot)
}

func extractValidDataTypeFromTopic(topic string, digest []byte) (ssz.Unmarshaler, error) {
	switch topic {
	case p2p.BlockSubnetTopicFormat:
		return extractDataTypeFromTypeMap(prysmTypes.BlockMap, digest)
	case p2p.AttestationSubnetTopicFormat:
		return extractDataTypeFromTypeMap(prysmTypes.AttestationMap, digest)
	case p2p.AggregateAndProofSubnetTopicFormat:
		return extractDataTypeFromTypeMap(prysmTypes.AggregateAttestationMap, digest)
	case p2p.AttesterSlashingSubnetTopicFormat:
		return extractDataTypeFromTypeMap(prysmTypes.AttesterSlashingMap, digest)
	case p2p.LightClientOptimisticUpdateTopicFormat:
		return extractDataTypeFromTypeMap(prysmTypes.LightClientOptimisticUpdateMap, digest)
	case p2p.LightClientFinalityUpdateTopicFormat:
		return extractDataTypeFromTypeMap(prysmTypes.LightClientFinalityUpdateMap, digest)
	}
	return nil, nil
}

func extractDataTypeFromTypeMap[T any](typeMap map[[4]byte]func() (T, error), digest []byte) (T, error) {
	var zero T

	if len(digest) == 0 {
		f, ok := typeMap[bytesutil.ToBytes4(params.BeaconConfig().GenesisForkVersion)]
		if !ok {
			return zero, fmt.Errorf("no %T type exists for the genesis fork version", zero)
		}
		return f()
	}
	if len(digest) != forkDigestLength {
		return zero, fmt.Errorf("invalid digest returned, wanted a length of %d but received %d", forkDigestLength, len(digest))
	}
	forkVersion, _, err := params.ForkDataFromDigest([4]byte(digest))
	if err != nil {
		return zero, fmt.Errorf("could not extract %T data type, saw digest=%#x", zero, digest)
	}

	f, ok := typeMap[forkVersion]
	if ok {
		return f()
	}
	return zero, fmt.Errorf("could not extract %T data type, saw digest=%#x", zero, digest)
}

func currentForkDigest(genesisState state.BeaconState) ([4]byte, error) {
	currentSlot := slots.CurrentSlot(genesisState.GenesisTime())
	currentEpoch := slots.ToEpoch(currentSlot)
	return params.ForkDigest(currentEpoch), nil
}
