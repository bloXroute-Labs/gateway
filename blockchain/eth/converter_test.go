package eth

import (
	"math/big"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/prysmaticlabs/prysm/v3/consensus-types/interfaces"
	"github.com/stretchr/testify/assert"
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

	canonicFormat, _, err := c.BxBlockToCanonicFormat(bxBlock, nil)
	assert.Nil(t, err)

	for i, tx := range canonicFormat.(*types.EthBlockNotification).Transactions {
		assert.Equal(t, tx["hash"], ethBlock.Transactions()[i].Hash().String())
	}
}

func TestConverter_BeaconBlock(t *testing.T) {
	c := Converter{}
	block := bxmock.NewEthBlock(10, common.Hash{})

	beaconBlock := bxmock.NewBellatrixBeaconBlock(t, 11, nil, block)

	bxBlock, err := c.BlockBlockchainToBDN(beaconBlock)
	assert.Nil(t, err)
	assert.Equal(t, block.Hash().Bytes(), bxBlock.Hash().Bytes())

	blockchainBlock, err := c.BlockBDNtoBlockchain(bxBlock)
	assert.Nil(t, err)

	bellatrixBlock, err := blockchainBlock.(interfaces.SignedBeaconBlock).PbBellatrixBlock()
	assert.NoError(t, err)

	// Check beacon BxBlock transactions exactly same as eth block
	for i, tx := range block.Transactions() {
		bellatrixTx := new(ethtypes.Transaction)
		err = bellatrixTx.UnmarshalBinary(bellatrixBlock.GetBlock().GetBody().GetExecutionPayload().GetTransactions()[i])
		assert.NoError(t, err)

		assert.Equal(t, tx.Hash(), bellatrixTx.Hash())
	}

	canonicFormat, beaconCanonicFormat, err := c.BxBlockToCanonicFormat(bxBlock, nil)
	assert.Nil(t, err)

	// Check eth notification transactions exactly same as beacon BxBlock
	for i, tx := range canonicFormat.(*types.EthBlockNotification).Transactions {
		bellatrixTx := new(ethtypes.Transaction)
		err = bellatrixTx.UnmarshalBinary(bellatrixBlock.GetBlock().GetBody().GetExecutionPayload().GetTransactions()[i])
		assert.NoError(t, err)

		assert.Equal(t, tx["hash"], bellatrixTx.Hash().String())
	}

	// Check beacon notification transactions exactly same as beacon BxBlock
	for i, txRaw := range beaconCanonicFormat.(*types.BellatrixBlockNotification).Block.Body.ExecutionPayload.Transactions {
		notificationTx := new(ethtypes.Transaction)
		err = notificationTx.UnmarshalBinary(txRaw)
		assert.NoError(t, err)

		bellatrixTx := new(ethtypes.Transaction)
		err = bellatrixTx.UnmarshalBinary(bellatrixBlock.GetBlock().GetBody().GetExecutionPayload().GetTransactions()[i])
		assert.NoError(t, err)

		assert.Equal(t, notificationTx.Hash(), bellatrixTx.Hash())
	}
}
