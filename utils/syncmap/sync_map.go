package syncmap

import "sync"

// SyncMap is concurrent safe map with generics
type SyncMap[K comparable, V any] struct {
	mx *sync.RWMutex

	m map[K]V
}

// New creates a new SyncMap.
func New[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		mx: new(sync.RWMutex),
		m:  make(map[K]V),
	}
}

// Get looks for the given key, and returns the value associated with it.
// The boolean it returns says whether the key is present in the map.
func (m *SyncMap[K, V]) Get(key K) (V, bool) {
	m.mx.RLock()
	val, exists := m.m[key]
	m.mx.RUnlock()

	return val, exists
}

// Set sets the key-value pair.
func (m *SyncMap[K, V]) Set(key K, val V) {
	m.mx.Lock()
	m.m[key] = val
	m.mx.Unlock()
}

// Del removes the key-value pair.
func (m *SyncMap[K, V]) Del(key K) {
	m.mx.Lock()
	delete(m.m, key)
	m.mx.Unlock()
}

// Len returns the length of the map.
func (m *SyncMap[K, V]) Len() int {
	m.mx.RLock()
	l := len(m.m)
	m.mx.RUnlock()

	return l
}

// Keys returns slice of all keys in map.
func (m *SyncMap[K, V]) Keys() []K {
	m.mx.RLock()

	keys := make([]K, 0, len(m.m))
	for key := range m.m {
		keys = append(keys, key)
	}

	m.mx.RUnlock()

	return keys
}
