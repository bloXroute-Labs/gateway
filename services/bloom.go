package services

import (
	"bufio"
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"path"
	"strconv"
	"sync"
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
	counter       uint32
}

// NewBloomFilter constructor for BloomFilter
func NewBloomFilter(ctx context.Context, clock utils.Clock, storeInterval time.Duration, datadir string, capacity uint32) (BloomFilter, error) {
	var err error

	bf := &bloomFilter{
		ctx:           ctx,
		mx:            &sync.RWMutex{},
		capacity:      capacity,
		current:       nil,
		previous:      nil,
		storeInterval: storeInterval,
		clock:         clock,
		datadir:       datadir,
		counter:       0,
	}

	bf.current, bf.previous, bf.counter, err = bf.newBfPair()
	if err != nil {
		return nil, err
	}

	go bf.storeOnDiskWorker()
	return bf, nil
}

// Add value to bloom filter
func (b *bloomFilter) Add(val []byte) {
	b.mx.Lock()
	b.current.Add(val)
	b.counter++
	b.mx.Unlock()

	// perform switch if needed
	if b.counter >= b.capacity {
		b.maybeSwitchFilters()
	}
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

	// 4. perform switch if needed
	if b.counter >= b.capacity {
		b.maybeSwitchFilters()
	}

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
	// callers should check if size is bigger than capacity,
	// but we need to double-check size while under the write-lock
	// to avoid multiple goroutines performing filters switch
	if b.counter < b.capacity {
		return
	}

	startTime := time.Now()
	b.mx.Lock()
	previousCopyBits := make([]uint64, len(b.current.BitSet().Bytes()))
	copy(b.current.BitSet().Bytes(), previousCopyBits)
	previousCopyM := b.current.Cap()
	previousCopyK := b.current.K()
	b.previous = b.current
	b.current = b.newEmptyBf()
	b.counter = 0
	b.mx.Unlock()
	lockDuration := time.Now().Sub(startTime).Milliseconds()

	previousCopy := bloom.FromWithM(previousCopyBits, previousCopyM, previousCopyK)
	bloomPath := b.storePath()
	bytes, _ := writeBloomToFile(previousCopy, path.Join(bloomPath, previousBloomFileName))
	totalDuration := time.Now().Sub(startTime).Milliseconds()

	log.Debugf("BloomFilter switched filters, %v bytes saved lock duration %v ms, total duration %v ms", bytes, lockDuration, totalDuration)

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
	b.mx.RLock()
	currentCopyBits := make([]uint64, len(b.current.BitSet().Bytes()))
	copy(b.current.BitSet().Bytes(), currentCopyBits)
	currentCopyM := b.current.Cap()
	currentCopyK := b.current.K()
	currentCounter := b.counter
	b.mx.RUnlock()
	lockDuration := time.Now().Sub(startTime).Milliseconds()

	currentCopy := bloom.FromWithM(currentCopyBits, currentCopyM, currentCopyK)
	bloomPath := b.storePath()

	bytes, err := writeBloomToFile(currentCopy, path.Join(bloomPath, currentBloomFileName))
	if err != nil {
		return err
	}

	err = writeCounterToFile(currentCounter, path.Join(bloomPath, counterBloomFileName))
	if err != nil {
		return err
	}
	totalDuration := time.Now().Sub(startTime).Milliseconds()

	log.Infof("BloomFilter store cache current counter %v, bytes %v, lock duration %v ms, total duration %v ms", b.counter, bytes, lockDuration, totalDuration)

	return nil
}

func (b *bloomFilter) storePath() string {
	return path.Join(b.datadir, bloomFilterDirName)
}

func (b *bloomFilter) newEmptyBf() *bloom.BloomFilter {
	return bloom.NewWithEstimates(uint(b.capacity), bloomFalsePositiveProbability)
}

func readCounterFromFile(path string) (uint32, error) {
	c, err := ioutil.ReadFile(path)
	if err != nil {
		return 0, err
	}
	line := string(c)
	counter, err := strconv.ParseInt(line, 10, 32)
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

	defer func() { _ = f.Close() }()

	bf := bloom.New(0, 0)

	readBytes, err = bf.ReadFrom(f)
	if err != nil {
		return nil, readBytes, fmt.Errorf("read bloom filter from file %s: %w", path, err)
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

	savedBytes, err = bf.WriteTo(f)
	if err != nil {
		return savedBytes, fmt.Errorf("write bloom filter into a file %s: %w", path, err)
	}

	err = f.Close()
	if err != nil {
		return savedBytes, fmt.Errorf("close bloom filter file %s: %w", path, err)
	}

	return savedBytes, nil
}

func writeCounterToFile(counter uint32, path string) error {
	file, err := os.OpenFile(path, os.O_CREATE|os.O_WRONLY|os.O_TRUNC, 0644)
	if err != nil {
		log.Errorf("BloomFilter failed to create counter file - %v", err)
		return nil
	}
	dataWriter := bufio.NewWriter(file)
	if _, err = dataWriter.WriteString(fmt.Sprint(counter)); err != nil {
		return err
	}
	if err = dataWriter.Flush(); err != nil {
		return err
	}
	if err = file.Close(); err != nil {
		return err
	}
	return nil
}
