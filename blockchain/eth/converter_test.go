package eth

import (
	"log"
	"math/big"
	"testing"

	"github.com/OffchainLabs/prysm/v7/consensus-types/interfaces"
	eth "github.com/OffchainLabs/prysm/v7/proto/prysm/v1alpha1"
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/bdn"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/beacon"
	bxethcommon "github.com/bloXroute-Labs/gateway/v2/blockchain/common"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/core"
	"github.com/bloXroute-Labs/gateway/v2/test/bxmock"
	"github.com/bloXroute-Labs/gateway/v2/types"
)

var BSCBlobSidecars bxethcommon.BlobSidecars

func init() {
	var err error
	BSCBlobSidecars, err = bxethcommon.ReadMockBSCBlobSidecars()
	if err != nil {
		log.Fatalf("Failed to read BSC blob sidecars: %v", err)
	}
}

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
	require.NoError(t, err)

	blockchainTx, err := c.TransactionBDNToBlockchain(bdnTx)
	require.NoError(t, err)

	originalEncodedBytes, err := rlp.EncodeToBytes(tx)
	require.NoError(t, err)

	processedEncodedBytes, err := rlp.EncodeToBytes(blockchainTx.(*ethtypes.Transaction))
	require.NoError(t, err)

	require.Equal(t, originalEncodedBytes, processedEncodedBytes)
}

func TestConverter_Block(t *testing.T) {
	c := Converter{}
	block := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(100)

	bxBlock, err := c.BlockBlockchainToBDN(core.NewBlockInfo(block, td))
	require.NoError(t, err)
	require.Equal(t, block.Hash().Bytes(), bxBlock.Hash().Bytes())

	blockchainBlock, err := c.BlockBDNtoBlockchain(bxBlock)
	require.NoError(t, err)

	blockInfo := blockchainBlock.(*core.BlockInfo)

	ethBlock := blockInfo.Block
	require.Equal(t, block.Header(), ethBlock.Header())
	for i, tx := range block.Transactions() {
		require.Equal(t, tx.Hash(), ethBlock.Transactions()[i].Hash())
	}
	require.Equal(t, block.Uncles(), ethBlock.Uncles())

	require.Equal(t, td, blockInfo.TotalDifficulty())

	canonicFormat, err := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil)
	require.NoError(t, err)

	for i, tx := range canonicFormat.Transactions {
		require.Equal(t, tx["hash"], ethBlock.Transactions()[i].Hash().String())
	}
}

func TestConverter_BSCBlockWithBlobs(t *testing.T) {
	c := Converter{}
	block := bxmock.NewEthBlock(10, common.Hash{})
	td := big.NewInt(100)

	require.Equal(t, len(BSCBlobSidecars), 1)

	block = block.WithSidecars(BSCBlobSidecars)
	block.Number().SetUint64(BSCBlobSidecars[0].BlockNumber.Uint64())

	bxBlock, err := c.BlockBlockchainToBDN(core.NewBlockInfo(block, td))
	require.NoError(t, err)
	require.Equal(t, block.Hash().Bytes(), bxBlock.Hash().Bytes())

	blockchainBlock, err := c.BlockBDNtoBlockchain(bxBlock)
	require.NoError(t, err)

	blockInfo := blockchainBlock.(*core.BlockInfo)

	ethBlock := blockInfo.Block
	require.Equal(t, block.Header(), ethBlock.Header())
	for i, tx := range block.Transactions() {
		require.Equal(t, tx.Hash(), ethBlock.Transactions()[i].Hash())
	}

	for i, sidecar := range block.Sidecars() {
		require.Equal(t, sidecar.BlobTxSidecar, ethBlock.Sidecars()[i].BlobTxSidecar)
		require.Equal(t, sidecar.TxHash, ethBlock.Sidecars()[i].TxHash)
		require.Equal(t, block.Hash(), ethBlock.Sidecars()[i].BlockHash)
		require.Equal(t, block.Number().Uint64(), ethBlock.Sidecars()[i].BlockNumber.Uint64())
	}

	require.Equal(t, block.Uncles(), ethBlock.Uncles())

	require.Equal(t, td, blockInfo.TotalDifficulty())

	canonicFormat, err := types.NewEthBlockNotification(ethBlock.Hash(), ethBlock, nil)
	require.NoError(t, err)

	for i, tx := range canonicFormat.Transactions {
		require.Equal(t, tx["hash"], ethBlock.Transactions()[i].Hash().String())
	}
}

func TestConverter(t *testing.T) {
	c := Converter{}

	block := bxmock.NewEthBlock(10, common.Hash{})

	t.Run("DenebBeaconBlock", func(t *testing.T) {
		beaconBlock := bxmock.NewDenebBeaconBlock(t, block)

		testConverter(t, c, beaconBlock, block)
	})
	t.Run("ElectraBeaconBlock", func(t *testing.T) {
		beaconBlock := bxmock.NewElectraBeaconBlock(t, block)

		testConverter(t, c, beaconBlock, block)
	})
}

func testConverter(t *testing.T, c Converter, beaconBlock interfaces.ReadOnlySignedBeaconBlock, block *bxethcommon.Block) {
	t.Helper()

	header, err := beaconBlock.Header()
	require.NoError(t, err)

	requiredBeaconHash, err := header.Header.HashTreeRoot()
	require.NoError(t, err)

	bxBlock, err := c.BlockBlockchainToBDN(beacon.NewWrappedReadOnlySignedBeaconBlock(beaconBlock))
	require.NoError(t, err)
	require.Equal(t, requiredBeaconHash[:], bxBlock.Hash().Bytes())

	blockchainBlock, err := c.BlockBDNtoBlockchain(bxBlock)
	require.NoError(t, err)

	readOnlyBlock, err := bdn.PbGenericBlock(blockchainBlock.(interfaces.ReadOnlySignedBeaconBlock))
	require.NoError(t, err)

	genericBlock := readOnlyBlock.GetBlock()

	var gbTransactions [][]byte

	switch bb := genericBlock.(type) {
	case *eth.GenericSignedBeaconBlock_Deneb:
		gbTransactions = bb.Deneb.GetBlock().GetBlock().GetBody().GetExecutionPayload().GetTransactions()
	case *eth.GenericSignedBeaconBlock_Electra:
		gbTransactions = bb.Electra.GetBlock().GetBlock().GetBody().GetExecutionPayload().GetTransactions()
	default:
		t.Fatalf("unexpected block type: %T", genericBlock)
	}

	// Check beacon BxBlock transactions exactly same as eth block
	for i, tx := range block.Transactions() {
		blockTx := new(ethtypes.Transaction)
		err = blockTx.UnmarshalBinary(gbTransactions[i])
		require.NoError(t, err)

		require.Equal(t, blockTx.Hash(), tx.Hash())
	}

	beaconCanonicFormat, err := types.NewBeaconBlockNotification(blockchainBlock.(interfaces.ReadOnlySignedBeaconBlock))
	require.NoError(t, err)

	var transactions [][]byte

	switch notification := beaconCanonicFormat.(type) {
	case *types.DenebBlockNotification:
		transactions = notification.Block.Body.ExecutionPayload.Transactions
	case *types.ElectraBlockNotification:
		transactions = notification.Block.Body.ExecutionPayload.Transactions
	default:
		t.Fatalf("unexpected notification type: %T", notification)
	}

	// Check beacon notification transactions exactly same as beacon BxBlock
	for i, txRaw := range transactions {
		notificationTx := new(ethtypes.Transaction)
		err = notificationTx.UnmarshalBinary(txRaw)
		require.NoError(t, err)

		tx := new(ethtypes.Transaction)
		err = tx.UnmarshalBinary(gbTransactions[i])
		require.NoError(t, err)

		require.Equal(t, notificationTx.Hash(), tx.Hash())
	}
}
