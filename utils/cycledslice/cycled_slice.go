package cycledslice

import (
	"sync"
)

// CycledSlice is a thread-safe data structure representing a cycled slice.
type CycledSlice[T any] struct {
	mu    sync.Mutex
	data  []T
	index int
}

// NewCycledSlice creates a new cycled slice from a slice of any type.
func NewCycledSlice[T any](slice []T) *CycledSlice[T] {
	return &CycledSlice[T]{data: slice}
}

// Next returns the next element in the cycled slice and advances the index in a thread-safe way.
func (cs *CycledSlice[T]) Next() T {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if len(cs.data) == 0 {
		var zeroValue T
		return zeroValue // Handle empty slice or other appropriate behavior
	}
	element := cs.data[cs.index]
	cs.index = (cs.index + 1) % len(cs.data)
	return element
}

// Current returns the current element in the cycled slice without advancing the index in a thread-safe way.
func (cs *CycledSlice[T]) Current() T {
	cs.mu.Lock()
	defer cs.mu.Unlock()

	if len(cs.data) == 0 {
		var zeroValue T
		return zeroValue // Handle empty slice or other appropriate behavior
	}
	return cs.data[cs.index]
}
