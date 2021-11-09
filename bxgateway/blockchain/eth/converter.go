package eth

import (
	"fmt"
	"github.com/bloXroute-Labs/bxgateway-private-go/bxgateway/types"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"math/big"
)

type bxBlockRLP struct {
	Header  rlp.RawValue
	Txs     []rlp.RawValue
	Trailer rlp.RawValue
}

// BlockInfo wraps an Ethereum block with its total difficulty.
type BlockInfo struct {
	Block           *ethtypes.Block
	totalDifficulty *big.Int
}

// NewBlockInfo composes a new BlockInfo. nil is considered a valid total difficulty for constructing this struct
func NewBlockInfo(block *ethtypes.Block, totalDifficulty *big.Int) *BlockInfo {
	info := &BlockInfo{
		Block: block,
	}
	info.SetTotalDifficulty(totalDifficulty)
	return info
}

// SetTotalDifficulty sets the total difficulty, filtering nil arguments
func (e *BlockInfo) SetTotalDifficulty(td *big.Int) {
	if td == nil {
		e.totalDifficulty = big.NewInt(0)
	} else {
		e.totalDifficulty = td
	}
}

// TotalDifficulty validates and returns the block's total difficulty. Any value <=0 may be encoded in types.BxBlock, and are considered invalid difficulties that should be treated as "unknown diffiuclty"
func (e BlockInfo) TotalDifficulty() *big.Int {
	if e.totalDifficulty.Int64() <= 0 {
		return nil
	}
	return e.totalDifficulty
}

// Converter is an Ethereum-BDN converter struct
type Converter struct{}

// TransactionBDNToBlockchain convert a BDN transaction to an Ethereum one
func (c Converter) TransactionBDNToBlockchain(transaction *types.BxTransaction) (interface{}, error) {
	var ethTransaction ethtypes.Transaction
	err := rlp.DecodeBytes(transaction.Content(), &ethTransaction)
	return &ethTransaction, err
}

// TransactionBlockchainToBDN converts an Ethereum transaction to a BDN transaction
func (c Converter) TransactionBlockchainToBDN(i interface{}) (*types.BxTransaction, error) {
	transaction := i.(*ethtypes.Transaction)
	hash := NewSHA256Hash(transaction.Hash())

	content, err := rlp.EncodeToBytes(transaction)
	if err != nil {
		return nil, err
	}

	return types.NewRawBxTransaction(hash, content), nil
}

// BlockBlockchainToBDN converts an Ethereum block to a BDN block
func (c Converter) BlockBlockchainToBDN(i interface{}) (*types.BxBlock, error) {
	blockInfo := i.(*BlockInfo)
	block := blockInfo.Block
	hash := NewSHA256Hash(block.Hash())

	encodedHeader, err := rlp.EncodeToBytes(block.Header())
	if err != nil {
		return nil, fmt.Errorf("could not encode block header: %v: %v", block.Header(), err)
	}

	encodedTrailer, err := rlp.EncodeToBytes(block.Uncles())
	if err != nil {
		return nil, fmt.Errorf("could not encode block trailer: %v: %v", block.Uncles(), err)
	}

	var txs []*types.BxBlockTransaction
	for _, tx := range block.Transactions() {
		txBytes, err := rlp.EncodeToBytes(tx)
		if err != nil {
			return nil, fmt.Errorf("could not encode transaction %v", tx)
		}

		txHash := NewSHA256Hash(tx.Hash())
		compressedTx := types.NewBxBlockTransaction(txHash, txBytes)
		txs = append(txs, compressedTx)
	}

	difficulty := blockInfo.TotalDifficulty()
	if difficulty == nil {
		difficulty = big.NewInt(0)
	}
	return types.NewBxBlock(hash, encodedHeader, txs, encodedTrailer, difficulty, block.Number())
}

// BlockBDNtoBlockchain converts a BDN block to an Ethereum block
func (c Converter) BlockBDNtoBlockchain(block *types.BxBlock) (interface{}, error) {
	txs := make([]rlp.RawValue, 0, len(block.Txs))
	for _, tx := range block.Txs {
		txs = append(txs, tx.Content())
	}

	b, err := rlp.EncodeToBytes(bxBlockRLP{
		Header:  block.Header,
		Txs:     txs,
		Trailer: block.Trailer,
	})
	if err != nil {
		return nil, fmt.Errorf("could not convert block %v to blockchain format: %v", block.Hash(), err)
	}

	var ethBlock ethtypes.Block
	if err = rlp.DecodeBytes(b, &ethBlock); err != nil {
		return nil, fmt.Errorf("could not convert block %v to blockchain format: %v", block.Hash(), err)
	}
	return NewBlockInfo(&ethBlock, block.TotalDifficulty), nil
}

// BxBlockToCanonicFormat converts a block from BDN format to BlockNotification format
func (c Converter) BxBlockToCanonicFormat(bxBlock *types.BxBlock) (*types.BlockNotification, error) {
	result, err := c.BlockBDNtoBlockchain(bxBlock)
	if err != nil {
		return nil, err
	}
	ethBlock := result.(*BlockInfo).Block

	ethTxs := make([]types.EthTransaction, 0, len(ethBlock.Transactions()))
	for _, tx := range ethBlock.Transactions() {
		var ethTx *types.EthTransaction
		txHash := NewSHA256Hash(tx.Hash())
		ethTx, err = types.NewEthTransaction(txHash, tx)
		if err != nil {
			return nil, err
		}
		ethTxs = append(ethTxs, *ethTx)
	}
	ethUncles := make([]types.Header, 0, len(ethBlock.Uncles()))
	for _, uncle := range ethBlock.Uncles() {
		ethUncle := types.ConvertEthHeaderToBlockNotificationHeader(uncle)
		ethUncles = append(ethUncles, *ethUncle)
	}
	blockNotification := types.BlockNotification{
		BlockHash:    ethBlock.Hash(),
		Header:       types.ConvertEthHeaderToBlockNotificationHeader(ethBlock.Header()),
		Transactions: ethTxs,
		Uncles:       ethUncles,
	}
	return &blockNotification, nil
}
