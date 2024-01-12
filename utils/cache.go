package utils

import (
	"bufio"
	"io"
	"os"
	"path"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
)

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
	funcToCache func(K) (*V, error)
	expDur      time.Duration
	cleanDur    time.Duration
	clock       Clock
}

// NewCache creates a new cache with the provided function to get the value if it is not in the cache
func NewCache[K comparable, V any](hasher syncmap.Hasher[K], f func(K) (*V, error), expDur time.Duration, cleanDur time.Duration) *Cache[K, V] {
	return newCache[K, V](hasher, f, expDur, cleanDur, RealClock{})
}

func newCache[K comparable, V any](hasher syncmap.Hasher[K], f func(K) (*V, error), dur time.Duration, cleanDur time.Duration, clock Clock) *Cache[K, V] {
	c := &Cache[K, V]{
		cacheMap:    *syncmap.NewTypedMapOf[K, value[*V]](hasher),
		funcToCache: f,
		expDur:      dur,
		cleanDur:    cleanDur,
		clock:       clock,
	}

	go c.cleanRoutine()

	return c
}

// Get returns the value for the provided key. If the value is not in the cache, the provided function will be called to get the value
func (c *Cache[K, V]) Get(key K) (item *V, err error) {
	val, exists := c.cacheMap.Load(key)
	if exists && c.clock.Now().Before(val.exp) {
		return val.item, nil
	}

	some, err := c.funcToCache(key)
	if err != nil {
		return nil, err
	}

	c.cacheMap.Store(key, value[*V]{
		item: some,
		exp:  c.clock.Now().Add(c.expDur),
	})

	return some, nil
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
		}

		return true
	})
}
