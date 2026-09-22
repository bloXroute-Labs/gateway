package types

import (
	"encoding/hex"
	"fmt"
	"sync"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/test/fixtures"
)

func prepareEthTx(b *testing.B) *EthTransaction {
	content, err := hex.DecodeString(fixtures.LegacyTransaction)
	if err != nil {
		b.Fatal(err)
	}
	return NewEthTransactionFromBytes(content, EmptySender)
}

func prepareDecodedEthTx(b *testing.B) *EthTransaction {
	ethTx := prepareEthTx(b)
	if _, err := ethTx.Tx(); err != nil {
		b.Fatal(err)
	}
	return ethTx
}

// --- Single-threaded: first call (includes RLP decode + cache write) ---

func BenchmarkEthTx_Nonce_FirstCall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ethTx := prepareEthTx(b)
		_, _ = ethTx.Nonce()
	}
}

func BenchmarkEthTx_Type_FirstCall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ethTx := prepareEthTx(b)
		_, _ = ethTx.Type()
	}
}

func BenchmarkEthTx_ChainID_FirstCall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ethTx := prepareEthTx(b)
		_, _ = ethTx.ChainID()
	}
}

func BenchmarkEthTx_From_FirstCall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ethTx := prepareEthTx(b)
		_, _ = ethTx.From()
	}
}

func BenchmarkEthTx_Fields_FirstCall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ethTx := prepareEthTx(b)
		_, _ = ethTx.Fields(AllFields)
	}
}

func BenchmarkEthTx_Filters_FirstCall(b *testing.B) {
	for i := 0; i < b.N; i++ {
		ethTx := prepareEthTx(b)
		_, _ = ethTx.Filters()
	}
}

// --- Single-threaded: cached read (the hot path) ---

func BenchmarkEthTx_Nonce_Cached(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ethTx.Nonce()
	}
}

func BenchmarkEthTx_Type_Cached(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ethTx.Type()
	}
}

func BenchmarkEthTx_ChainID_Cached(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ethTx.ChainID()
	}
}

func BenchmarkEthTx_From_Cached(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	// warm up sender cache
	_, _ = ethTx.From()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ethTx.From()
	}
}

func BenchmarkEthTx_Fields_Cached(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	// warm up fields cache
	_, _ = ethTx.Fields(AllFields)
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ethTx.Fields(AllFields)
	}
}

func BenchmarkEthTx_Filters_Cached(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	// warm up filters cache
	_, _ = ethTx.Filters()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ethTx.Filters()
	}
}

func BenchmarkEthTx_RawTx_Cached(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	// warm up binary cache
	_, _ = ethTx.RawTx()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ethTx.RawTx()
	}
}

func BenchmarkEthTx_RawTxHex_Cached(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	// warm up hexTx cache
	_, _ = ethTx.RawTxHex()
	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		_, _ = ethTx.RawTxHex()
	}
}

// --- Concurrent: cached read under contention ---

func BenchmarkEthTx_Nonce_Concurrent(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	for _, goroutines := range []int{4, 8, 16, 32} {
		b.Run(fmt.Sprintf("g%d", goroutines), func(b *testing.B) {
			b.SetParallelism(goroutines)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = ethTx.Nonce()
				}
			})
		})
	}
}

func BenchmarkEthTx_From_Concurrent(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	_, _ = ethTx.From() // warm up sender cache
	for _, goroutines := range []int{4, 8, 16, 32} {
		b.Run(fmt.Sprintf("g%d", goroutines), func(b *testing.B) {
			b.SetParallelism(goroutines)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = ethTx.From()
				}
			})
		})
	}
}

func BenchmarkEthTx_Fields_Concurrent(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	_, _ = ethTx.Fields(AllFields) // warm up fields cache
	for _, goroutines := range []int{4, 8, 16, 32} {
		b.Run(fmt.Sprintf("g%d", goroutines), func(b *testing.B) {
			b.SetParallelism(goroutines)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = ethTx.Fields(AllFields)
				}
			})
		})
	}
}

func BenchmarkEthTx_Filters_Concurrent(b *testing.B) {
	ethTx := prepareDecodedEthTx(b)
	_, _ = ethTx.Filters() // warm up filters cache
	for _, goroutines := range []int{4, 8, 16, 32} {
		b.Run(fmt.Sprintf("g%d", goroutines), func(b *testing.B) {
			b.SetParallelism(goroutines)
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					_, _ = ethTx.Filters()
				}
			})
		})
	}
}

// --- Concurrent: first-call race (multiple goroutines race to decode) ---

func BenchmarkEthTx_Nonce_RaceDecode(b *testing.B) {
	for _, goroutines := range []int{4, 8, 16, 32} {
		b.Run(fmt.Sprintf("g%d", goroutines), func(b *testing.B) {
			for i := 0; i < b.N; i++ {
				ethTx := prepareEthTx(b)
				var wg sync.WaitGroup
				wg.Add(goroutines)
				for g := 0; g < goroutines; g++ {
					go func() {
						defer wg.Done()
						_, _ = ethTx.Nonce()
					}()
				}
				wg.Wait()
			}
		})
	}
}
