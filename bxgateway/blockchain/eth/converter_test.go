package eth

import (
	"github.com/bloXroute-Labs/bxgateway-private-go/test/bxmock"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/assert"
	"math/big"
	"testing"
)

func TestConverter_Transactions(t *testing.T) {
	testTransactionType(t, ethtypes.LegacyTxType)
	testTransactionType(t, ethtypes.AccessListTxType)
	testTransactionType(t, ethtypes.DynamicFeeTxType)
}

func testTransactionType(t *testing.T, txType uint8) {
	c := Converter{}
	tx := bxmock.NewSignedEthTx(txType, 1, nil)

	bdnTx, err := c.TransactionBlockchainToBDN(tx)
	assert.Nil(t, err)

	blockchainTx, err := c.TransactionBDNToBlockchain(bdnTx)
	assert.Nil(t, err)

	originalEncodedBytes, err := rlp.EncodeToBytes(tx)
	assert.Nil(t, err)

	processedEncodedBytes, err := rlp.EncodeToBytes(blockchainTx.(*ethtypes.Transaction))
	assert.Nil(t, err)

	assert.Equal(t, originalEncodedBytes, processedEncodedBytes)
}

func TestConverter_Block(t *testing.T) {
	c := Converter{}
	block := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(100)

	bxBlock, err := c.BlockBlockchainToBDN(NewBlockInfo(block, td))
	assert.Nil(t, err)
	assert.Equal(t, block.Hash().Bytes(), bxBlock.Hash().Bytes())

	blockchainBlock, err := c.BlockBDNtoBlockchain(bxBlock)
	assert.Nil(t, err)

	blockInfo := blockchainBlock.(*BlockInfo)

	ethBlock := blockInfo.Block
	assert.Equal(t, block.Header(), ethBlock.Header())
	for i, tx := range block.Transactions() {
		assert.Equal(t, tx.Hash(), ethBlock.Transactions()[i].Hash())
	}
	assert.Equal(t, block.Uncles(), ethBlock.Uncles())

	assert.Equal(t, td, blockInfo.TotalDifficulty())

	canonicFormat, err := c.BxBlockToCanonicFormat(bxBlock)
	assert.Nil(t, err)
	for i, tx := range canonicFormat.Transactions {
		assert.Equal(t, tx.Hash.Bytes(), ethBlock.Transactions()[i].Hash().Bytes())
	}
}
