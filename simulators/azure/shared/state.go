package simulator

import "sync"

// StateStore is a generic, thread-safe in-memory store for simulated cloud resources.
// Each resource type should use its own StateStore instance.
type StateStore[T any] struct {
	mu    sync.RWMutex
	items map[string]T
}

// NewStateStore creates a new empty StateStore.
func NewStateStore[T any]() *StateStore[T] {
	return &StateStore[T]{
		items: make(map[string]T),
	}
}

// Get retrieves a resource by ID. Returns the resource and true if found,
// or the zero value and false if not.
func (s *StateStore[T]) Get(id string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[id]
	return v, ok
}

// Put stores a resource by ID, overwriting any existing value.
func (s *StateStore[T]) Put(id string, item T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = item
}

// Delete removes a resource by ID. Returns true if the resource existed.
func (s *StateStore[T]) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[id]
	if ok {
		delete(s.items, id)
	}
	return ok
}

// List returns all stored resources in no particular order.
func (s *StateStore[T]) List() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]T, 0, len(s.items))
	for _, v := range s.items {
		result = append(result, v)
	}
	return result
}

// Filter returns all resources matching the predicate.
func (s *StateStore[T]) Filter(fn func(T) bool) []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	var result []T
	for _, v := range s.items {
		if fn(v) {
			result = append(result, v)
		}
	}
	return result
}

// Len returns the number of stored resources.
func (s *StateStore[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

// Update atomically reads, modifies, and writes back a resource.
// Returns false if the resource was not found.
func (s *StateStore[T]) Update(id string, fn func(*T)) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	v, ok := s.items[id]
	if !ok {
		return false
	}
	fn(&v)
	s.items[id] = v
	return true
}
