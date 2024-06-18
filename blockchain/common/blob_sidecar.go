package common

import (
	"math/big"

	"github.com/ethereum/go-ethereum/common"
	ethTypes "github.com/ethereum/go-ethereum/core/types"
)

//! This is BSC specific BlobSidecar implementation
//! code taken from github.com/bnb-chain/bsc

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
