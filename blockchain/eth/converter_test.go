package eth

import (
	"math/big"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/beacon"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/prysmaticlabs/prysm/v5/consensus-types/interfaces"
	"github.com/stretchr/testify/assert"
)

func TestConverter_Transactions(t *testing.T) {
	testTransactionType(t, ethtypes.LegacyTxType)
	testTransactionType(t, ethtypes.AccessListTxType)
	testTransactionType(t, ethtypes.DynamicFeeTxType)
	testTransactionType(t, ethtypes.BlobTxType)
}

func testTransactionType(t *testing.T, txType uint8) {
	c := Converter{}
	tx := bxmock.NewSignedEthTx(txType, 1, nil, nil)

	bdnTx, err := c.TransactionBlockchainToBDN(tx)
	assert.NoError(t, err)

	blockchainTx, err := c.TransactionBDNToBlockchain(bdnTx)
	assert.NoError(t, err)

	originalEncodedBytes, err := rlp.EncodeToBytes(tx)
	assert.NoError(t, err)

	processedEncodedBytes, err := rlp.EncodeToBytes(blockchainTx.(*ethtypes.Transaction))
	assert.NoError(t, err)

	assert.Equal(t, originalEncodedBytes, processedEncodedBytes)
}

func TestConverter_Block(t *testing.T) {
	c := Converter{}
	block := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(100)

	bxBlock, err := c.BlockBlockchainToBDN(NewBlockInfo(block, td))
	assert.NoError(t, err)
	assert.Equal(t, block.Hash().Bytes(), bxBlock.Hash().Bytes())

	blockchainBlock, err := c.BlockBDNtoBlockchain(bxBlock)
	assert.NoError(t, err)

	blockInfo := blockchainBlock.(*BlockInfo)

	ethBlock := blockInfo.Block
	assert.Equal(t, block.Header(), ethBlock.Header())
	for i, tx := range block.Transactions() {
		assert.Equal(t, tx.Hash(), ethBlock.Transactions()[i].Hash())
	}
	assert.Equal(t, block.Uncles(), ethBlock.Uncles())

	assert.Equal(t, td, blockInfo.TotalDifficulty())

	canonicFormat, err := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil, false)
	assert.NoError(t, err)

	for i, tx := range canonicFormat.Transactions {
		assert.Equal(t, tx["hash"], ethBlock.Transactions()[i].Hash().String())
	}
}

func TestConverter_DenebBeaconBlock(t *testing.T) {
	c := Converter{}
	block := bxmock.NewEthBlock(10, common.Hash{})

	beaconBlock := bxmock.NewDenebBeaconBlock(t, 11, nil, block)

	bxBlock, err := c.BlockBlockchainToBDN(beacon.NewWrappedReadOnlySignedBeaconBlock(beaconBlock))
	assert.NoError(t, err)
	assert.Equal(t, block.Hash().Bytes(), bxBlock.Hash().Bytes())

	blockchainBlock, err := c.BlockBDNtoBlockchain(bxBlock)
	assert.NoError(t, err)

	denebBlock, err := blockchainBlock.(interfaces.ReadOnlySignedBeaconBlock).PbDenebBlock()
	assert.NoError(t, err)

	// Check beacon BxBlock transactions exactly same as eth block
	for i, tx := range block.Transactions() {
		blockTx := new(ethtypes.Transaction)
		err = blockTx.UnmarshalBinary(denebBlock.GetBlock().GetBody().GetExecutionPayload().GetTransactions()[i])
		assert.NoError(t, err)

		assert.Equal(t, blockTx.Hash(), tx.Hash())
	}

	beaconCanonicFormat, err := types.NewBeaconBlockNotification(blockchainBlock.(interfaces.ReadOnlySignedBeaconBlock))
	assert.NoError(t, err)

	// Check beacon notification transactions exactly same as beacon BxBlock
	for i, txRaw := range beaconCanonicFormat.(*types.DenebBlockNotification).Block.Body.ExecutionPayload.Transactions {
		notificationTx := new(ethtypes.Transaction)
		err = notificationTx.UnmarshalBinary(txRaw)
		assert.NoError(t, err)

		tx := new(ethtypes.Transaction)
		err = tx.UnmarshalBinary(denebBlock.GetBlock().GetBody().GetExecutionPayload().GetTransactions()[i])
		assert.NoError(t, err)

		assert.Equal(t, notificationTx.Hash(), tx.Hash())
	}
}
