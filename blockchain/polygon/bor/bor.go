// Package bor is a monkey patching of helper methods from https://github.com/maticnetwork/bor
// in the future we probably would move to use bor as a package directly
// but for the moment no need to have it as a requirement whole package.
package bor

import (
	"fmt"
	"io"
	"math/big"

	"golang.org/x/crypto/sha3"

	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/core/types"
	"github.com/ethereum/go-ethereum/crypto"
	"github.com/ethereum/go-ethereum/rlp"
)

const extraSeal = 65 // fixed number of extra-data suffix bytes reserved for signer seal

type runState uint8

const (
	stateIdle = runState(iota)
	stateBooting
	stateRunning
)

// Ecrecover extracts the Ethereum account address from a signed header.
func Ecrecover(header *types.Header) (common.Address, error) {
	// Retrieve the signature from the header extra-data
	if len(header.Extra) < extraSeal {
		// block's extra-data section doesn't seem to contain a 65 byte secp256k1 signature.
		return common.Address{}, fmt.Errorf("extra-data too short: %d < %d", len(header.Extra), extraSeal)
	}

	signature := header.Extra[len(header.Extra)-extraSeal:]

	sealed, err := SealHash(header)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to seal header hash: %w", err)
	}

	// Recover the public key and the Ethereum address
	pubKey, err := crypto.Ecrecover(sealed.Bytes(), signature)
	if err != nil {
		return common.Address{}, fmt.Errorf("failed to recover public key from signature: %w", err)
	}

	var signer common.Address

	copy(signer[:], crypto.Keccak256(pubKey[1:])[12:])

	return signer, nil
}

// SealHash returns the hash of a block prior to it being sealed.
func SealHash(header *types.Header) (hash common.Hash, err error) {
	hasher := sha3.NewLegacyKeccak256()
	if err = encodeSigHeader(hasher, header); err != nil {
		return common.Hash{}, err
	}

	hasher.Sum(hash[:0])

	return hash, nil
}

func encodeSigHeader(w io.Writer, header *types.Header) error {
	enc := []interface{}{
		header.ParentHash,
		header.UncleHash,
		header.Coinbase,
		header.Root,
		header.TxHash,
		header.ReceiptHash,
		header.Bloom,
		header.Difficulty,
		header.Number,
		header.GasLimit,
		header.GasUsed,
		header.Time,
		header.Extra[:len(header.Extra)-65], // Yes, this will panic if extra is too short
		header.MixDigest,
		header.Nonce,
	}

	if IsJaipur(header.Number) {
		if header.BaseFee != nil {
			enc = append(enc, header.BaseFee)
		}
	}

	return rlp.Encode(w, enc)
}

// isForked returns whether a fork scheduled at block s is active at the given head block.
func isForked(s, head *big.Int) bool {
	if s == nil || head == nil {
		return false
	}
	return s.Cmp(head) <= 0
}

// IsJaipur returns whether a fork scheduled at mainnet jaipur block is active at the given head block.
func IsJaipur(number *big.Int) bool {
	return isForked(
		big.NewInt(23850000), // mainnet jaipur block
		number,
	)
}
