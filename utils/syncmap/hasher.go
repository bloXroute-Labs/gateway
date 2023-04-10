package syncmap

import (
	"hash/maphash"

	log "github.com/bloXroute-Labs/gateway/v2/logger"

	ethcommon "github.com/ethereum/go-ethereum/common"

	"github.com/bloXroute-Labs/gateway/v2/types"
)

// Hasher type of hasher functions
type Hasher[K comparable] func(maphash.Seed, K) uint64

// AccountIDHasher hasher function for AccountIDHasher key type.
// converts AccountIDHasher to string and returns Sum64 uint64
func AccountIDHasher(seed maphash.Seed, key types.AccountID) uint64 {
	return writeStringHash(seed, string(key))
}

// EthCommonHasher hasher function to hash EthCommonHasher
func EthCommonHasher(seed maphash.Seed, key ethcommon.Hash) uint64 {
	return writeStringHash(seed, key.String())
}

// writeStringHash writes string hash and returns sum64
func writeStringHash(seed maphash.Seed, key string) uint64 {
	var h maphash.Hash

	h.SetSeed(seed)

	// it always writes all of s and never fails; the count and error result are for implementing io.StringWriter.
	if _, err := h.WriteString(key); err != nil {
		log.Warn("Can't write string to hash", err)
	}

	return h.Sum64()
}
