package beacon

import (
	"context"
	"errors"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
)

type broadcastFunc func(interfaces.SignedBeaconBlock) error

type blockProcessor struct {
	config    *network.EthConfig
	chain     *chainAdapter
	log       *log.Entry
	bridge    blockchain.Bridge
	broadcast broadcastFunc
}

func newBlockProcessor(ctx context.Context, config *network.EthConfig, chain *chainAdapter, bridge blockchain.Bridge, broadcast broadcastFunc, log *log.Entry) *blockProcessor {
	return &blockProcessor{
		config:    config,
		chain:     chain,
		bridge:    bridge,
		broadcast: broadcast,
		log:       log,
	}
}

func (n *blockProcessor) ProcessBDNBlock(bdnBlock *types.BxBlock) {
	beaconBlock, err := n.storeBDNBlock(bdnBlock)
	if err != nil {
		logBlockConverterFailure(err, bdnBlock)
		return
	}

	// nil if block is a duplicate and does not need processing
	if beaconBlock == nil {
		return
	}

	hash, err := beaconBlock.Block().HashTreeRoot()
	if err != nil {
		n.log.Errorf("could not extract hash of beacon block slot %v: %s", beaconBlock.Block().Slot(), err)
		return
	}

	if n.broadcast != nil {
		go func() {
			if err := n.broadcast(beaconBlock); err != nil {
				n.log.Errorf("could not broadcast block %s: %v", ethcommon.Hash(hash), err)
			}
		}()
	}
}

// storeBDNBlock will return a nil block and no error if block is a duplicate
func (n *blockProcessor) storeBDNBlock(bdnBlock *types.BxBlock) (interfaces.SignedBeaconBlock, error) {
	blockHash := ethcommon.BytesToHash(bdnBlock.BeaconHash().Bytes())
	if n.chain.HasBlock(blockHash) {
		n.log.Debugf("duplicate block %v from BDN, skipping", blockHash)
		return nil, nil
	}

	blockchainBlock, err := n.bridge.BlockBDNtoBlockchain(bdnBlock)
	if err != nil {
		logBlockConverterFailure(err, bdnBlock)
		return nil, errors.New("could not convert BDN block to beacon block")
	}

	beaconBlock, ok := blockchainBlock.(interfaces.SignedBeaconBlock)
	if !ok {
		logBlockConverterFailure(err, bdnBlock)
		return nil, errors.New("could not convert BDN block to beacon block")
	}

	if _, err := n.chain.AddBlock(beaconBlock, BSBDN); err != nil {
		return nil, fmt.Errorf("could not add block %v: %v", blockHash, err)
	}
	return beaconBlock, nil
}

func (n *blockProcessor) ProcessBlockchainBlock(log *log.Entry, endpoint types.NodeEndpoint, block interfaces.SignedBeaconBlock) error {
	blockHash, err := block.Block().HashTreeRoot()
	if err != nil {
		return fmt.Errorf("could not get block hash: %v", err)
	}

	blockHashString := ethcommon.BytesToHash(blockHash[:])
	blockHeight := uint64(block.Block().Slot())

	if err := n.chain.ValidateBlock(block); err != nil {
		if err == ErrAlreadySeen {
			log.Debugf("skipping block %v (height %v): %v", blockHashString, blockHeight, err)
		} else {
			log.Warnf("skipping block %v (height %v): %v", blockHashString, blockHeight, err)
		}
		return nil
	}

	log.Debugf("processing new block %v (height %v)", blockHashString, blockHeight)
	newHeadCount, err := n.chain.AddBlock(block, BSBlockchain)
	if err != nil {
		return fmt.Errorf("could not add block %v: %v", blockHashString, err)
	}

	n.sendConfirmedBlocksToBDN(log, newHeadCount, endpoint)

	// TODO: send to _others_ peers once we support multiple nodes

	return nil
}

func (n *blockProcessor) sendConfirmedBlocksToBDN(log *log.Entry, count int, peerEndpoint types.NodeEndpoint) {
	newHeads, err := n.chain.GetNewHeadsForBDN(count)
	if err != nil {
		log.Errorf("could not fetch chainstate: %v", err)
	}

	// iterate in reverse to send all new heads in ascending order to BDN
	for i := len(newHeads) - 1; i >= 0; i-- {
		newHead := newHeads[i]

		bdnBlock, err := n.bridge.BlockBlockchainToBDN(newHead)
		if err != nil {
			log.Errorf("could not convert block: %v", err)
			continue
		}

		if err := n.bridge.SendBlockToBDN(bdnBlock, peerEndpoint); err != nil {
			log.Errorf("could not send block to BDN: %v", err)
			continue
		}

		n.chain.MarkSentToBDN(ethcommon.Hash(bdnBlock.BeaconHash()))
	}

	b, err := n.blockAtDepth(n.config.BlockConfirmationsCount)
	if err != nil {
		log.Debugf("cannot retrieve bxblock at depth %v, %v", n.config.BlockConfirmationsCount, err)
		return
	}
	blockHash := ethcommon.Hash(b.BeaconHash())

	if n.chain.HasConfirmationSentToBDN(blockHash) {
		log.Debugf("block %v has already been sent in a block confirm message to gateway", blockHash)
		return
	}
	log.Tracef("sending block (%v) confirm message to gateway from backend", blockHash)
	err = n.bridge.SendConfirmedBlockToGateway(b, peerEndpoint)
	if err != nil {
		log.Debugf("failed sending block(%v) confirmation message to gateway, %v", blockHash, err)
		return
	}
	n.chain.MarkConfirmationSentToBDN(blockHash)
}

func (n *blockProcessor) blockAtDepth(chainDepth int) (*types.BxBlock, error) {
	block, err := n.chain.BlockAtDepth(chainDepth)
	if err != nil {
		log.Debugf("cannot retrieve block with chain depth %v, %v", chainDepth, err)
		return nil, err
	}
	bxBlock, err := n.bridge.BlockBlockchainToBDN(block)
	if err != nil {
		log.Debugf("cannot convert eth block to BDN block at the chain depth %v with hash %v, %v", chainDepth, bxBlock.BeaconHash(), err)
		return nil, err
	}
	return bxBlock, err
}
