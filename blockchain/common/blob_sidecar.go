package common

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
)

// This is a BSC-specific BlobSidecar implementation
// https://github.com/bnb-chain/bsc/blob/256d8811f441c29cb0812943dc660ac13192829c/core/types/blob_sidecar.go#L33

// BlobSidecars is a slice of BlobTxSidecars that can be encoded to an index
type BlobSidecars []*BlobSidecar

// BlobSidecar is a BlobTxSidecar with additional metadata
type BlobSidecar struct {
	BlobTxSidecar *ethTypes.BlobTxSidecar
	BlockNumber   *big.Int    `json:"blockNumber"`
	BlockHash     common.Hash `json:"blockHash"`
	TxIndex       uint64      `json:"transactionIndex"`
	TxHash        common.Hash `json:"transactionHash"`
}
