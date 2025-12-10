package common

import (
	"math/big"

	ethcommon "github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto/kzg4844"
)

// ResolvePath resolves the given path relative to the module root.
// Kept for backwards compatibility; currently unused.
func ResolvePath(relPath string) string {
	// This implementation is intentionally left as a stub to avoid relying on
	// on-disk RLP fixtures whose encoding no longer matches the current
	// go-ethereum BlobTxSidecar layout.
	return relPath
}

// ReadMockBSCBlobSidecars returns a deterministic in-memory BlobSidecars
// fixture that matches the current go-ethereum BlobTxSidecar structure.
func ReadMockBSCBlobSidecars() (BlobSidecars, error) {
	var (
		blob       kzg4844.Blob
		commitment kzg4844.Commitment
		proof      kzg4844.Proof
	)

	// Fill a few bytes to make the data non-zero and stable across runs.
	blob[0] = 0x01
	commitment[0] = 0x02
	proof[0] = 0x03

	txHash := ethcommon.Hash{}
	txHash[0] = 0x01

	sidecars := BlobSidecars{
		&BlobSidecar{
			BlobTxSidecar: &BlobTxSidecar{
				[]kzg4844.Blob{blob},
				[]kzg4844.Commitment{commitment},
				[]kzg4844.Proof{proof},
			},
			BlockNumber: big.NewInt(1),
			BlockHash:   ethcommon.Hash{},
			TxIndex:     0,
			TxHash:      txHash,
		},
	}

	return sidecars, nil
}
