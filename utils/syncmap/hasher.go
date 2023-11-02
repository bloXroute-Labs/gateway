package syncmap

import (
	"hash/maphash"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// Hasher type of hasher functions
type Hasher[K comparable] func(maphash.Seed, K) uint64

// AccountIDHasher hasher function for AccountIDHasher key type.
// converts AccountIDHasher to string and returns Sum64 uint64
func AccountIDHasher(seed maphash.Seed, key types.AccountID) uint64 {
	return StringHasher(seed, string(key))
}

// EthCommonHasher hasher function to hash EthCommonHasher
func EthCommonHasher(seed maphash.Seed, key ethcommon.Hash) uint64 {
	return StringHasher(seed, key.String())
}

// NodeIDHasher hasher function for types.NodeID key type.
// converts types.NodeID to string and returns Sum64 uint64
func NodeIDHasher(seed maphash.Seed, key types.NodeID) uint64 {
	return StringHasher(seed, string(key))
}

// SHA256HashHasher hasher function for types.SHA256Hash key type.
// converts types.SHA256Hash to string and returns Sum64 uint64
func SHA256HashHasher(seed maphash.Seed, key types.SHA256Hash) uint64 {
	return maphash.Bytes(seed, key[:])
}

// StringHasher writes string hash and returns sum64
func StringHasher(seed maphash.Seed, key string) uint64 {
	return maphash.String(seed, key)
}
