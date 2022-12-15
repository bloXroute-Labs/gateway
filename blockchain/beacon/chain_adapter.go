package beacon

import (
	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
)

type chainAdapter struct {
	beaconBlock bool
	ethChain    *eth.Chain
	chain       *Chain
}

func newChainAdapter(beaconBlock bool, ethChain *eth.Chain, chain *Chain) *chainAdapter {
	return &chainAdapter{
		beaconBlock: beaconBlock,
		ethChain:    ethChain,
		chain:       chain,
	}
}

// AddBlock adds the provided block from the source into storage, updating the chainstate if the block comes from a reliable source. AddBlock returns the number of new canonical hashes added to the head if a reorganization happened. TODO: consider computing difficulty in here?
func (c *chainAdapter) AddBlock(b interfaces.SignedBeaconBlock, source BlockSource) (int, error) {
	if c.beaconBlock {
		return c.chain.AddBlock(b, source)
	}

	ethBlock, err := eth.BeaconBlockToEthBlock(b)
	if err != nil {
		return 0, err
	}

	return c.ethChain.AddBlock(eth.NewBlockInfo(ethBlock, nil), eth.BlockSource(source)), nil
}

// HasBlock indicates if block has been stored locally
func (c *chainAdapter) HasBlock(hash ethcommon.Hash) bool {
	if c.beaconBlock {
		return c.chain.HasBlock(hash)
	}

	return c.ethChain.HasBlock(hash)
}

// ValidateBlock determines if block can potentially be added to the chain
func (c *chainAdapter) ValidateBlock(block interfaces.SignedBeaconBlock) error {
	if c.beaconBlock {
		return c.chain.ValidateBlock(block)
	}

	ethBlock, err := eth.BeaconBlockToEthBlock(block)
	if err != nil {
		return err
	}

	return c.ethChain.ValidateBlock(ethBlock)
}

// GetNewHeadsForBDN fetches the newest blocks on the chainstate that have not previously been sent to the BDN. In cases of error, as many entries are still returned along with the error. Entries are returned in descending order.
func (c *chainAdapter) GetNewHeadsForBDN(count int) ([]interface{}, error) {
	var blks []interface{}
	if c.beaconBlock {
		blocks, err := c.chain.GetNewHeadsForBDN(count)
		if err != nil {
			return nil, err
		}

		for _, block := range blocks {
			blks = append(blks, block)
		}
	} else {
		blocks, err := c.ethChain.GetNewHeadsForBDN(count)
		if err != nil {
			return nil, err
		}

		for _, block := range blocks {
			blks = append(blks, block)
		}
	}

	return blks, nil

}

// MarkSentToBDN marks a block as having been sent to the BDN, so it does not need to be sent again in the future
func (c *chainAdapter) MarkSentToBDN(hash ethcommon.Hash) {
	if c.beaconBlock {
		c.chain.MarkSentToBDN(hash)
		return
	}

	c.ethChain.MarkSentToBDN(hash)
}

// MarkSentToBDN marks a block confirmation as having been sent to the BDN, so it does not need to be sent again in the future
func (c *chainAdapter) MarkConfirmationSentToBDN(hash ethcommon.Hash) {
	if c.beaconBlock {
		c.chain.MarkConfirmationSentToBDN(hash)
		return
	}

	c.ethChain.MarkConfirmationSentToBDN(hash)
}

// HasConfirmationSentToBDN indicates if block confirmation has been sent to the BDN
func (c *chainAdapter) HasConfirmationSentToBDN(hash ethcommon.Hash) bool {
	if c.beaconBlock {
		return c.chain.HasConfirmationSentToBDN(hash)
	}

	return c.ethChain.HasConfirmationSendToBDN(hash)
}

// BlockAtDepth returns the blockRefChain with depth from the head of the chain
func (c *chainAdapter) BlockAtDepth(chainDepth int) (interface{}, error) {
	if c.beaconBlock {
		return c.chain.BlockAtDepth(chainDepth)
	}

	return c.ethChain.BlockAtDepth(chainDepth)
}

func (c *chainAdapter) HeadHeight() uint64 {
	if c.beaconBlock {
		return c.chain.HeadHeight()
	}

	return c.ethChain.HeadHeight()
}
