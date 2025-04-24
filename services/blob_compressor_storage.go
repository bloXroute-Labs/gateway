package services

import (
	"encoding/hex"

	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/log"
)

// BlobCompressorStorage is an interface for storing and retrieving KzgCommitment to TxHash mappings
type BlobCompressorStorage interface {
	// AddKzgCommitmentToTxHash adds a mapping from KzgCommitment to TxHash
	StoreKzgCommitmentToTxHashRecords(eth *ethtypes.Transaction)
	KzgCommitmentToTxHash(kzgCommitment string) (string, bool)
	Clear()
	RemoveByTxHash(hash string)
}

type noOpBlobCompressorStorage struct{}

// StoreKzgCommitmentToTxHashRecords does nothing
func (bc *noOpBlobCompressorStorage) StoreKzgCommitmentToTxHashRecords(ethTx *ethtypes.Transaction) {
}

// Clear does nothing
func (bc *noOpBlobCompressorStorage) Clear() {
}

// KzgCommitmentToTxHash always returns false
func (bc *noOpBlobCompressorStorage) KzgCommitmentToTxHash(kzgCommitment string) (string, bool) {
	return "", false
}

// RemoveByTxHash does nothing
func (bc *noOpBlobCompressorStorage) RemoveByTxHash(string) {
}

// NewNoOpBlockCompressorStorage creates a new NoOpBlockCompressorStorage
func NewNoOpBlockCompressorStorage() BlobCompressorStorage {
	return &noOpBlobCompressorStorage{}
}

type blobCompressorStorage struct {
	kzgCommitmentToTxHash *syncmap.SyncMap[string, string]
	reverseMapForCleaning *syncmap.SyncMap[string, []string]
}

// NewBlobCompressorStorage creates a new BlobCompressorStorage
func NewBlobCompressorStorage() BlobCompressorStorage {
	return &blobCompressorStorage{
		kzgCommitmentToTxHash: syncmap.NewStringMapOf[string](),
		reverseMapForCleaning: syncmap.NewStringMapOf[[]string](),
	}
}

// Clear clears the storage
func (bc *blobCompressorStorage) Clear() {
	bc.kzgCommitmentToTxHash.Clear()
	bc.reverseMapForCleaning.Clear()
}

// RemoveByTxHash removes the KzgCommitment to TxHash mappings for the given TxHash
func (bc *blobCompressorStorage) RemoveByTxHash(hash string) {
	kzgCommitments, ok := bc.reverseMapForCleaning.Load(hash)
	if !ok {
		return
	}

	for _, kzg := range kzgCommitments {
		bc.kzgCommitmentToTxHash.Delete(kzg)
	}

	bc.reverseMapForCleaning.Delete(hash)
}

// KzgCommitmentToTxHash loads the value for the given key
func (bc *blobCompressorStorage) KzgCommitmentToTxHash(kzgCommitment string) (string, bool) {
	return bc.kzgCommitmentToTxHash.Load(kzgCommitment)
}

// StoreKzgCommitmentToTxHashRecords stores the KzgCommitment to TxHash mappings
func (bc *blobCompressorStorage) StoreKzgCommitmentToTxHashRecords(ethTx *ethtypes.Transaction) {
	if ethTx.BlobTxSidecar() == nil {
		// This should never happen
		log.Warn("BlobTxSidecar is nil when storing KzgCommitment to TxHash records")
		return
	}

	kzgCommitments := ethTx.BlobTxSidecar().Commitments
	if len(kzgCommitments) == 0 {
		// This should never happen
		log.Warn("No KzgCommitments found when storing KzgCommitment to TxHash records")
		return
	}

	encodedKzgCommitments := make([]string, len(kzgCommitments))
	txHashStr := string(ethTx.Hash().Bytes())

	for i, kzgCommitment := range kzgCommitments {
		encodedKzgComm := hex.EncodeToString(kzgCommitment[:])
		bc.kzgCommitmentToTxHash.Store(encodedKzgComm, txHashStr)
		encodedKzgCommitments[i] = encodedKzgComm
	}

	bc.reverseMapForCleaning.Store(txHashStr, encodedKzgCommitments)
}
