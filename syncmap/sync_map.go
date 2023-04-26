package syncmap

import (
	"fmt"
	"sync"
)

type SyncMap[K comparable, V any] struct {
	wrapped map[K]V
	mux     sync.RWMutex
}

func New[K comparable, V any]() *SyncMap[K, V] {
	return &SyncMap[K, V]{
		wrapped: make(map[K]V),
	}
}

// Load returns the value stored in the map for a key, or nil if no
// value is present.
// The ok result indicates whether value was found in the map.
func (m *SyncMap[K, V]) Load(key K) (value V, ok bool) {
	m.mux.RLock()
	defer m.mux.RUnlock()
	value, ok = m.wrapped[key]
	return value, ok
}

// Store sets the value for a key.
func (m *SyncMap[K, V]) Store(key K, value V) {
	m.Swap(key, value)
}

// Swap swaps the value for a key and returns the previous value if any.
// The loaded result reports whether the key was present.
func (m *SyncMap[K, V]) Swap(key K, value V) (previous V, loaded bool) {
	m.mux.Lock()
	defer m.mux.Unlock()
	previous, loaded = m.wrapped[key]
	m.wrapped[key] = value
	return previous, loaded
}

// Delete deletes the value for a key.
func (m *SyncMap[K, V]) Delete(key K) {
	m.LoadAndDelete(key)
}

// LoadAndDelete deletes the value for a key, returning the previous value if any.
// The loaded result reports whether the key was present.
func (m *SyncMap[K, V]) LoadAndDelete(key K) (value V, loaded bool) {
	if value, loaded = m.Load(key); !loaded {
		return value, loaded
	}
	m.mux.Lock()
	defer m.mux.Unlock()
	value, loaded = m.wrapped[key]
	delete(m.wrapped, key)
	return value, loaded
}

// LoadOrStore returns the existing value for the key if present.
// Otherwise, it stores and returns the given value.
// The loaded result is true if the value was loaded, false if stored.
func (m *SyncMap[K, V]) LoadOrStore(key K, value V) (actual V, loaded bool) {
	if actual, loaded = m.Load(key); loaded {
		return actual, loaded
	}
	m.mux.Lock()
	defer m.mux.Unlock()
	if actual, loaded = m.wrapped[key]; loaded {
		return actual, loaded
	}
	m.wrapped[key] = value
	return value, false
}

// CompareAndSwap swaps the old and new values for key
// if the value stored in the map is equal to old.
// The old value must be of a comparable type.
func (m *SyncMap[K, V]) CompareAndSwap(key K, old V, new V) bool {
	if value, _ := m.Load(key); any(value) != any(old) {
		return false
	}
	m.mux.Lock()
	defer m.mux.Unlock()
	if value, _ := m.wrapped[key]; any(value) != any(old) {
		return false
	}
	m.wrapped[key] = new
	return true
}

// CompareAndDelete deletes the entry for key if its value is equal to old.
// The old value must be of a comparable type.
//
// If there is no current value for key in the map, CompareAndDelete
// returns false (even if the old value is the nil interface value).
func (m *SyncMap[K, V]) CompareAndDelete(key K, old V) (deleted bool) {
	if value, loaded := m.Load(key); !loaded || any(value) != any(old) {
		return false
	}
	m.mux.Lock()
	defer m.mux.Unlock()
	if value, loaded := m.wrapped[key]; !loaded || any(value) != any(old) {
		return false
	}
	delete(m.wrapped, key)
	return true
}

// Range calls f sequentially for each key and value present in the map.
// If f returns false, range stops the iteration.
//
// Range does not necessarily correspond to any consistent snapshot of the Map's
// contents: no key will be visited more than once, but if the value for any key
// is stored or deleted concurrently (including by f), Range may reflect any
// mapping for that key from any point during the Range call. Range does not
// block other methods on the receiver; even f itself may call any method on m.
//
// Range may be O(N) with the number of elements in the map even if f returns
// false after a constant number of calls.
func (m *SyncMap[K, V]) Range(f func(key K, value V) bool) {
	for _, k := range m.Keys() {
		v, ok := m.Load(k)
		if !ok {
			continue
		}
		if !f(k, v) {
			break
		}
	}
}

// Keys returns a slice containing the SyncMap's keys.
func (m *SyncMap[K, V]) Keys() []K {
	m.mux.RLock()
	defer m.mux.RUnlock()
	keys := make([]K, 0, len(m.wrapped))
	for key, _ := range m.wrapped {
		keys = append(keys, key)
	}
	return keys
}

// Clear reassigns the underlying map to a newly allocated empty map.
func (m *SyncMap[K, V]) Clear() {
	m.mux.Lock()
	defer m.mux.Unlock()
	m.wrapped = make(map[K]V)
}

// Call blocks all other methods on the receiver and calls f on the map.
func (m *SyncMap[K, V]) Call(f func(map[K]V)) {
	m.mux.Lock()
	defer m.mux.Unlock()
	f(m.wrapped)
}

func (m *SyncMap[K, V]) String() string {
	m.mux.RLock()
	defer m.mux.RUnlock()
	return fmt.Sprintf("%T{wrapped:%#v}", m, m.wrapped)
}
