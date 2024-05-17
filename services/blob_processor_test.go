package services

import (
	"encoding/hex"
	"os"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/eth"
	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/ethereum/go-ethereum/rlp"
	ethpb "github.com/prysmaticlabs/prysm/v5/proto/prysm/v1alpha1"
	"github.com/stretchr/testify/require"
)

func TestBlobProcessor_BlobTypesConverting(t *testing.T) {
	blobProcessor := blobProcessor{}

	TxHash := []byte{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}
	compressedSidecar := &types.CompressedEthBlobSidecar{
		Index:                    1,
		TxHash:                   TxHash,
		KzgCommitment:            []byte{0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20},
		KzgProof:                 []byte{0x21, 0x22, 0x23, 0x24, 0x25, 0x26, 0x27, 0x28},
		SignedBlockHeader:        &ethpb.SignedBeaconBlockHeader{},
		CommitmentInclusionProof: [][]byte{{0x29, 0x2A, 0x2B, 0x2C, 0x2D, 0x2E, 0x2F, 0x30}},
	}

	fullBlob := []byte{0x31, 0x32, 0x33, 0x34, 0x35, 0x36, 0x37, 0x38}

	ethSidecar := blobProcessor.compressedBlobSidecarToEthBlobSidecar(compressedSidecar, fullBlob)

	require.Equal(t, compressedSidecar.Index, ethSidecar.Index)
	require.Equal(t, compressedSidecar.KzgCommitment, ethSidecar.KzgCommitment)
	require.Equal(t, compressedSidecar.KzgProof, ethSidecar.KzgProof)
	require.Equal(t, compressedSidecar.SignedBlockHeader, ethSidecar.SignedBlockHeader)
	require.Equal(t, compressedSidecar.CommitmentInclusionProof, ethSidecar.CommitmentInclusionProof)
	require.Equal(t, fullBlob, ethSidecar.Blob)

	compressedFromOriginalSidecar := blobProcessor.ethBlobSidecarToCompressedBlobSidecar(ethSidecar, TxHash)

	require.Equal(t, ethSidecar.Index, compressedFromOriginalSidecar.Index)
	require.Equal(t, TxHash, compressedFromOriginalSidecar.TxHash)
	require.Equal(t, ethSidecar.KzgCommitment, compressedFromOriginalSidecar.KzgCommitment)
	require.Equal(t, ethSidecar.KzgProof, compressedFromOriginalSidecar.KzgProof)
	require.Equal(t, ethSidecar.SignedBlockHeader, compressedFromOriginalSidecar.SignedBlockHeader)
	require.Equal(t, ethSidecar.CommitmentInclusionProof, compressedFromOriginalSidecar.CommitmentInclusionProof)
}

func TestBlobProcessor_CompresDecompress(t *testing.T) {
	var sidecar ethpb.BlobSidecar
	data, err := os.ReadFile("../types/test_data/blob.ssz")
	require.NoError(t, err)
	err = sidecar.UnmarshalSSZ(data)
	require.NoError(t, err)

	blobCompressorStorage := NewBlobCompressorStorage().(*blobCompressorStorage)

	txStore := NewEthTxStore(&utils.MockClock{}, 30*time.Second, 30*time.Second, 30*time.Second,
		NewEmptyShortIDAssigner(), NewHashHistory("seenTxs", 30*time.Minute), nil, sdnmessage.BlockchainNetworks{testNetworkNum: &blockchainNetwork}, newTestBloomFilter(t), blobCompressorStorage)

	blobProcessor := NewBlobProcessor(txStore, nil).(*blobProcessor)

	castedExpecetedKZGCommitment := kzg4844.Commitment{}
	copy(castedExpecetedKZGCommitment[:], sidecar.KzgCommitment)

	castedExpectedBlob := kzg4844.Blob{}
	copy(castedExpectedBlob[:], sidecar.Blob)

	// create a BlobTypeTx with KZG commitments and blobs
	commitments := []kzg4844.Commitment{{0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20}, castedExpecetedKZGCommitment}
	blobs := []kzg4844.Blob{{0x39, 0x3A, 0x3B, 0x3C, 0x3D, 0x3E, 0x3F, 0x40}, castedExpectedBlob}
	tx := newBlobTypeTx(commitments, blobs)
	txHash := tx.Hash().Bytes()
	txHashCasted, _ := types.NewSHA256Hash(txHash)
	content, _ := rlp.EncodeToBytes(tx)

	// check the flow of adding a tx to the tx store
	txStore.Add(txHashCasted, content, types.ShortIDEmpty, network.EthMainnetChainID, false, types.TFWithSidecar, time.Now(), tx.ChainId().Int64(), types.EmptySender)

	// check that tx it's possible to lookup tx by kzg commitment
	storedTx, err := txStore.GetTxByKzgCommitment(hex.EncodeToString(commitments[0][:]))
	require.NoError(t, err)
	require.Equal(t, txHash, storedTx.Hash().Bytes())

	converter := eth.Converter{}
	initialBxBeaconMessage, err := converter.BeaconMessageToBDN(&sidecar)
	require.NoError(t, err)
	require.Equal(t, types.BxBeaconMessageTypeEthBlob, initialBxBeaconMessage.Type)

	// test the compression
	compressedBxBeaconMessage, err := blobProcessor.compressForBDN(initialBxBeaconMessage)
	require.NoError(t, err)

	// check that the message was compressed
	require.Equal(t, types.BxBeaconMessageTypeCompressedEthBlob, compressedBxBeaconMessage.Type)
	require.NotEqual(t, initialBxBeaconMessage.Data, compressedBxBeaconMessage.Data)

	// check that inside the compressed message there is a reference to the tx hash
	compressedEthSidecar := types.CompressedEthBlobSidecar{}
	err = compressedEthSidecar.UnmarshalSSZ(compressedBxBeaconMessage.Data)
	require.NoError(t, err)
	require.Equal(t, txHash, compressedEthSidecar.TxHash)

	// create a new beacon message with the compressed data to simulate message from BDN
	beaconMessage := bxmessage.NewBeaconMessage(
		compressedBxBeaconMessage.Hash,
		compressedBxBeaconMessage.BlockHash,
		compressedBxBeaconMessage.Type,
		compressedBxBeaconMessage.Data,
		compressedBxBeaconMessage.Index,
		compressedBxBeaconMessage.Slot,
		network.EthMainnetChainID,
	)

	// check that the compressed message can be decompressed
	decompressedBxMessage, err := blobProcessor.decompressFromBDN(beaconMessage)
	require.NoError(t, err)

	// check that the decompressed message is the same as the original
	require.Equal(t, initialBxBeaconMessage, decompressedBxMessage)

	resultSidecar := ethpb.BlobSidecar{}
	err = resultSidecar.UnmarshalSSZ(decompressedBxMessage.Data)
	require.NoError(t, err)

	// check that the decompressed sidecar is the same as the original
	require.Equal(t, sidecar.Blob, resultSidecar.Blob)
}
