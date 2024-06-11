package intent

import (
	"bytes"
	"errors"
	"fmt"

	"github.com/bloXroute-Labs/gateway/v2/bxmessage"
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
)

// ValidateSignature validates the signature
func ValidateSignature(address string, hash, signature []byte) error {
	if !common.IsHexAddress(address) {
		return ErrInvalidAddress
	}

	if len(hash) != bxmessage.Keccak256HashLen {
		return ErrInvalidHash
	}

	if len(signature) != bxmessage.ECDSASignatureLen {
		return ErrInvalidSignatureLength
	}

	pubKey, err := crypto.SigToPub(hash, signature)
	if err != nil {
		return fmt.Errorf("failed to validate signature: %w", err)
	}

	if !bytes.Equal(crypto.PubkeyToAddress(*pubKey).Bytes(), common.HexToAddress(address).Bytes()) {
		return ErrInvalidSignature
	}

	return nil
}
