package services

import (
	"encoding/hex"
	"testing"

	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
	"github.com/holiman/uint256"
	"github.com/stretchr/testify/require"

	"github.com/bloXroute-Labs/gateway/v2/blockchain/network"
)

func newBlobTypeTx(commitments []kzg4844.Commitment, blobs []kzg4844.Blob) *ethtypes.Transaction {
	nonce := uint64(1)
	pk, _ := crypto.GenerateKey()
	address := crypto.PubkeyToAddress(pk.PublicKey)
	chainIDBigInt := uint256.NewInt(network.EthMainnetChainID)
	unsignedTx := ethtypes.NewTx(&ethtypes.BlobTx{
		ChainID:    chainIDBigInt,
		Nonce:      nonce,
		GasTipCap:  uint256.NewInt(100),
		GasFeeCap:  uint256.NewInt(100),
		Gas:        0,
		To:         address,
		Value:      uint256.NewInt(1),
		Data:       []byte{},
		BlobFeeCap: uint256.NewInt(100),
		BlobHashes: []common.Hash{},
		Sidecar: &ethtypes.BlobTxSidecar{
			Blobs:       blobs,
			Commitments: commitments,
			Proofs:      []kzg4844.Proof{},
		},
		AccessList: ethtypes.AccessList{},
		V:          &uint256.Int{},
		R:          &uint256.Int{},
		S:          &uint256.Int{},
	})
	return unsignedTx
}

func TestBlobCompressorStorage(t *testing.T) {
	storage := NewBlobCompressorStorage().(*blobCompressorStorage)
	// casting to blobCompressorStorage to access the internal syncmap

	require.NotNil(t, storage.kzgCommitmentToTxHash)
	require.NotNil(t, storage.reverseMapForCleaning)
	require.Equal(t, 0, storage.kzgCommitmentToTxHash.Size())
	require.Equal(t, 0, storage.reverseMapForCleaning.Size())

	mockedCommitments := []kzg4844.Commitment{{0x11, 0x12, 0x13, 0x14, 0x15, 0x16, 0x17, 0x18}, {0x19, 0x1A, 0x1B, 0x1C, 0x1D, 0x1E, 0x1F, 0x20}}
	tx := newBlobTypeTx(mockedCommitments, []kzg4844.Blob{})
	txHashExpected := string(tx.Hash().Bytes())

	storage.StoreKzgCommitmentToTxHashRecords(tx)

	// for each commitment, there should be a mapping to the tx hash
	require.Equal(t, 2, storage.kzgCommitmentToTxHash.Size())
	// for each tx hash, there should be a mapping to the slice of commitments
	require.Equal(t, 1, storage.reverseMapForCleaning.Size())

	// check the mapping from commitment to tx hash
	for _, commitment := range mockedCommitments {
		commitmentStr := hex.EncodeToString(commitment[:])
		txHashFromMap, ok := storage.kzgCommitmentToTxHash.Load(commitmentStr)
		require.True(t, ok)
		require.Equal(t, txHashExpected, txHashFromMap)
	}

	// check the mapping from tx hash to commitments
	commitmentsFromMap, ok := storage.reverseMapForCleaning.Load(txHashExpected)
	require.True(t, ok)
	require.Equal(t, 2, len(commitmentsFromMap))
	for _, commitment := range mockedCommitments {
		commitmentStr := hex.EncodeToString(commitment[:])
		require.Contains(t, commitmentsFromMap, commitmentStr)
	}

	// check clearing the storage by tx hash
	require.Equal(t, 2, storage.kzgCommitmentToTxHash.Size())
	require.Equal(t, 1, storage.reverseMapForCleaning.Size())
	storage.RemoveByTxHash(txHashExpected)
	require.Equal(t, 0, storage.kzgCommitmentToTxHash.Size())
	require.Equal(t, 0, storage.reverseMapForCleaning.Size())

	// check full clearing of the storage
	storage.StoreKzgCommitmentToTxHashRecords(tx)
	require.Equal(t, 2, storage.kzgCommitmentToTxHash.Size())
	require.Equal(t, 1, storage.reverseMapForCleaning.Size())
	storage.Clear()
	require.Equal(t, 0, storage.kzgCommitmentToTxHash.Size())
	require.Equal(t, 0, storage.reverseMapForCleaning.Size())
}
