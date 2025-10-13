package hasher

import (
	"hash/maphash"

	"github.com/bloXroute-Labs/bxcommon-go/syncmap"
	bxtypes "github.com/bloXroute-Labs/bxcommon-go/types"
	types "github.com/bloXroute-Labs/gateway/v2/types"
	ethcommon "github.com/ethereum/go-ethereum/common"
)

// EthCommonHasher hasher function to hash EthCommonHasher
func EthCommonHasher(seed maphash.Seed, key ethcommon.Hash) uint64 {
	return syncmap.StringHasher(seed, key.String())
}

// NodeIDHasher hasher function for bxtypes.NodeID key type.
// converts bxtypes.NodeID to string and returns Sum64 uint64
func NodeIDHasher(seed maphash.Seed, key bxtypes.NodeID) uint64 {
	return syncmap.StringHasher(seed, string(key))
}

// SHA256HashHasher hashes types.SHA256Hash by using its string representation
func SHA256HashHasher(seed maphash.Seed, key types.SHA256Hash) uint64 {
	return syncmap.StringHasher(seed, key.String())
}
