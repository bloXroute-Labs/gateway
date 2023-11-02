package syncmap

import (
	"hash/maphash"
	"math/rand"
	"os"
	"testing"

	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/google/uuid"
)

var maphashSeed maphash.Seed

func TestMain(m *testing.M) {
	// same seed for all tests
	maphashSeed = maphash.MakeSeed()
	uuid.SetRand(rand.New(rand.NewSource(0)))

	os.Exit(m.Run())
}

func oldStringHasher(seed maphash.Seed, key string) uint64 {
	var h maphash.Hash

	h.SetSeed(seed)

	// it always writes all of s and never fails; the count and error result are for implementing io.StringWriter.
	if _, err := h.WriteString(key); err != nil {
		log.Warn("Can't write string to hash", err)
	}

	return h.Sum64()
}

func BenchmarkOldStringHasher(b *testing.B) {
	strs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		strs[i] = uuid.NewString()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		oldStringHasher(maphashSeed, strs[i])
	}
}

func BenchmarkStringHasher(b *testing.B) {
	strs := make([]string, b.N)
	for i := 0; i < b.N; i++ {
		strs[i] = uuid.NewString()
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		StringHasher(maphashSeed, strs[i])
	}
}
