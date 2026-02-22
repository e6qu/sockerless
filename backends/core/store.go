package core

import (
	"context"
	"sync"
	"time"

	"github.com/sockerless/api"
)

// StateStore is a generic, thread-safe in-memory store for resources.
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

// Get retrieves a resource by ID. Returns the resource and true if found.
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

// List returns all stored resources.
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
	return ok
}

// Keys returns all stored keys.
func (s *StateStore[T]) Keys() []string {
	s.mu.RLock()
	defer s.mu.RUnlock()
	keys := make([]string, 0, len(s.items))
	for k := range s.items {
		keys = append(keys, k)
	}
	return keys
}

// Store holds all in-memory state shared by all backends.
type Store struct {
	Containers     *StateStore[api.Container]
	ContainerNames *StateStore[string] // name → container ID
	Images         *StateStore[api.Image]
	Networks       *StateStore[api.Network]
	Volumes        *StateStore[api.Volume]
	Execs          *StateStore[api.ExecInstance]
	Creds          *StateStore[api.AuthRequest]
	WaitChs        sync.Map // containerID → chan struct{}
	LogBuffers     sync.Map // containerID → []byte
	Processes      sync.Map // containerID → ContainerProcess
	VolumeDirs     sync.Map // volumeName → string (host temp dir path)
	StagingDirs    sync.Map // containerID → string (pre-start archive staging dir)
	HealthChecks   sync.Map // containerID → context.CancelFunc
	BuildContexts  sync.Map // imageID → string (temp dir with COPY files at destination paths)
}

// NewStore creates a new store with all sub-stores initialized.
func NewStore() *Store {
	return &Store{
		Containers:     NewStateStore[api.Container](),
		ContainerNames: NewStateStore[string](),
		Images:         NewStateStore[api.Image](),
		Networks:       NewStateStore[api.Network](),
		Volumes:        NewStateStore[api.Volume](),
		Execs:          NewStateStore[api.ExecInstance](),
		Creds:          NewStateStore[api.AuthRequest](),
	}
}

// StopContainer transitions a container to the exited state and closes wait channels.
func (st *Store) StopContainer(id string, exitCode int) {
	// Cancel health check goroutine if running
	if cancel, ok := st.HealthChecks.LoadAndDelete(id); ok {
		cancel.(context.CancelFunc)()
	}

	st.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "exited"
		c.State.Running = false
		c.State.Pid = 0
		c.State.ExitCode = exitCode
		c.State.FinishedAt = time.Now().UTC().Format(time.RFC3339Nano)
	})

	if ch, ok := st.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
}
