package utils

import (
	"errors"
	"testing"
	"time"

	"github.com/bloXroute-Labs/gateway/v2/utils/syncmap"
	"github.com/stretchr/testify/require"
)

func TestCache_Get(t *testing.T) {
	clock := &MockClock{}
	successCalls := 0

	// Create a new cache
	cache := newCache[string, int](syncmap.StringHasher, func(key string) (*int, error) {
		switch key {
		case "key1":
			successCalls++
			if successCalls > 2 {
				require.FailNow(t, "unexpected call")
			}

			value := 1
			return &value, nil
		case "key2":
			return nil, errors.New("some error")
		default:
			require.FailNowf(t, "unexpected key", "key: %s", key)
		}

		return nil, nil
	}, time.Minute, 5*time.Minute, clock)

	// Test getting a value that is not in the cache
	item, err := cache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, 1, *item)

	// Test getting a value that is in cache
	item, err = cache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, 1, *item)

	// Test getting a value that is in the cache
	item, err = cache.Get("key2")
	require.Error(t, err)
	require.Nil(t, item)

	clock.IncTime(2 * time.Minute)

	// Test getting a value that is in the cache but expired
	item, err = cache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, 1, *item)

	// Test getting a value that is in cache
	item, err = cache.Get("key1")
	require.NoError(t, err)
	require.Equal(t, 1, *item)

	// Get value only when value expired
	require.Equal(t, 2, successCalls)
}

func TestCache_Clean(t *testing.T) {
	clock := &MockClock{}
	cache := newCache[string, int](syncmap.StringHasher, func(key string) (*int, error) {
		return nil, nil
	}, time.Minute, 5*time.Minute, clock)

	val1 := 1
	val2 := 2
	val3 := 3

	// Add some items to the cache
	cache.cacheMap.Store("key1", value[*int]{item: &val1, exp: clock.Now().Add(time.Minute)})
	cache.cacheMap.Store("key2", value[*int]{item: &val2, exp: clock.Now().Add(2 * time.Minute)})
	cache.cacheMap.Store("key3", value[*int]{item: &val3, exp: clock.Now().Add(3 * time.Minute)})

	clock.IncTime(2 * time.Minute)

	// Call the clean method
	cache.clean()

	// Check if expired items are removed from the cache
	_, ok := cache.cacheMap.Load("key1")
	require.False(t, ok)

	_, ok = cache.cacheMap.Load("key2")
	require.False(t, ok)

	_, ok = cache.cacheMap.Load("key3")
	require.True(t, ok)
}
