package syncmap_test

import (
	"strconv"
	"sync"
	"testing"

	"github.com/bloXroute-Labs/gateway/v2/sdnmessage"
	"github.com/bloXroute-Labs/gateway/v2/types"
	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

const goroutineCount = 100

func BenchmarkSyncMap(b *testing.B) {
	b.Run("sync.Map", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sm := new(sync.Map)
			var wg sync.WaitGroup

			// Spawn multiple goroutines to perform concurrent read and write operations
			for j := 0; j < goroutineCount; j++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()

					for k := 0; k < 1000; k++ {
						key := strconv.Itoa(id*1000 + k)
						sm.Store(key, id)
						sm.Load(key)
					}
				}(j)
			}

			// Wait for all goroutines to finish
			wg.Wait()
		}
	})

	b.Run("syncmap.SyncMap", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sm := syncmap.NewStringMapOf[int]()
			var wg sync.WaitGroup

			// Spawn multiple goroutines to perform concurrent read and write operations
			for j := 0; j < goroutineCount; j++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()

					for k := 0; k < 1000; k++ {
						key := strconv.Itoa(id*1000 + k)
						sm.Store(key, id)
						sm.Load(key)
					}
				}(j)
			}

			// Wait for all goroutines to finish
			wg.Wait()
		}
	})

	b.Run("sync.Map[joint-keys]", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sm := new(sync.Map)
			var wg sync.WaitGroup

			// Spawn multiple goroutines to perform concurrent read and write operations
			for j := 0; j < goroutineCount; j++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()

					for k := 0; k < 1000; k++ {
						key := strconv.Itoa(1000 + k)
						sm.Store(key, id)
						sm.Load(key)
					}
				}(j)
			}

			// Wait for all goroutines to finish
			wg.Wait()
		}
	})

	b.Run("syncmap.SyncMap[joint-keys]", func(b *testing.B) {
		for i := 0; i < b.N; i++ {
			sm := syncmap.NewStringMapOf[int]()
			var wg sync.WaitGroup

			// Spawn multiple goroutines to perform concurrent read and write operations
			for j := 0; j < goroutineCount; j++ {
				wg.Add(1)
				go func(id int) {
					defer wg.Done()

					for k := 0; k < 1000; k++ {
						key := strconv.Itoa(1000 + k)
						sm.Store(key, id)
						sm.Load(key)
					}
				}(j)
			}

			// Wait for all goroutines to finish
			wg.Wait()
		}
	})
}

func BenchmarkSyncMapRead(b *testing.B) {
	b.Run("sync.Map", func(b *testing.B) {
		sm := new(sync.Map)

		// Initialize the sync.Map with keys and values
		for i := 0; i < 10000; i++ {
			key := strconv.Itoa(i)
			sm.Store(key, i)
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup

			// Spawn multiple goroutines to perform concurrent read operations
			for j := 0; j < 10; j++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()

					for k := 0; k < 1000; k++ {
						key := strconv.Itoa(i*1000 + k)
						sm.Load(key)
					}
				}(j)
			}

			// Wait for all goroutines to finish
			wg.Wait()
		}
	})

	b.Run("syncmap.SyncMap", func(b *testing.B) {
		sm := syncmap.NewStringMapOf[int]()

		// Initialize the syncmap.SyncMap with keys and values
		for i := 0; i < 10000; i++ {
			key := strconv.Itoa(i)
			sm.Store(key, i)
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup

			// Spawn multiple goroutines to perform concurrent read operations
			for j := 0; j < 10; j++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()

					for k := 0; k < 1000; k++ {
						key := strconv.Itoa(i*1000 + k)
						sm.Load(key)
					}
				}(j)
			}

			// Wait for all goroutines to finish
			wg.Wait()
		}
	})
}

func BenchmarkSyncMapReadWithConversion(b *testing.B) {
	b.Run("sync.Map", func(b *testing.B) {
		sm := new(sync.Map)

		// Initialize the sync.Map with keys and values
		for i := 0; i < 10000; i++ {
			key := types.AccountID(strconv.Itoa(i))
			sm.Store(key, &sdnmessage.Account{})
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup

			// Spawn multiple goroutines to perform concurrent read operations
			for j := 0; j < 10; j++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()

					for k := 0; k < 1000; k++ {
						key := types.AccountID(strconv.Itoa(i*1000 + k))
						if value, ok := sm.Load(key); ok {
							account := value.(*sdnmessage.Account)
							_ = account
						}
					}
				}(j)
			}

			// Wait for all goroutines to finish
			wg.Wait()
		}
	})

	b.Run("syncmap.SyncMap", func(b *testing.B) {
		sm := syncmap.NewTypedMapOf[types.AccountID, *sdnmessage.Account](syncmap.AccountIDHasher)

		// Initialize the syncmap.SyncMap with keys and values
		for i := 0; i < 10000; i++ {
			key := types.AccountID(strconv.Itoa(i))
			sm.Store(key, &sdnmessage.Account{})
		}

		b.ResetTimer()

		for i := 0; i < b.N; i++ {
			var wg sync.WaitGroup

			// Spawn multiple goroutines to perform concurrent read operations
			for j := 0; j < 10; j++ {
				wg.Add(1)
				go func(i int) {
					defer wg.Done()

					for k := 0; k < 1000; k++ {
						key := types.AccountID(strconv.Itoa(i*1000 + k))
						account, _ := sm.Load(key)
						_ = account
					}
				}(j)
			}

			// Wait for all goroutines to finish
			wg.Wait()
		}
	})
}
