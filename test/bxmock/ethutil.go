package bxmock

import (
	"github.com/ethereum/go-ethereum/common"
	ethtypes "github.com/ethereum/go-ethereum/core/types"
	"golang.org/x/crypto/sha3"
	"hash"
	"math/rand"
)

// GenerateBloom randomly generates a bloom
func GenerateBloom() ethtypes.Bloom {
	var bloom ethtypes.Bloom
	_, _ = rand.Read(bloom[:])
	return bloom
}

// GenerateBlockNonce randomly generates a block nonce
func GenerateBlockNonce() ethtypes.BlockNonce {
	var blockNonce ethtypes.BlockNonce
	_, _ = rand.Read(blockNonce[:])
	return blockNonce
}

// GenerateAddress randomly generates an address
func GenerateAddress() common.Address {
	var address common.Address
	_, _ = rand.Read(address[:])
	return address
}

// TestHasher is the helper tool for transaction/receipt list hashing (taken from Geth test cases)
type TestHasher struct {
	hasher hash.Hash
}

// NewTestHasher creates a new TestHasher
func NewTestHasher() *TestHasher {
	return &TestHasher{hasher: sha3.NewLegacyKeccak256()}
}

// Reset resets the test hasher to its initial state
func (h *TestHasher) Reset() {
	h.hasher.Reset()
}

// Update updates test hasher values
func (h *TestHasher) Update(key, val []byte) {
	h.hasher.Write(key)
	h.hasher.Write(val)
}

// Hash returns an Ethereum common hash
func (h *TestHasher) Hash() common.Hash {
	return common.BytesToHash(h.hasher.Sum(nil))
}
