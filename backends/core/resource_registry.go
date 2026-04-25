package core

import (
	"sync"
	"time"

	"github.com/rs/zerolog"
)

// ResourceEntry tracks a single cloud resource.
type ResourceEntry struct {
	ContainerID  string            `json:"containerId"`
	Backend      string            `json:"backend"`
	ResourceType string            `json:"resourceType"` // "task", "function", "job", "site"
	ResourceID   string            `json:"resourceId"`   // ARN or full name
	InstanceID   string            `json:"instanceId"`
	CreatedAt    time.Time         `json:"createdAt"`
	CleanedUp    bool              `json:"cleanedUp"`
	Status       string            `json:"status,omitempty"`
	Metadata     map[string]string `json:"metadata,omitempty"`
}

// IsActive returns true if the entry represents a live resource.
// Empty status is treated as active for backward compatibility.
func (e *ResourceEntry) IsActive() bool {
	if e.CleanedUp {
		return false
	}
	return e.Status == "" || e.Status == "pending" || e.Status == "active"
}

// ResourceRegistry tracks all cloud resources created by this instance.
type ResourceRegistry struct {
	mu       sync.RWMutex
	entries  map[string]*ResourceEntry // keyed by ResourceID
	filePath string                    // for JSON persistence
	logger   zerolog.Logger
}

// NewResourceRegistry creates a new resource registry.
// If filePath is empty, persistence is disabled.
func NewResourceRegistry(filePath string, logger ...zerolog.Logger) *ResourceRegistry {
	rr := &ResourceRegistry{
		entries:  make(map[string]*ResourceEntry),
		filePath: filePath,
	}
	if len(logger) > 0 {
		rr.logger = logger[0]
	} else {
		rr.logger = zerolog.Nop()
	}
	return rr
}

// Register adds a resource entry to the registry.
func (rr *ResourceRegistry) Register(entry ResourceEntry) {
	rr.mu.Lock()
	rr.entries[entry.ResourceID] = &entry
	rr.mu.Unlock()
	rr.autoSave()
}

// Get returns the entry for the given resource ID.
func (rr *ResourceRegistry) Get(resourceID string) (ResourceEntry, bool) {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	if e, ok := rr.entries[resourceID]; ok {
		return *e, true
	}
	return ResourceEntry{}, false
}

// IsCleanedUp reports whether the given resource is marked cleaned up
// in the registry. Returns false when the resource is unknown.
func (rr *ResourceRegistry) IsCleanedUp(resourceID string) bool {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	if e, ok := rr.entries[resourceID]; ok {
		return e.CleanedUp
	}
	return false
}

// MarkCleanedUp marks a resource as cleaned up.
func (rr *ResourceRegistry) MarkCleanedUp(resourceID string) {
	rr.mu.Lock()
	if e, ok := rr.entries[resourceID]; ok {
		e.CleanedUp = true
		e.Status = "cleanedUp"
	}
	rr.mu.Unlock()
	rr.autoSave()
}

// MarkActive sets a resource's status to "active".
func (rr *ResourceRegistry) MarkActive(resourceID string) {
	rr.mu.Lock()
	if e, ok := rr.entries[resourceID]; ok {
		e.Status = "active"
	}
	rr.mu.Unlock()
	rr.autoSave()
}

// ListActive returns all entries that have not been cleaned up.
func (rr *ResourceRegistry) ListActive() []ResourceEntry {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	var result []ResourceEntry
	for _, e := range rr.entries {
		if e.IsActive() {
			result = append(result, *e)
		}
	}
	return result
}

// ListOrphaned returns active entries older than maxAge.
func (rr *ResourceRegistry) ListOrphaned(maxAge time.Duration) []ResourceEntry {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	cutoff := time.Now().Add(-maxAge)
	var result []ResourceEntry
	for _, e := range rr.entries {
		if e.IsActive() && e.CreatedAt.Before(cutoff) {
			result = append(result, *e)
		}
	}
	return result
}

// ListAll returns all entries.
func (rr *ResourceRegistry) ListAll() []ResourceEntry {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	result := make([]ResourceEntry, 0, len(rr.entries))
	for _, e := range rr.entries {
		result = append(result, *e)
	}
	return result
}

// autoSave is a no-op. The registry is purely in-memory; durable
// state lives in the cloud (sockerless-managed=true tags), and on
// startup `RecoverOnStartup` repopulates the registry from a cloud
// scan. Persisting to disk would (a) make CWD load-bearing for backend
// recovery and (b) drift out of sync with cloud actuals — both
// violations of the stateless invariant in
// `specs/CLOUD_RESOURCE_MAPPING.md` § State boundaries.
func (rr *ResourceRegistry) autoSave() {}

// Save is a no-op kept for API stability. See autoSave.
func (rr *ResourceRegistry) Save() error { return nil }

// Load is a no-op kept for API stability. See autoSave.
func (rr *ResourceRegistry) Load() error { return nil }
