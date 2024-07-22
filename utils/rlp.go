package utils

import (
	"errors"
	"io"

	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/rlp"
)

// RLP errors
var (
	ErrInvalidRLP  = errors.New("invalid RLP")
	ErrRLPTooSmall = errors.New("RLP too small")
)

var minTxRLPSize = 100 // https://ethereum.stackexchange.com/a/39033

var (
	// RLP tree indexes
	blobHashesRLPTree  = []int{1, 10}
	blobsRLPTree       = []int{1, 1}
	commitmentsRLPTree = []int{1, 2}
	proofsRLPTree      = []int{1, 3}
)

// BlobHashesFromTxBinary returns the blob hashes from the transaction binary
func BlobHashesFromTxBinary(b []byte) ([][]byte, error) {
	// Only for blob without sidecar
	d, _, err := RLPTreeFetch(b, blobHashesRLPTree)

	return d, err
}

// TxSidecarBlobsFromBinary returns the blobs from the transaction binary
func TxSidecarBlobsFromBinary(b []byte) ([][]byte, error) {
	d, _, err := RLPTreeFetch(b, blobsRLPTree)

	return d, err
}

// TxSidecarCommitmentsFromBinary returns the commitments from the transaction binary
func TxSidecarCommitmentsFromBinary(b []byte) ([][]byte, error) {
	d, _, err := RLPTreeFetch(b, commitmentsRLPTree)

	return d, err
}

// TxSidecarProofsFromBinary returns the proofs from the transaction binary
func TxSidecarProofsFromBinary(b []byte) ([][]byte, error) {
	d, _, err := RLPTreeFetch(b, proofsRLPTree)

	return d, err
}

// TxCutSidecarFromBinary returns the transaction without sidecar
func TxCutSidecarFromBinary(b []byte) ([]byte, error) {
	if len(b) < minTxRLPSize {
		return nil, ErrRLPTooSmall
	}

	if b[0] != types.BlobTxType {
		return b, nil
	}

	var start uint64
	switch {
	case b[1] < 0xf8:
		start = 3 // one byte length
	default:
		start = 2 + uint64(b[1]-0xf7)
	}

	if b[start] < 0xc0 {
		return b, nil
	}

	var end uint64
	if b[start] < 0xf8 {
		end = start + uint64(b[start]-0xc0) + 1
	} else {
		if len(b) < int(start+1) {
			return nil, ErrInvalidRLP
		}

		lengthOfLength := uint64(b[start] - 0xf7)
		length, err := readSize(b[start+1:], b[start]-0xf7)
		if err != nil {
			return nil, err
		}
		end = start + lengthOfLength + length + 1
	}

	if len(b) < int(end) {
		return nil, ErrInvalidRLP
	}

	return append([]byte{types.BlobTxType}, b[start:end]...), nil
}

func readSize(b []byte, slen byte) (uint64, error) {
	switch slen {
	case 1:
		return uint64(b[0]), nil
	case 2:
		return uint64(b[0])<<8 | uint64(b[1]), nil
	case 3:
		return uint64(b[0])<<16 | uint64(b[1])<<8 | uint64(b[2]), nil
	case 4:
		return uint64(b[0])<<24 | uint64(b[1])<<16 | uint64(b[2])<<8 | uint64(b[3]), nil
	case 5:
		return uint64(b[0])<<32 | uint64(b[1])<<24 | uint64(b[2])<<16 | uint64(b[3])<<8 | uint64(b[4]), nil
	case 6:
		return uint64(b[0])<<40 | uint64(b[1])<<32 | uint64(b[2])<<24 | uint64(b[3])<<16 | uint64(b[4])<<8 | uint64(b[5]), nil
	case 7:
		return uint64(b[0])<<48 | uint64(b[1])<<40 | uint64(b[2])<<32 | uint64(b[3])<<24 | uint64(b[4])<<16 | uint64(b[5])<<8 | uint64(b[6]), nil
	case 8:
		return uint64(b[0])<<56 | uint64(b[1])<<48 | uint64(b[2])<<40 | uint64(b[3])<<32 | uint64(b[4])<<24 | uint64(b[5])<<16 | uint64(b[6])<<8 | uint64(b[7]), nil
	}

	return 0, ErrInvalidRLP
}

// TxRLPType returns the transaction RLP type
func TxRLPType(b []byte) (uint8, error) {
	if len(b) < minTxRLPSize {
		return 0, ErrRLPTooSmall
	}

	if b[0] >= 0xc0 {
		// Legacy transaction without envelop type is not changed
		return types.LegacyTxType, nil
	}

	if b[0] >= 0xb7 {
		// string with length more than 55 bytes
		return b[b[0]-0xb7+1], nil
	}

	if b[0] >= 0x80 {
		// string with length less or equal than 55 bytes
		return b[1], nil
	}

	// Binary, type first
	return b[0], nil
}

// TxRLPToBinary converts the transaction RLP to binary
func TxRLPToBinary(b []byte) ([]byte, error) {
	if len(b) < minTxRLPSize {
		return nil, ErrRLPTooSmall
	}

	if b[0] >= 0xc0 {
		// Legacy transaction without envelop type is not changed
		return b, nil
	}

	if b[0] >= 0xb7 {
		// string with length more than 55 bytes
		return b[b[0]-0xb7+1:], nil
	}

	if b[0] >= 0x80 {
		// string with length less or equal than 55 bytes
		return b[1:], nil
	}

	// Binary
	return b, nil
}

// RLPTreeFetch returns the RLP tree object
func RLPTreeFetch(b []byte, idxs []int) ([][]byte, int, error) {
	return rlpTreeFetch(b, idxs, 0, 0)
}

func rlpTreeFetch(b []byte, idxs []int, depth int, offset int) ([][]byte, int, error) {
	// TODO: iterrative approach over recursive

	result := make([][]byte, 0)

	if depth > 3 {
		return nil, 0, ErrInvalidRLP
	}

	for i := 0; ; i++ {
		if i >= 100 {
			return nil, 0, ErrInvalidRLP
		}

		k, c, r, err := rlp.Split(b)
		if err != nil {
			if err == io.ErrUnexpectedEOF {
				return result, offset, nil
			}
			return nil, 0, ErrInvalidRLP
		}

		if depth <= len(idxs)-1 && idxs[depth] == i {
			if k == rlp.List {
				data, offset, err := rlpTreeFetch(c, idxs, depth+1, offset)
				if err != nil {
					return nil, 0, err
				}

				if depth <= len(idxs)-1 && idxs[depth] == i {
					// Poping up
					return data, offset, nil
				}
			} else {
				return [][]byte{c}, offset, nil
			}
		}

		result = append(result, c)

		switch k {
		case rlp.Byte, rlp.String:
			offset += int(rlp.BytesSize(c))
		case rlp.List:
			offset += int(rlp.ListSize(uint64(len(c))))
		}

		b = r
	}
}
