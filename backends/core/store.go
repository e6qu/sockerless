package core

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
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

// Store holds all in-memory state shared by all backends.
type Store struct {
	// ImageStatePath, if non-empty, is the on-disk JSON file where
	// `Images` is persisted after every mutation via PersistImages
	// and restored on startup via RestoreImages. Populated from
	// $SOCKERLESS_STATE_DIR by NewBaseServer. Enables `docker pull`
	// state to survive backend restart (fixes BUG-697).
	ImageStatePath string

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
	VolumeDirs     sync.Map // volumeName → string (host temp dir path)
	StagingDirs    sync.Map // containerID → string (pre-start archive staging dir)
	PathMappings   sync.Map // containerID → map[string]string (container path → host path)
	HealthChecks   sync.Map // containerID → context.CancelFunc
	BuildContexts  sync.Map // imageID → string (temp dir with COPY files at destination paths)
	TmpfsDirs      sync.Map // containerID → []string (tmpfs temp dir paths)
	PrevCPUStats   sync.Map // containerID → *prevCPUStats
	ImageHistory   sync.Map // imageID → []ImageHistoryItem (real build history)
	LayerContent   sync.Map // layerDigest → []byte (preserved layer tarballs from docker load)
	IPAlloc        *IPAllocator
	RenameMu       sync.Mutex
	RestartHook    func(containerID string, exitCode int) bool
	pidCounter     atomic.Int64 // incrementing PID counter
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

// imagesSnapshot is the on-disk shape for Store.Images persistence.
// Keyed by every alias StoreImageWithAliases emits — the restore
// repopulates the exact map.
type imagesSnapshot struct {
	Version int                  `json:"version"`
	Items   map[string]api.Image `json:"items"`
}

const imagesSnapshotVersion = 1

// PersistImages serializes `Images` to the given JSON file via atomic
// temp-file + rename. Safe to call on every image mutation. Nil path
// is a no-op. Returns any error so the caller can log — failures
// should not abort the operation that triggered them.
func (s *Store) PersistImages(path string) error {
	if path == "" {
		return nil
	}
	s.Images.mu.RLock()
	snap := imagesSnapshot{
		Version: imagesSnapshotVersion,
		Items:   make(map[string]api.Image, len(s.Images.items)),
	}
	for k, v := range s.Images.items {
		snap.Items[k] = v
	}
	s.Images.mu.RUnlock()

	data, err := json.Marshal(&snap)
	if err != nil {
		return fmt.Errorf("marshal images snapshot: %w", err)
	}

	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return fmt.Errorf("mkdir state dir: %w", err)
	}
	tmp, err := os.CreateTemp(filepath.Dir(path), ".images-*.json.tmp")
	if err != nil {
		return fmt.Errorf("create temp: %w", err)
	}
	tmpName := tmp.Name()
	if _, err := tmp.Write(data); err != nil {
		tmp.Close()
		os.Remove(tmpName)
		return fmt.Errorf("write temp: %w", err)
	}
	if err := tmp.Close(); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("close temp: %w", err)
	}
	if err := os.Rename(tmpName, path); err != nil {
		os.Remove(tmpName)
		return fmt.Errorf("rename temp: %w", err)
	}
	return nil
}

// RestoreImages reads the JSON file at `path` (written by
// PersistImages) and loads every entry into `Images`. A missing file
// is not an error — a fresh environment starts with an empty store.
func (s *Store) RestoreImages(path string) error {
	if path == "" {
		return nil
	}
	data, err := os.ReadFile(path)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return fmt.Errorf("read images snapshot: %w", err)
	}
	var snap imagesSnapshot
	if err := json.Unmarshal(data, &snap); err != nil {
		return fmt.Errorf("unmarshal images snapshot: %w", err)
	}
	if snap.Version != imagesSnapshotVersion {
		return fmt.Errorf("images snapshot: unknown version %d", snap.Version)
	}
	s.Images.mu.Lock()
	for k, v := range snap.Items {
		s.Images.items[k] = v
	}
	s.Images.mu.Unlock()
	return nil
}

// DefaultImageStatePath returns `$SOCKERLESS_STATE_DIR/images.json`,
// or `$HOME/.sockerless/state/images.json` when the env var is unset.
// Empty-string return means no persistence (e.g. no $HOME available).
func DefaultImageStatePath() string {
	if dir := os.Getenv("SOCKERLESS_STATE_DIR"); dir != "" {
		return filepath.Join(dir, "images.json")
	}
	home, err := os.UserHomeDir()
	if err != nil || home == "" {
		return ""
	}
	return filepath.Join(home, ".sockerless", "state", "images.json")
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
