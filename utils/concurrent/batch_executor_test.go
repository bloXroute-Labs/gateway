package concurrent

import (
	"fmt"
	"math/rand"
	"sync"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

// Global map to track processed IDs.
var processedIDs sync.Map

// Pre-generate a large pool of unique IDs to ensure each goroutine has enough to choose from.
const totalUniqueIDs = 100000

var uniqueIDs []string

func init() {
	uniqueIDs = make([]string, totalUniqueIDs)
	for i := 0; i < totalUniqueIDs; i++ {
		uniqueIDs[i] = fmt.Sprintf("uniqueID-%d", i)
	}
}

func TestBatchExecutor(t *testing.T) {
	// Our custom executeFunc for testing.
	executeFunc := func(ids []string) ([]string, error) {
		for _, id := range ids {
			if _, loaded := processedIDs.LoadOrStore(id, struct{}{}); loaded {
				t.Errorf("ID %s has been processed more than once", id)
			}
		}
		// Mock processing and return results.
		var results []string
		for _, id := range ids {
			results = append(results, "processed "+id)
		}
		return results, nil
	}

	manager := NewBatchExecutor(executeFunc, syncmap.StringHasher, 1, time.Millisecond, time.Second)

	// Simulate concurrent processing.
	var wg sync.WaitGroup
	for i := 0; i < 5; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			ids := []string{fmt.Sprintf("id%d", i), "id3"} // Ensure "id3" is requested by multiple goroutines.
			results, _, err := manager.Execute(ids)
			if err != nil {
				t.Error("Error:", err)
			}
			for _, result := range results {
				expectedResult := "processed " + ids[0]
				if result != expectedResult && result != "processed id3" {
					t.Errorf("Unexpected result: got %v, want %v or 'processed id3'", result, expectedResult)
				}
			}
		}(i)
	}

	wg.Wait()
}

func BenchmarkBatchExecutor(b *testing.B) {
	executeFunc := func(ids []string) ([]string, error) {
		results := make([]string, len(ids))
		copy(results, ids) // Use copy function to copy elements from ids to results.
		return results, nil
	}

	// Pre-generate non-unique IDs outside the timed section.
	totalIDs := 1000

	percentages := []int{25, 50, 75} // Percentages of repeated IDs to test.

	for _, percentage := range percentages {
		b.Run(fmt.Sprintf("%d%%_repeat", percentage), func(b *testing.B) {
			manager := NewBatchExecutor(executeFunc, syncmap.StringHasher, totalIDs, 300*time.Microsecond, time.Second)

			nonUniqueCount := totalIDs * percentage / 100
			nonUniqueIDs := make([]string, nonUniqueCount)
			for i := 0; i < nonUniqueCount; i++ {
				nonUniqueIDs[i] = fmt.Sprintf("nonUniqueID-%d", i)
			}

			b.ResetTimer()
			b.RunParallel(func(pb *testing.PB) {
				for pb.Next() {
					// Select a subset of unique IDs for this goroutine.
					startIndex := rand.Intn(totalUniqueIDs - totalIDs) // Ensure room for totalIDs.
					idsSubset := append([]string(nil), uniqueIDs[startIndex:startIndex+(totalIDs-nonUniqueCount)]...)
					idsSubset = append(idsSubset, nonUniqueIDs...) // Mix in non-unique IDs.

					// Measure only the time taken by ProcessIDs.
					results, _, err := manager.Execute(idsSubset)
					if err != nil {
						b.Fatalf("Benchmark failed with error: %s", err)
					}

					for i, id := range idsSubset {
						expectedResult := id
						if results[i] != expectedResult {
							b.Fatalf("Unexpected result: got %v, want %v", results[i], expectedResult)
						}
					}
				}
			})
		})
	}
}
