package orderedmap

import (
	"sync"

	orderedmap "github.com/wk8/go-ordered-map/v2"
)

// OrderedMap is concurrent safe ordered map
// Concurrency safety might not spread on updating elements
type OrderedMap[K comparable, V any] struct {
	om *orderedmap.OrderedMap[K, V]

	lock *sync.RWMutex
}

// New creates a new OrderedMap.
func New[K comparable, V any]() *OrderedMap[K, V] {
	return &OrderedMap[K, V]{
		om:   orderedmap.New[K, V](),
		lock: &sync.RWMutex{},
	}
}

// Len returns the length of the ordered map.
func (m *OrderedMap[K, V]) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.om.Len()
}

// Set sets the key-value pair, and returns what `Get` would have returned
// on that key prior to the call to `Set`.
func (m *OrderedMap[K, V]) Set(key K, value V) (V, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.om.Set(key, value)
}

// Store is an alias for Set.
func (m *OrderedMap[K, V]) Store(key K, value V) {
	m.lock.Lock()
	defer m.lock.Unlock()

	m.om.Store(key, value)
}

// MoveAfter moves the value associated with key to its new position after the one associated with markKey.
func (m *OrderedMap[K, V]) MoveAfter(key, mark K) error {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.om.MoveAfter(key, mark)
}

// Get looks for the given key, and returns the value associated with it,
// or nil if not found. The boolean it returns says whether the key is present in the map.
func (m *OrderedMap[K, V]) Get(key K) (V, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.om.Get(key)
}

// Delete removes the key-value pair, and returns what `Get` would have returned
// on that key prior to the call to `Delete`.
func (m *OrderedMap[K, V]) Delete(key K) (V, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.om.Delete(key)
}

// Oldest returns a pointer to the oldest pair. It's meant to be used to iterate on the ordered map's
// pairs from the oldest to the newest, e.g.:
// for pair := orderedMap.Oldest(); pair != nil; pair = pair.Next() { fmt.Printf("%v => %v\n", pair.Key, pair.Value) }
func (m *OrderedMap[K, V]) Oldest() *Pair[K, V] {
	m.lock.RLock()
	defer m.lock.RUnlock()

	oldest := m.om.Oldest()
	if oldest != nil {
		return &Pair[K, V]{
			Pair: m.om.Oldest(),
			lock: m.lock,
		}
	}

	return nil
}

// Newest returns a pointer to the newest pair. It's meant to be used to iterate on the ordered map's
// pairs from the newest to the oldest, e.g.:
// for pair := orderedMap.Oldest(); pair != nil; pair = pair.Next() { fmt.Printf("%v => %v\n", pair.Key, pair.Value) }
func (m *OrderedMap[K, V]) Newest() *Pair[K, V] {
	m.lock.RLock()
	defer m.lock.RUnlock()

	newest := m.om.Newest()
	if newest != nil {
		return &Pair[K, V]{
			Pair: m.om.Newest(),
			lock: m.lock,
		}
	}

	return nil
}

// Pair is key value pair
type Pair[K comparable, V any] struct {
	*orderedmap.Pair[K, V]
	lock *sync.RWMutex
}

// Next returns a pointer to the next pair.
func (p *Pair[K, V]) Next() *Pair[K, V] {
	p.lock.RLock()
	defer p.lock.RUnlock()

	next := p.Pair.Next()
	if next != nil {
		return &Pair[K, V]{
			Pair: next,
			lock: p.lock,
		}
	}

	return nil
}

// Prev returns a pointer to the previous pair.
func (p *Pair[K, V]) Prev() *Pair[K, V] {
	p.lock.RLock()
	defer p.lock.RUnlock()

	prev := p.Pair.Prev()
	if prev != nil {
		return &Pair[K, V]{
			Pair: prev,
			lock: p.lock,
		}
	}

	return nil
}
