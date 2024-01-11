package services

import (
	"context"
	"math/rand"
	"os"
	"sync"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/test"
	"github.com/bloXroute-Labs/gateway/v2/utils"
	"github.com/stretchr/testify/assert"
)

const (
	capacity             = 1e6
	bloomFilterQueueSize = 500
)

func TestMain(m *testing.M) {
	rand.Seed(1)
	os.Exit(m.Run())
}

func TestBloomFilter_counter(t *testing.T) {
	tmp, err := os.CreateTemp("", "")
	assert.NoError(t, err)
	defer os.Remove(tmp.Name())

	counter := uint32(3241)
	err = writeCounterToFile(counter, tmp.Name())
	assert.NoError(t, err)
	err = tmp.Close()
	assert.NoError(t, err)

	tmp, err = os.Open(tmp.Name())
	assert.NoError(t, err)
	counterFromFile, err := readCounterFromFile(tmp.Name())
	assert.NoError(t, err)

	assert.Equal(t, counter, counterFromFile)
}

func TestBloomFilter(t *testing.T) {
	// Complex test that checks if bloom filter works as expected.
	// It adds 1000 hashes to the bloom filter and checks if they are there.
	// Also it checks that new hashes added during storing bloom filter on disk because at this point we are using queue to not block Add method.

	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.Remove(tmpDir)

	mockClock := utils.MockClock{}

	ctx := context.Background()
	bf, _ := newBloomFilter(ctx, &mockClock, 1*time.Hour, tmpDir, capacity, bloomFilterQueueSize)

	hashCount := bloomFilterQueueSize / 10
	hashes := generateHashes(hashCount)

	for _, hash := range hashes {
		bf.Add(hash)
	}

	test.WaitChanConsumed(t, bf.queue)

	for i, hash := range hashes {
		assert.True(t, bf.Check(hash), i)
	}

	assert.Equal(t, uint32(hashCount), bf.counter.Load())

	// When storing bloom filter on disk we are using queue temporarily to not block adding new hashes.
	// At this time check will return false. This is tradeoff.

	ctx, cancel := context.WithCancel(context.Background())
	stopNewTxChan := make(chan struct{})

	// adding new hashes during storing bloom filter on disk
	// stop when store is done
	var newHashes [][]byte
	go func() {
		defer close(stopNewTxChan)

		for i := 0; i < bloomFilterQueueSize; i++ {
			select {
			case <-ctx.Done():
				return
			default:
				hash := make([]byte, 32)
				_, err := rand.Read(hash)
				assert.NoError(t, err)

				newHashes = append(newHashes, hash)
				bf.Add(hash)
			}
		}

		// Waiting until storeOnDisk finished if bloom was filled before
		<-ctx.Done()
	}()

	go func() {
		err := bf.storeOnDisk()
		assert.NoError(t, err)
		cancel()
	}()

	<-stopNewTxChan

	test.WaitChanConsumed(t, bf.queue)

	assert.Equal(t, uint32(hashCount+len(newHashes)), bf.counter.Load())

	// check if new hashes added during storing bloom filter on disk are in the bloom filter
	for i, hash := range newHashes {
		if !bf.Check(hash) {
			t.Error("hash not found", i)
			t.FailNow()
		}
	}
}

func TestBloomFilter_maybeSwitchFilters(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.Remove(tmpDir)

	mockClock := utils.MockClock{}

	ctx := context.Background()
	overflowPercent := 10
	overflow := bloomFilterQueueSize / overflowPercent
	cap := uint32(bloomFilterQueueSize - overflow) // don't want any queue overflow to not lose hashes
	bf, _ := newBloomFilter(ctx, &mockClock, 1*time.Hour, tmpDir, cap, bloomFilterQueueSize)

	newCapacity := int(cap) + overflow
	hashes := generateHashes(newCapacity)
	workers := 10
	part := int(newCapacity / workers)

	wg := &sync.WaitGroup{}
	for i := 0; i < workers; i++ {
		wg.Add(1)
		h := hashes[i*part : (i+1)*part]

		go func(hashes [][]byte, wg *sync.WaitGroup) {
			defer wg.Done()

			for _, hash := range hashes {
				bf.Add(hash)
			}
		}(h, wg)
	}

	wg.Wait()
	test.WaitChanConsumed(t, bf.queue)

	assert.Equal(t, uint32(overflow), bf.counter.Load(), "%d != %d", overflow, bf.counter.Load())

	// check every second hash because when hash is in previous bloom filter but not in current it will add it to current
	// which would cause another switch. So after add and full check we will lose last 10% of hashes
	// this is normal because we are checking exact same hash list after switch which is not the case in real use
	// https://excalidraw.com/#json=Kb-ebaoIQ_OvepePZVIqZ,44-bANvSaG6uYdaGvbdqJQ
	for i := 0; i < newCapacity/2; i++ {
		if !bf.Check(hashes[i*2]) {
			t.Error("hash not found", i*2)
			t.FailNow()
		}
	}
}

func TestBloomFilter_storeOnDisk(t *testing.T) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.Remove(tmpDir)

	mockClock := utils.MockClock{}

	ctx := context.Background()
	bf, _ := newBloomFilter(ctx, &mockClock, 1*time.Hour, tmpDir, capacity, bloomFilterQueueSize)

	// fill the bloom partially
	hashes := generateHashes(bloomFilterQueueSize)

	for _, hash := range hashes {
		bf.Add(hash)
	}

	test.WaitChanConsumed(t, bf.queue)

	bf.storeOnDisk()

	tmpDir2, err := os.MkdirTemp("", "")
	assert.NoError(t, err)
	defer os.Remove(tmpDir2)

	// read the bloom filter
	bf2, err := newBloomFilter(ctx, &mockClock, 1*time.Hour, tmpDir2, capacity, bloomFilterQueueSize)
	ctx.Done()
	assert.NoError(t, err)

	assert.Equal(t, len(bf.current.BitSet().Bytes()), len(bf2.current.BitSet().Bytes()))
	assert.Equal(t, cap(bf.current.BitSet().Bytes()), cap(bf2.current.BitSet().Bytes()))
}

func BenchmarkBloomFilter_Add(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(b, err)
	defer os.Remove(tmpDir)

	mockClock := utils.MockClock{}

	ctx := context.Background()
	bf, _ := newBloomFilter(ctx, &mockClock, 1*time.Hour, tmpDir, 66e6, 10000)
	bf.queue = make(chan []byte, b.N)

	hashes := generateHashes(b.N)

	b.ResetTimer()
	for _, hash := range hashes {
		bf.add(hash)
	}
}

func BenchmarkBloomFilter_Check(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(b, err)
	defer os.Remove(tmpDir)

	mockClock := utils.MockClock{}

	ctx := context.Background()
	bf, err := newBloomFilter(ctx, &mockClock, 1*time.Hour, tmpDir, 66e6, 10000)
	assert.NoError(b, err)

	// fill the bloom
	hashes := generateHashes(b.N)

	for _, hash := range hashes {
		bf.add(hash)
	}

	b.ResetTimer()
	for i := 0; i < b.N; i++ {
		bf.Check(hashes[i])
	}
}

func Benchmark_storeOnDisk(b *testing.B) {
	tmpDir, err := os.MkdirTemp("", "")
	assert.NoError(b, err)
	defer os.Remove(tmpDir)

	mockClock := utils.MockClock{}

	ctx := context.Background()
	bf, err := newBloomFilter(ctx, &mockClock, 1*time.Hour, tmpDir, 66e6, 10000)
	assert.NoError(b, err)

	hashes := generateHashes(b.N)
	for _, hash := range hashes {
		bf.add(hash)
	}

	b.ResetTimer()
	for i := 0; i <= b.N; i++ {
		bf.storeOnDisk()
	}
}

func generateHashes(count int) [][]byte {
	hashes := make([][]byte, count)
	for i := 0; i < count; i++ {
		hash := make([]byte, 32)
		_, _ = rand.Read(hash)

		hashes[i] = hash
	}

	return hashes
}
