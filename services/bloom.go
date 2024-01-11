package services

import (
	"bufio"
	"context"
	"fmt"
	"os"
	"path"
	"strconv"
	"sync"
	"sync/atomic"
	"time"

	"github.com/bits-and-blooms/bloom/v3"
	log "github.com/bloXroute-Labs/gateway/v2/logger"
	"github.com/bloXroute-Labs/gateway/v2/utils"
)

const (
	bloomFilterDirName            = "bloom"
	currentBloomFileName          = "current.bloom"
	previousBloomFileName         = "previous.bloom"
	counterBloomFileName          = "counter.bloom"
	bloomFalsePositiveProbability = 1e-6
)

// BloomFilter interface
type BloomFilter interface {
	// Check checks if value is stored in either current or previous bloom filters,
	// adds it to current bf if it's not there,
	// can perform filters switch and returns check result
	Check(val []byte) bool
	Add(val []byte)
}

// NoOpBloomFilter is a placeholder bloom filter
type NoOpBloomFilter struct {
	BloomFilter
}

// Check does nothing
func (no NoOpBloomFilter) Check(_ []byte) bool {
	return false
}

// Add does nothing
func (no NoOpBloomFilter) Add(_ []byte) {
}

// bloomFilter implements BloomFilter
type bloomFilter struct {
	ctx           context.Context
	mx            *sync.RWMutex
	capacity      uint32
	current       *bloom.BloomFilter
	previous      *bloom.BloomFilter
	storeInterval time.Duration
	clock         utils.Clock
	datadir       string
	counter       atomic.Uint32
	queue         chan []byte
	stopConsume   chan struct{}
}

// NewBloomFilter constructor for BloomFilter
func NewBloomFilter(ctx context.Context, clock utils.Clock, storeInterval time.Duration, datadir string, capacity uint32, queueSize uint32) (BloomFilter, error) {
	return newBloomFilter(ctx, clock, storeInterval, datadir, capacity, queueSize)
}

func newBloomFilter(ctx context.Context, clock utils.Clock, storeInterval time.Duration, datadir string, capacity uint32, queueSize uint32) (*bloomFilter, error) {
	bf := &bloomFilter{
		ctx:           ctx,
		mx:            &sync.RWMutex{},
		capacity:      capacity,
		current:       nil,
		previous:      nil,
		storeInterval: storeInterval,
		clock:         clock,
		datadir:       datadir,
		counter:       atomic.Uint32{},
		queue:         make(chan []byte, queueSize),
		stopConsume:   make(chan struct{}),
	}

	var err error
	var counter uint32
	bf.current, bf.previous, counter, err = bf.newBfPair()
	if err != nil {
		return nil, err
	}

	bf.counter.Store(counter)

	go bf.storeOnDiskWorker()
	go bf.consumeWorker()

	return bf, nil
}

// Add value to bloom filter
func (b *bloomFilter) Add(val []byte) {
	select {
	case b.queue <- val:
	default:
		log.Warn("BloomFilter queue is full during persisting cache, dropping value")
	}
}

func (b *bloomFilter) add(val []byte) {
	b.counter.Add(1)

	b.mx.Lock()
	b.current.Add(val)
	b.mx.Unlock()

	// it uses lock inside to control locking granularly
	b.maybeSwitchFilters()
}

// Check if value exists in bloom filter
func (b *bloomFilter) Check(val []byte) bool {
	// 1. check if entry is present in current bloom_filter
	b.mx.RLock()
	currTestResult := b.current.Test(val)
	b.mx.RUnlock()

	if currTestResult {
		return true
	}

	// 2. add value to current bf (even if it is present in previous)
	b.Add(val)

	// 3. check if entry is present in previous bf
	b.mx.RLock()
	prevTestResul := b.previous.Test(val)
	b.mx.RUnlock()

	return prevTestResul
}

func (b *bloomFilter) storeOnDiskWorker() {
	alert := b.clock.Ticker(b.storeInterval).Alert()
	done := b.ctx.Done()

	for {
		select {
		case <-alert:
			err := b.storeOnDisk()
			if err != nil {
				log.Errorf("BloomFilter: store on disk: %s", err)
			}

		case <-done:
			return
		}
	}
}

func (b *bloomFilter) maybeSwitchFilters() {
	if !b.counter.CompareAndSwap(b.capacity, 0) {
		return
	}

	startTime := time.Now()

	b.mx.Lock()
	b.previous = b.current
	b.current = b.newEmptyBf()
	b.mx.Unlock()

	lockDuration := time.Since(startTime)

	// do not block consumer while writting on disk
	go func() {
		// No writes to previous filter so we can omit read lock
		bytes, err := writeBloomToFile(b.previous, path.Join(b.storePath(), previousBloomFileName))
		if err != nil {
			log.Errorf("BloomFilter failed to write previous bloom filter to file - %v", err)
			return
		}

		totalDuration := time.Since(startTime)

		log.Infof("BloomFilter switched filters, %v bytes saved lock duration %v ms, total duration %v ms", bytes, lockDuration.Milliseconds(), totalDuration.Milliseconds())
	}()
}

// newBfPair tries to read current and previous bloom filter from disk,
// if there are no files it returns empty pair
func (b *bloomFilter) newBfPair() (curr, prev *bloom.BloomFilter, counter uint32, err error) {
	bloomPath := b.storePath()
	var readBytes int64

	// if bloom directory doesn't exist - create it
	if _, err := os.Stat(bloomPath); os.IsNotExist(err) {
		if e := os.Mkdir(bloomPath, os.ModePerm); e != nil {
			return nil, nil, 0, e
		}
	}

	currentBloomFilePath := path.Join(bloomPath, currentBloomFileName)
	previousBloomFilePath := path.Join(bloomPath, previousBloomFileName)
	counterBloomFilePath := path.Join(bloomPath, counterBloomFileName)

	curr, readBytes, err = readBloomFromFile(currentBloomFilePath)
	switch {
	case os.IsNotExist(err):
		curr = b.newEmptyBf()
		log.Infof("BloomFilter bloom file %s does not exist", currentBloomFilePath)
	case err != nil:
		return nil, nil, 0, err
	default:
		log.Infof("BloomFilter read bloom filter from file %s, bytes %v", currentBloomFilePath, readBytes)
	}

	prev, readBytes, err = readBloomFromFile(previousBloomFilePath)
	switch {
	case os.IsNotExist(err):
		prev = b.newEmptyBf()
		log.Infof("BloomFilter bloom file %s does not exist", previousBloomFilePath)
	case err != nil:
		return nil, nil, 0, err
	default:
		log.Infof("BloomFilter read bloom filter from file %s, bytes %v", previousBloomFilePath, readBytes)
	}

	counter, err = readCounterFromFile(counterBloomFilePath)
	switch {
	case os.IsNotExist(err):
		counter = curr.ApproximatedSize()
		log.Infof("BloomFilter counter file %s does not exist", counterBloomFilePath)
	case err != nil:
		return nil, nil, 0, err
	default:
		log.Infof("BloomFilter read counter from file %s", counterBloomFilePath)
	}

	return curr, prev, counter, nil
}

func (b *bloomFilter) storeOnDisk() error {
	startTime := time.Now()

	// Mutex works in FIFO order which means write lock in consume worker will lock read lock in Check func
	// so let's pause adding new hashes in order to not locking Check.
	// This would not work if you have multiple consume workers.
	b.stopConsume <- struct{}{}
	<-b.stopConsume

	// Lock here is not neccecary because we already stopped adding new hashes
	currentCounter := b.counter.Load()
	currentCopy := bloom.FromWithM(b.current.BitSet().Clone().Bytes(), b.current.Cap(), b.current.K())

	b.stopConsume <- struct{}{}

	// notify queue worker to start adding new values
	copyDuration := time.Since(startTime)

	bloomPath := b.storePath()
	bytes, err := writeBloomToFile(currentCopy, path.Join(bloomPath, currentBloomFileName))
	if err != nil {
		return err
	}

	if err := writeCounterToFile(currentCounter, path.Join(bloomPath, counterBloomFileName)); err != nil {
		return err
	}

	totalDuration := time.Since(startTime)

	log.Infof("BloomFilter store cache current counter %v, bytes %v, copy duration %v ms, total duration %v ms", b.counter.Load(), bytes, copyDuration.Milliseconds(), totalDuration.Milliseconds())

	return nil
}

func (b *bloomFilter) consumeWorker() {
	for {
		select {
		case val := <-b.queue:
			b.add(val)
		case <-b.stopConsume:
			b.stopConsume <- struct{}{}
			<-b.stopConsume
		case <-b.ctx.Done():
			return
		}
	}
}

func (b *bloomFilter) storePath() string {
	return path.Join(b.datadir, bloomFilterDirName)
}

func (b *bloomFilter) newEmptyBf() *bloom.BloomFilter {
	return bloom.NewWithEstimates(uint(b.capacity), bloomFalsePositiveProbability)
}

func readCounterFromFile(path string) (uint32, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return 0, err
	}

	counter, err := strconv.ParseUint(string(data), 10, 32)
	if err != nil {
		return 0, err
	}

	return uint32(counter), nil
}

func readBloomFromFile(path string) (*bloom.BloomFilter, int64, error) {
	var readBytes int64
	f, err := os.Open(path)
	if err != nil {
		return nil, readBytes, err
	}

	bf := bloom.New(0, 0)
	dataReader := bufio.NewReader(f)
	readBytes, err = bf.ReadFrom(dataReader)
	if err != nil {
		return nil, readBytes, fmt.Errorf("read bloom filter from file %s: %w", path, err)
	}

	if err := f.Close(); err != nil {
		return nil, readBytes, fmt.Errorf("close bloom filter file %s: %w", path, err)
	}

	return bf, readBytes, nil
}

func writeBloomToFile(bf *bloom.BloomFilter, path string) (int64, error) {
	var savedBytes int64
	err := os.Remove(path)
	if err != nil && !os.IsNotExist(err) {
		return savedBytes, err
	}

	f, err := os.Create(path)
	if err != nil {
		return savedBytes, fmt.Errorf("create file %s: %w", path, err)
	}

	dataWriter := bufio.NewWriter(f)
	savedBytes, err = bf.WriteTo(dataWriter)
	if err != nil {
		return savedBytes, fmt.Errorf("write bloom filter into a file %s: %w", path, err)
	}

	if err = dataWriter.Flush(); err != nil {
		return savedBytes, err
	}

	if err := f.Close(); err != nil {
		return savedBytes, fmt.Errorf("close bloom filter file %s: %w", path, err)
	}

	return savedBytes, nil
}

func writeCounterToFile(counter uint32, path string) error {
	f, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Errorf("BloomFilter failed to create counter file - %v", err)
		return nil
	}

	if _, err := f.WriteString(fmt.Sprintf("%d", counter)); err != nil {
		return err
	}

	if err := f.Close(); err != nil {
		return fmt.Errorf("close bloom filter counter file %s: %w", path, err)
	}

	return nil
}
