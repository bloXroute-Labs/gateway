package syncmap

import (
	"github.com/puzpuzpuz/xsync/v2"
)

// SyncMap is concurrent safe map with generics / arbitrarily typed keys
// Documentation: https://pkg.go.dev/github.com/puzpuzpuz/xsync
type SyncMap[K comparable, V any] struct {
	m *xsync.MapOf[K, V]
}

// NewIntegerMapOf new map of integer keys
func NewIntegerMapOf[K xsync.IntegerConstraint, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m: xsync.NewIntegerMapOf[K, V](),
	}
}

// NewStringMapOf new map of string keys
func NewStringMapOf[V any]() *SyncMap[string, V] {
	return &SyncMap[string, V]{
		m: xsync.NewMapOf[V](),
	}
}

// NewTypedMapOf new map with arbitrary keys
func NewTypedMapOf[K comparable, V any](hasher Hasher[K]) *SyncMap[K, V] {
	return &SyncMap[K, V]{
		m: xsync.NewTypedMapOf[K, V](hasher),
	}
}

// Load loads value by key
func (m *SyncMap[K, V]) Load(key K) (val V, exists bool) {
	return m.m.Load(key)
}

// Store stores key and value in a  map
func (m *SyncMap[K, V]) Store(key K, val V) {
	m.m.Store(key, val)
}

// LoadOrStore load or store key values
func (m *SyncMap[K, V]) LoadOrStore(key K, val V) (actual V, loaded bool) {
	return m.m.LoadOrStore(key, val)
}

// LoadAndStore load and store key values
func (m *SyncMap[K, V]) LoadAndStore(key K, val V) (actual V, loaded bool) {
	return m.m.LoadAndStore(key, val)
}

// LoadAndDelete load and delete object
func (m *SyncMap[K, V]) LoadAndDelete(key K) (val V, loaded bool) {
	return m.m.LoadAndDelete(key)
}

// Clear empty everything
func (m *SyncMap[K, V]) Clear() {
	m.m.Clear()
}

// Delete delete specific object/value by key
func (m *SyncMap[K, V]) Delete(key K) {
	m.m.Delete(key)
}

// Size returns the size of a map
func (m *SyncMap[K, V]) Size() int {
	return m.m.Size()
}

// Range calls f sequentially for each key and value present in the map. If f returns false, range stops the iteration.
func (m *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	m.m.Range(f)
}

// Keys returns slice of all keys in map.
func (m *SyncMap[K, V]) Keys() (keys []K) {
	m.Range(func(key K, value V) bool {
		keys = append(keys, key)
		return true
	})

	return keys
}

// Compute either sets the computed new value for the key or deletes the value for the key.
func (m *SyncMap[K, V]) Compute(key K, valueFn func(oldValue V, loaded bool) (newValue V, delete bool)) (actual V, loaded bool) {
	return m.m.Compute(key, valueFn)
}

// Has checks whether object exists or not
func (m *SyncMap[K, V]) Has(key K) (exists bool) {
	_, exists = m.m.Load(key)
	return
}
