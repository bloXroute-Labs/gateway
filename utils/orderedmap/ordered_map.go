package orderedmap

import (
	"sync"

	orderedmap "github.com/wk8/go-ordered-map"
)

// OrderedMap is concurrent safe ordered map
// Concurrency safety might not spread on updating elements
type OrderedMap struct {
	om *orderedmap.OrderedMap

	lock *sync.RWMutex
}

// New creates a new OrderedMap.
func New() *OrderedMap {
	return &OrderedMap{
		om:   orderedmap.New(),
		lock: &sync.RWMutex{},
	}
}

// Len returns the length of the ordered map.
func (m *OrderedMap) Len() int {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.om.Len()
}

// Set sets the key-value pair, and returns what `Get` would have returned
// on that key prior to the call to `Set`.
func (m *OrderedMap) Set(key, value interface{}) (interface{}, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.om.Set(key, value)
}

// Get looks for the given key, and returns the value associated with it,
// or nil if not found. The boolean it returns says whether the key is present in the map.
func (m *OrderedMap) Get(key interface{}) (interface{}, bool) {
	m.lock.RLock()
	defer m.lock.RUnlock()

	return m.om.Get(key)
}

// Delete removes the key-value pair, and returns what `Get` would have returned
// on that key prior to the call to `Delete`.
func (m *OrderedMap) Delete(key interface{}) (interface{}, bool) {
	m.lock.Lock()
	defer m.lock.Unlock()

	return m.om.Delete(key)
}

// Oldest returns a pointer to the oldest pair. It's meant to be used to iterate on the ordered map's
// pairs from the oldest to the newest, e.g.:
// for pair := orderedMap.Oldest(); pair != nil; pair = pair.Next() { fmt.Printf("%v => %v\n", pair.Key, pair.Value) }
func (m *OrderedMap) Oldest() *Pair {
	m.lock.RLock()
	defer m.lock.RUnlock()

	oldest := m.om.Oldest()
	if oldest != nil {
		return &Pair{
			Pair: m.om.Oldest(),
			lock: m.lock,
		}
	}

	return nil
}

// Newest returns a pointer to the newest pair. It's meant to be used to iterate on the ordered map's
// pairs from the newest to the oldest, e.g.:
// for pair := orderedMap.Oldest(); pair != nil; pair = pair.Next() { fmt.Printf("%v => %v\n", pair.Key, pair.Value) }
func (m *OrderedMap) Newest() *Pair {
	m.lock.RLock()
	defer m.lock.RUnlock()

	newest := m.om.Newest()
	if newest != nil {
		return &Pair{
			Pair: m.om.Newest(),
			lock: m.lock,
		}
	}

	return nil
}

// Pair is key value pair
type Pair struct {
	*orderedmap.Pair
	lock *sync.RWMutex
}

// Next returns a pointer to the next pair.
func (p *Pair) Next() *Pair {
	p.lock.RLock()
	defer p.lock.RUnlock()

	next := p.Pair.Next()
	if next != nil {
		return &Pair{
			Pair: next,
			lock: p.lock,
		}
	}

	return nil
}

// Prev returns a pointer to the previous pair.
func (p *Pair) Prev() *Pair {
	p.lock.RLock()
	defer p.lock.RUnlock()

	prev := p.Pair.Prev()
	if prev != nil {
		return &Pair{
			Pair: prev,
			lock: p.lock,
		}
	}

	return nil
}
