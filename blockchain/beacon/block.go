package beacon

import (
	"context"
	"encoding/hex"
	"errors"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
)

type broadcastFunc func(interfaces.ReadOnlySignedBeaconBlock) error

type blockProcessor struct {
	config    *network.EthConfig
	chain     *Chain
	log       *log.Entry
	bridge    blockchain.Bridge
	broadcast broadcastFunc
}

func newBlockProcessor(ctx context.Context, config *network.EthConfig, chain *Chain, bridge blockchain.Bridge, broadcast broadcastFunc, log *log.Entry) *blockProcessor {
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
func (n *blockProcessor) storeBDNBlock(bdnBlock *types.BxBlock) (interfaces.ReadOnlySignedBeaconBlock, error) {
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

	beaconBlock, ok := blockchainBlock.(interfaces.ReadOnlySignedBeaconBlock)
	if !ok {
		logBlockConverterFailure(err, bdnBlock)
		return nil, errors.New("could not convert BDN block to beacon block")
	}

	if _, err := n.chain.AddBlock(beaconBlock, BSBDN); err != nil {
		return nil, fmt.Errorf("could not add block %v: %v", blockHash, err)
	}
	return beaconBlock, nil
}

func (n *blockProcessor) ProcessBlockchainBlock(log *log.Entry, endpoint types.NodeEndpoint, block interfaces.ReadOnlySignedBeaconBlock) error {
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

	if len(newHeads) > 1 {
		log.Debugf("sending %d blocks from blockchain", len(newHeads))
	}

	// iterate in reverse to send all new heads in ascending order to BDN
	for i := len(newHeads) - 1; i >= 0; i-- {
		beaconBlock := newHeads[i]

		bdnBeaconBlock, err := n.bridge.BlockBlockchainToBDN(beaconBlock)
		if err != nil {
			log.Errorf("could not convert beacon block %v: %v", beaconBlockHash(beaconBlock), err)
			continue
		}

		if err := n.bridge.SendBlockToBDN(bdnBeaconBlock, peerEndpoint); err != nil {
			log.Errorf("could not send block to BDN: %v", err)
			continue
		}

		n.chain.MarkSentToBDN(ethcommon.Hash(bdnBeaconBlock.BeaconHash()))

		// convert and send ETH block for back compatibility
		ethBlock, err := eth.BeaconBlockToEthBlock(beaconBlock)
		if err != nil {
			log.Errorf("could not convert beacon block %v eth block: %v", bdnBeaconBlock.BeaconHash(), err)
			return
		}

		bdnEthBlock, err := n.bridge.BlockBlockchainToBDN(ethBlock)
		if err != nil {
			log.Errorf("could not convert eth block: %v", err)
			return
		}

		if err := n.bridge.SendBlockToBDN(bdnEthBlock, peerEndpoint); err != nil {
			log.Errorf("could not send block to BDN: %v", err)
			return
		}
	}

	beaconBlock, err := n.chain.BlockAtDepth(n.config.BlockConfirmationsCount)
	if err != nil {
		log.Debugf("cannot retrieve block with chain depth %v, %v", n.config.BlockConfirmationsCount, err)
		return
	}

	bdnBeaconBlock, err := n.bridge.BlockBlockchainToBDN(beaconBlock)
	if err != nil {
		log.Debugf("cannot convert beacon block to BDN block at the chain depth %v with hash %v, %v", n.config.BlockConfirmationsCount, beaconBlockHash(beaconBlock), err)
		return
	}

	blockHash := ethcommon.Hash(bdnBeaconBlock.BeaconHash())

	if n.chain.HasConfirmationSentToBDN(blockHash) {
		log.Debugf("block %v has already been sent in a block confirm message to gateway", blockHash)
		return
	}
	log.Tracef("sending beacon block (%v) confirm message to gateway from backend", blockHash)
	err = n.bridge.SendConfirmedBlockToGateway(bdnBeaconBlock, peerEndpoint)
	if err != nil {
		log.Debugf("failed sending beacon block(%v) confirmation message to gateway, %v", blockHash, err)
		return
	}
	n.chain.MarkConfirmationSentToBDN(blockHash)

	// convert and send ETH block confirmation for back compatibility
	ethBlock, err := eth.BeaconBlockToEthBlock(beaconBlock)
	if err != nil {
		log.Errorf("could not convert beacon block %v eth block: %v", bdnBeaconBlock.BeaconHash(), err)
		return
	}

	bdnEthBlock, err := n.bridge.BlockBlockchainToBDN(ethBlock)
	if err != nil {
		log.Debugf("cannot convert eth block to BDN block at the chain depth %v with hash %v, %v", n.config.BlockConfirmationsCount, bdnBeaconBlock.BeaconHash(), err)
		return
	}
	log.Tracef("sending eth block (%v) confirm message to gateway from backend", blockHash)
	err = n.bridge.SendConfirmedBlockToGateway(bdnEthBlock, peerEndpoint)
	if err != nil {
		log.Debugf("failed sending eth block(%v) confirmation message to gateway, %v", blockHash, err)
		return
	}
}

func beaconBlockHash(block interfaces.ReadOnlySignedBeaconBlock) string {
	// takes some time, but fine if use in rare errors
	hash, err := block.Block().HashTreeRoot()
	if err != nil {
		log.Errorf("cannot retrieve beacon block hash: %v", err)
		return ""
	}

	return hex.EncodeToString(hash[:])
}
