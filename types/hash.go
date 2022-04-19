package types

import (
	"encoding/hex"
	"errors"
	"fmt"
	"github.com/ethereum/go-ethereum/crypto"
	"math/rand"
)

// SHA256HashLen is the byte length of SHA256 hashes
const SHA256HashLen = 32

// SHA256Hash represents a byte array containing SHA256 hash (e.g. transaction hash, block hash)
type SHA256Hash [SHA256HashLen]byte

// SHA256HashList represents hash list
type SHA256HashList []SHA256Hash

// NewSHA256Hash converts an existing byte array to a SHA256Hash
func NewSHA256Hash(b []byte) (SHA256Hash, error) {
	var hash SHA256Hash
	if len(b) != SHA256HashLen {
		return hash, errors.New("provided hash string is an incorrect length")
	}
	copy(hash[:], b)
	return hash, nil
}

// NewSHA256HashFromString parses a SHA256Hash from a serialized string format
func NewSHA256HashFromString(hashStr string) (SHA256Hash, error) {
	hashBytes, err := DecodeHex(hashStr)
	if err != nil {
		return SHA256Hash{}, fmt.Errorf("could not decode hash string: %v %v", hashStr, err)
	}

	return NewSHA256Hash(hashBytes)
}

// NewSHA256FromKeccak derives an SHA256Hash object using keccak hash on the provided byte array
func NewSHA256FromKeccak(b []byte) SHA256Hash {
	keccakHash := crypto.Keccak256(b)
	var hash SHA256Hash
	copy(hash[:], keccakHash)
	return hash
}

// GenerateSHA256Hash randomly generates a new SHA256Hash object
func GenerateSHA256Hash() SHA256Hash {
	var hash SHA256Hash
	_, _ = rand.Read(hash[:])
	return hash
}

// Bytes returns the underlying byte representation
func (s SHA256Hash) Bytes() []byte {
	return s[:]
}

// String dumps the SHA256Hash to a readable format
func (s SHA256Hash) String() string {
	return hex.EncodeToString(s[:])
}

// Format dumps the SHA256Hash to a readable format, optionally with a 0x prefix
func (s SHA256Hash) Format(prefix bool) string {
	if prefix {
		return fmt.Sprintf("%v%v", "0x", s)
	}
	return s.String()
}

// DecodeHex gets the bytes of a hexadecimal string, with or without its `0x` prefix
func DecodeHex(str string) ([]byte, error) {
	if len(str) > 2 && str[:2] == "0x" {
		str = str[2:]
	}
	return hex.DecodeString(str)
}
