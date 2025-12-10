package common

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
)

// This is a BSC-specific BlobSidecar implementation
// https://github.com/bnb-chain/bsc/blob/256d8811f441c29cb0812943dc660ac13192829c/core/types/blob_sidecar.go#L33

// BlobSidecars is a slice of BlobTxSidecars that can be encoded to an index
type BlobSidecars []*BlobSidecar

// BlobSidecar is a BlobTxSidecar with additional metadata
type BlobSidecar struct {
	BlobTxSidecar *BlobTxSidecar
	BlockNumber   *big.Int    `json:"blockNumber"`
	BlockHash     common.Hash `json:"blockHash"`
	TxIndex       uint64      `json:"transactionIndex"`
	TxHash        common.Hash `json:"transactionHash"`
}

// BlobTxSidecar contains the blobs of a blob transaction.
type BlobTxSidecar struct {
	Blobs       []kzg4844.Blob       // Blobs needed by the blob pool
	Commitments []kzg4844.Commitment // Commitments needed by the blob pool
	Proofs      []kzg4844.Proof      // Proofs needed by the blob pool
}
