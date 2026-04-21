package core

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"
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

// PruneIf atomically removes all items matching the predicate, holding the
// write lock for the entire operation to prevent races with concurrent creates.
func (s *StateStore[T]) PruneIf(pred func(key string, val T) bool) []T {
	s.mu.Lock()
	defer s.mu.Unlock()
	var pruned []T
	for k, v := range s.items {
		if pred(k, v) {
			delete(s.items, k)
			pruned = append(pruned, v)
		}
	}
	return pruned
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

// Store holds all in-memory state shared by all backends. Per
// the Images store is purely an in-process cache; no on-disk
// persistence. `docker images` / `docker inspect <image>` are served
// from the cache when populated by recent `docker pull` calls; backends
// that want a complete view derive directly from the cloud registry.
type Store struct {
	Containers     *StateStore[api.Container]
	ContainerNames *StateStore[string] // name → container ID
	Images         *StateStore[api.Image]
	Networks       *StateStore[api.Network]
	Volumes        *StateStore[api.Volume]
	Execs          *StateStore[api.ExecInstance]
	Creds          *StateStore[api.AuthRequest]
	Pods           *PodRegistry
	WaitChs        sync.Map // containerID → chan struct{}
	LogBuffers     sync.Map // containerID → []byte
	// InvocationResults records per-container FaaS invocation outcomes so
	// CloudState can report an accurate `exited` state (+ exit code +
	// stopped-at) once the invoke call returns. Populated by the
	// invocation-driving goroutine on Lambda / GCF / AZF (Phase 95).
	// Crash-scoped: a restarted backend loses these and falls back to
	// cloud state (function exists ⇒ `running` until the user removes it).
	InvocationResults sync.Map // containerID → InvocationResult
	VolumeDirs        sync.Map // volumeName → string (host temp dir path)
	StagingDirs       sync.Map // containerID → string (pre-start archive staging dir)
	PathMappings      sync.Map // containerID → map[string]string (container path → host path)
	HealthChecks      sync.Map // containerID → context.CancelFunc
	BuildContexts     sync.Map // imageID → string (temp dir with COPY files at destination paths)
	TmpfsDirs         sync.Map // containerID → []string (tmpfs temp dir paths)
	PrevCPUStats      sync.Map // containerID → *prevCPUStats
	ImageHistory      sync.Map // imageID → []ImageHistoryItem (real build history)
	LayerContent      sync.Map // layerDigest → []byte (preserved layer tarballs from docker load)
	IPAlloc           *IPAllocator
	RenameMu          sync.Mutex
	RestartHook       func(containerID string, exitCode int) bool
	pidCounter        atomic.Int64 // incrementing PID counter
}

// InvocationResult captures the outcome of a single FaaS invocation so
// Docker state can reflect the container as exited with the right exit
// code and finish time. Lambda maps `FunctionError` → 1 (otherwise 0);
// GCF/AZF map HTTP status codes (2xx ⇒ 0, 4xx/5xx ⇒ 1, 408 ⇒ 124);
// ContainerStop writes 137.
type InvocationResult struct {
	ExitCode   int
	FinishedAt time.Time
	Error      string
}

// PutInvocationResult records the outcome of a container's FaaS
// invocation. Sets FinishedAt to the current time if not provided.
func (st *Store) PutInvocationResult(id string, r InvocationResult) {
	if r.FinishedAt.IsZero() {
		r.FinishedAt = time.Now()
	}
	st.InvocationResults.Store(id, r)
}

// GetInvocationResult returns the recorded outcome for a container, or
// (zero, false) if the invocation hasn't finished (or hasn't happened).
func (st *Store) GetInvocationResult(id string) (InvocationResult, bool) {
	v, ok := st.InvocationResults.Load(id)
	if !ok {
		return InvocationResult{}, false
	}
	return v.(InvocationResult), true
}

// DeleteInvocationResult clears the recorded outcome for a container —
// used by ContainerRemove so the container really disappears.
func (st *Store) DeleteInvocationResult(id string) {
	st.InvocationResults.Delete(id)
}

// HTTPStatusToExitCode maps a FaaS HTTP-trigger response status to a
// Docker-style container exit code. 2xx ⇒ 0; 408 (request timeout) ⇒
// 124 (GNU `timeout` convention matches what Lambda uses for timeouts);
// any other 4xx/5xx ⇒ 1 (function-code crash). Used by GCF + AZF.
func HTTPStatusToExitCode(status int) int {
	switch {
	case status >= 200 && status < 300:
		return 0
	case status == 408:
		return 124
	default:
		return 1
	}
}

// HTTPInvokeErrorExitCode maps an HTTP-client error (network, TLS,
// context timeout) to an exit code. Context deadline / timeout becomes
// 124; everything else becomes 1.
func HTTPInvokeErrorExitCode(err error) int {
	if err == nil {
		return 0
	}
	es := err.Error()
	if containsAny(es, "context deadline exceeded", "Client.Timeout", "i/o timeout") {
		return 124
	}
	return 1
}

func containsAny(s string, subs ...string) bool {
	for _, sub := range subs {
		if strings.Contains(s, sub) {
			return true
		}
	}
	return false
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
		Pods:           NewPodRegistry(),
		IPAlloc:        NewIPAllocator(),
	}
}

// NextPID returns the next incrementing PID for realistic container simulation.
// Replaces hardcoded Pid=42 / Pid=43 values.
func (st *Store) NextPID() int {
	return int(st.pidCounter.Add(1))
}

// StopContainer transitions a container to the exited state and closes wait channels.
// If a RestartHook is set and returns true, the container is restarted instead of exiting.
func (st *Store) StopContainer(id string, exitCode int) {
	// Cancel health check goroutine if running
	if cancel, ok := st.HealthChecks.LoadAndDelete(id); ok {
		cancel.(context.CancelFunc)()
	}

	// Check restart policy before transitioning to exited
	if st.RestartHook != nil && st.RestartHook(id, exitCode) {
		return
	}

	st.forceStop(id, exitCode)
}

// ForceStopContainer transitions a container to exited, bypassing any restart policy.
// Used by explicit stop/kill handlers where the user intends to stop the container.
func (st *Store) ForceStopContainer(id string, exitCode int) {
	if cancel, ok := st.HealthChecks.LoadAndDelete(id); ok {
		cancel.(context.CancelFunc)()
	}
	st.forceStop(id, exitCode)
}

// RevertToCreated reverts a container from "running" back to "created" state.
// Used when a cloud operation fails after the container was optimistically set to running.
// The wait channel is closed so waiters are unblocked and can observe the reverted state.
func (st *Store) RevertToCreated(id string) {
	st.Containers.Update(id, func(c *api.Container) {
		c.State.Status = "created"
		c.State.Running = false
		c.State.Pid = 0
		c.State.StartedAt = "0001-01-01T00:00:00Z"
	})
	if ch, ok := st.WaitChs.LoadAndDelete(id); ok {
		close(ch.(chan struct{}))
	}
}

func (st *Store) forceStop(id string, exitCode int) {
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
