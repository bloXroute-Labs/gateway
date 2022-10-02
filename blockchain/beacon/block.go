package beacon

import (
	"context"
	"errors"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/blockchain"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/logger"
	bxTypes "github.com/bloXroute-Labs/gateway/v2/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
)

type broadcastFunc func(interfaces.SignedBeaconBlock) error

type blockProcessor struct {
	config      *network.EthConfig
	chain       *Chain
	log         *logger.Entry
	bridge      blockchain.Bridge
	broadcast   broadcastFunc
	beaconBlock bool
}

func newBlockProcessor(ctx context.Context, config *network.EthConfig, bridge blockchain.Bridge, broadcast broadcastFunc, beaconBlock bool, log *logger.Entry) *blockProcessor {
	return &blockProcessor{
		config:      config,
		chain:       NewChain(ctx),
		bridge:      bridge,
		broadcast:   broadcast,
		beaconBlock: beaconBlock,
		log:         log,
	}
}

func (n *blockProcessor) ProcessBDNBlock(bdnBlock *bxTypes.BxBlock) {
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
		if err := n.broadcast(beaconBlock); err != nil {
			n.log.Errorf("could not broadcast block %s: %v", hash, err)
		}
	}
}

// storeBDNBlock will return a nil block and no error if block is a duplicate
func (n *blockProcessor) storeBDNBlock(bdnBlock *bxTypes.BxBlock) (interfaces.SignedBeaconBlock, error) {
	blockHash := ethcommon.BytesToHash(bdnBlock.Hash().Bytes())
	if n.chain.HasBlock(blockHash) {
		n.log.Debugf("duplicate block %v from BDN, skipping", blockHash)
		return nil, nil
	}

	blockchainBlock, err := n.bridge.BlockBDNtoBlockchain(bdnBlock)
	if err != nil {
		logBlockConverterFailure(err, bdnBlock)
		return nil, errors.New("could not convert BDN block to Ethereum block")
	}

	beaconBlock, ok := blockchainBlock.(interfaces.SignedBeaconBlock)
	if !ok {
		logBlockConverterFailure(err, bdnBlock)
		return nil, errors.New("could not convert BDN block to Ethereum block")
	}

	if _, err := n.chain.AddBlock(beaconBlock, BSBDN); err != nil {
		return nil, fmt.Errorf("could not add block %v: %v", blockHash, err)
	}
	return beaconBlock, nil
}

func (n *blockProcessor) ProcessBlockchainBlock(log *logger.Entry, endpoint bxTypes.NodeEndpoint, block interfaces.SignedBeaconBlock) error {
	blockHash, err := block.Block().HashTreeRoot()
	if err != nil {
		return fmt.Errorf("could not get block hash: %v", err)
	}

	blockHashString := ethcommon.BytesToHash(blockHash[:]).String()
	blockHeight := uint64(block.Block().Slot())

	if err := n.validateBlock(blockHash, blockHeight); err != nil {
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

func (n *blockProcessor) validateBlock(blockHash ethcommon.Hash, blockHeight uint64) error {
	if err := n.chain.ValidateBlock(blockHash, blockHeight); err != nil {
		return err
	}

	chainHeight := n.chain.chainState.head().height
	if chainHeight > 0 && int64(chainHeight)-int64(blockHeight) > int64(n.config.IgnoreSlotCount) {
		return fmt.Errorf("block too old, chainstate height %v", chainHeight)
	}
	return nil
}

func (n *blockProcessor) sendConfirmedBlocksToBDN(log *logger.Entry, count int, peerEndpoint bxTypes.NodeEndpoint) {
	newHeads, err := n.chain.GetNewHeadsForBDN(count)
	if err != nil {
		log.Errorf("could not fetch chainstate: %v", err)
	}

	// iterate in reverse to send all new heads in ascending order to BDN
	for i := len(newHeads) - 1; i >= 0; i-- {
		newHead := newHeads[i]

		bdnBlock, err := n.blockBlockchainToBDN(newHead)
		if err != nil {
			log.Errorf("could not convert block: %v", err)
			continue
		}

		err = n.bridge.SendBlockToBDN(bdnBlock, peerEndpoint)
		if err != nil {
			log.Errorf("could not send block to BDN: %v", err)
			continue
		}

		hash, err := newHead.Block().HashTreeRoot()
		if err != nil {
			log.Errorf("could not get hash: %v", err)
			continue
		}

		n.chain.MarkSentToBDN(hash)
	}

	b, err := n.blockAtDepth(log, n.config.BlockConfirmationsCount)
	if err != nil {
		log.Debugf("cannot retrieve bxblock at depth %v, %v", n.config.BlockConfirmationsCount, err)
		return
	}
	blockHash := ethcommon.BytesToHash(b.Hash().Bytes())
	metadata, ok := n.chain.getBlockMetadata(blockHash)
	if !ok {
		log.Debugf("cannot retrieve block metadata at depth %v, with hash %v", n.config.BlockConfirmationsCount, blockHash)
		return
	}
	if metadata.cnfMsgSent {
		log.Debugf("block %v has already been sent in a block confirm message to gateway", b.Hash())
		return
	}
	log.Tracef("sending block (%v) confirm message to gateway from backend", b.Hash())
	err = n.bridge.SendConfirmedBlockToGateway(b, peerEndpoint)
	if err != nil {
		log.Debugf("failed sending block(%v) confirmation message to gateway, %v", b.Hash(), err)
		return
	}
	n.chain.storeBlockMetadata(blockHash, metadata.height, metadata.confirmed, true)
}

func (n *blockProcessor) blockAtDepth(log *logger.Entry, chainDepth int) (*bxTypes.BxBlock, error) {
	block, err := n.chain.BlockAtDepth(chainDepth)
	if err != nil {
		log.Debugf("cannot retrieve block with chain depth %v, %v", chainDepth, err)
		return nil, err
	}

	hash, err := block.Block().HashTreeRoot()
	if err != nil {
		log.Debugf("could not get block hash with chain depth %v, %v", chainDepth, err)
		return nil, err
	}

	bxBlock, err := n.blockBlockchainToBDN(block)
	if err != nil {
		log.Debugf("cannot convert block to BDN block at the chain depth %v with hash %v, %v", chainDepth, hash, err)
		return nil, err
	}

	return bxBlock, nil
}

func (n *blockProcessor) blockBlockchainToBDN(block interfaces.SignedBeaconBlock) (*bxTypes.BxBlock, error) {
	if n.beaconBlock {
		return n.bridge.BlockBlockchainToBDN(block)
	}

	ethBlock, err := eth.BeaconBlockToEthBlock(block)
	if err != nil {
		return nil, err
	}

	return n.bridge.BlockBlockchainToBDN(eth.NewBlockInfo(ethBlock, ethBlock.Difficulty()))
}
