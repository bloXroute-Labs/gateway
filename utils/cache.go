package utils

import (
	"bufio"
	"io"
	"os"
	"path"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

const defaultBypass = time.Second * 10

// UpdateCacheFile - update a cache file
func UpdateCacheFile(dataDir string, fileName string, value []byte) error {
	cacheFileName := path.Join(dataDir, fileName)
	f, err := os.OpenFile(cacheFileName, os.O_CREATE|os.O_WRONLY, 0644)
	if err != nil {
		return err
	}
	defer f.Close()
	writer := bufio.NewWriter(f)
	_, err = writer.Write(value)
	if err != nil {
		return err
	}

	return writer.Flush()
}

// LoadCacheFile - load a cache file
func LoadCacheFile(dataDir string, fileName string) ([]byte, error) {
	cacheFileName := path.Join(dataDir, fileName)
	f, err := os.Open(cacheFileName)
	if err != nil {
		return nil, err
	}
	defer f.Close()

	return io.ReadAll(bufio.NewReader(f))
}

type value[V any] struct {
	item V
	exp  time.Time
}

// Cache is a thread-safe cache that will call the provided function to get the value if it is not in the cache
type Cache[K comparable, V any] struct {
	cacheMap    syncmap.SyncMap[K, value[*V]]
	lockMap     syncmap.SyncMap[K, chan struct{}]
	funcToCache func(K) (*V, error)
	expDur      time.Duration
	cleanDur    time.Duration
	clock       Clock
}

// NewCache creates a new cache with the provided function to get the value if it is not in the cache
func NewCache[K comparable, V any](hasher syncmap.Hasher[K], valueFetcher func(K) (*V, error), expDur time.Duration, cleanDur time.Duration) *Cache[K, V] {
	return newCache[K, V](hasher, valueFetcher, expDur, cleanDur, RealClock{})
}

func newCache[K comparable, V any](hasher syncmap.Hasher[K], valueFetcher func(K) (*V, error), dur time.Duration, cleanDur time.Duration, clock Clock) *Cache[K, V] {
	c := &Cache[K, V]{
		cacheMap:    *syncmap.NewTypedMapOf[K, value[*V]](hasher),
		lockMap:     *syncmap.NewTypedMapOf[K, chan struct{}](hasher),
		funcToCache: valueFetcher,
		expDur:      dur,
		cleanDur:    cleanDur,
		clock:       clock,
	}

	go c.cleanRoutine()

	return c
}

// Get returns the value for the provided key. If the value is not in the cache, the provided function will be called to get the value
func (c *Cache[K, V]) Get(key K) (item *V, err error) {
	lock, loaded := c.lockMap.LoadOrStore(key, make(chan struct{}))
	if loaded {
		// lock exists, wait for it to be release or bypass (lock is always released when the value is cached)
		select {
		case <-lock:
		case <-c.clock.Timer(defaultBypass).Alert():
		}

		val, exists := c.cacheMap.Load(key)
		if exists && c.clock.Now().Before(val.exp) {
			return val.item, nil
		}
	} else {
		// release the lock after the value is cached
		defer close(lock)
	}

	// fetch the value in case of the initial lock or the bypass
	// the initial lock will be released once the getAndStore function is done
	item, err = c.getAndStore(key)
	if err != nil {
		// delete the lock if the function fails
		c.lockMap.Delete(key)
	}

	return
}

func (c *Cache[K, V]) getAndStore(key K) (*V, error) {
	item, err := c.funcToCache(key)
	if err != nil {
		return nil, err
	}

	c.cacheMap.Store(key, value[*V]{
		item: item,
		exp:  c.clock.Now().Add(c.expDur),
	})

	return item, nil
}

func (c *Cache[K, V]) cleanRoutine() {
	ticker := c.clock.Timer(c.cleanDur)

	for range ticker.Alert() {
		c.clean()
	}
}

func (c *Cache[K, V]) clean() {
	now := c.clock.Now()
	c.cacheMap.Range(func(key K, value value[*V]) bool {
		if !now.Before(value.exp) {
			c.cacheMap.Delete(key)
			c.lockMap.Delete(key)
		}

		return true
	})
}
