package simulator

import "sync"

// Store is the interface for a typed key-value store.
// Implemented by MemoryStore (in-memory) and SQLiteStore (persistent).
type Store[T any] interface {
	Get(id string) (T, bool)
	Put(id string, item T)
	Delete(id string) bool
	List() []T
	Filter(fn func(T) bool) []T
	Len() int
	Update(id string, fn func(*T)) bool
}

// StateStore is an alias for backward compatibility.
// New code should use Store[T] interface or MemoryStore[T] directly.
type StateStore[T any] = MemoryStore[T]

// MemoryStore is an in-memory implementation of Store backed by a map.
type MemoryStore[T any] struct {
	mu    sync.RWMutex
	items map[string]T
}

// NewStateStore creates a new in-memory store. Returns Store[T] for interface compatibility.
func NewStateStore[T any]() *MemoryStore[T] {
	return &MemoryStore[T]{
		items: make(map[string]T),
	}
}

func (s *MemoryStore[T]) Get(id string) (T, bool) {
	s.mu.RLock()
	defer s.mu.RUnlock()
	v, ok := s.items[id]
	return v, ok
}

func (s *MemoryStore[T]) Put(id string, item T) {
	s.mu.Lock()
	defer s.mu.Unlock()
	s.items[id] = item
}

func (s *MemoryStore[T]) Delete(id string) bool {
	s.mu.Lock()
	defer s.mu.Unlock()
	_, ok := s.items[id]
	if ok {
		delete(s.items, id)
	}
	return ok
}

func (s *MemoryStore[T]) List() []T {
	s.mu.RLock()
	defer s.mu.RUnlock()
	result := make([]T, 0, len(s.items))
	for _, v := range s.items {
		result = append(result, v)
	}
	return result
}

func (s *MemoryStore[T]) Filter(fn func(T) bool) []T {
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

func (s *MemoryStore[T]) Len() int {
	s.mu.RLock()
	defer s.mu.RUnlock()
	return len(s.items)
}

func (s *MemoryStore[T]) Update(id string, fn func(*T)) bool {
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
