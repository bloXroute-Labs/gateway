package hasher

import (
	"hash/maphash"

	"github.com/bloXroute-Labs/bxcommon-go/v2/syncmap"
	ethcommon "github.com/ethereum/go-ethereum/common"
)

// EthCommonHasher hasher function to hash EthCommonHasher
func EthCommonHasher(seed maphash.Seed, key ethcommon.Hash) uint64 {
	return syncmap.StringHasher(seed, key.String())
}
