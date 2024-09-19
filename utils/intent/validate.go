package intent

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/ethereum/go-ethereum/common"
	"github.com/ethereum/go-ethereum/crypto"
)

var (
	// ErrInvalidSignature is returned when the signature is invalid
	ErrInvalidSignature = errors.New("invalid signature")
	// ErrInvalidAddress is returned when the address is not valid hex address
	ErrInvalidAddress = errors.New("invalid address")
	// ErrInvalidHash is returned when the hash is not valid Keccak256Hash
	ErrInvalidHash = errors.New("invalid hash")
	// ErrInvalidSignatureLength is returned when the signature length is not equal to ECDSASignatureLen
	ErrInvalidSignatureLength = errors.New("invalid signature length")
	// ErrHashMismatch is returned when the provided hash does not match the calculated hash
	ErrHashMismatch = errors.New("hash mismatch")
)

// ValidateHashAndSignature validates the hash and signature
func ValidateHashAndSignature(address string, hash, signature, data []byte) error {
	if !common.IsHexAddress(address) {
		return ErrInvalidAddress
	}

	if len(hash) != types.Keccak256HashLen {
		return ErrInvalidHash
	}

	if len(signature) != types.ECDSASignatureLen {
		return ErrInvalidSignatureLength
	}

	pubKey, err := crypto.SigToPub(hash, signature)
	if err != nil {
		return fmt.Errorf("failed to validate signature: %w", err)
	}

	if !bytes.Equal(crypto.PubkeyToAddress(*pubKey).Bytes(), common.HexToAddress(address).Bytes()) {
		return ErrInvalidSignature
	}

	if !bytes.Equal(crypto.Keccak256Hash(data).Bytes(), hash) {
		return ErrHashMismatch
	}

	return nil
}
