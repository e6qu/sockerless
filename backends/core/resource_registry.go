package core

import (
	"encoding/json"
	"os"
	"sync"
	"time"
)

// ResourceEntry tracks a single cloud resource.
type ResourceEntry struct {
	ContainerID  string    `json:"containerId"`
	Backend      string    `json:"backend"`
	ResourceType string    `json:"resourceType"` // "task", "function", "job", "site"
	ResourceID   string    `json:"resourceId"`   // ARN or full name
	InstanceID   string    `json:"instanceId"`
	CreatedAt    time.Time `json:"createdAt"`
	CleanedUp    bool      `json:"cleanedUp"`
}

// ResourceRegistry tracks all cloud resources created by this instance.
type ResourceRegistry struct {
	mu       sync.RWMutex
	entries  map[string]*ResourceEntry // keyed by ResourceID
	filePath string                     // for JSON persistence
}

// NewResourceRegistry creates a new resource registry.
// If filePath is empty, persistence is disabled.
func NewResourceRegistry(filePath string) *ResourceRegistry {
	return &ResourceRegistry{
		entries:  make(map[string]*ResourceEntry),
		filePath: filePath,
	}
}

// Register adds a resource entry to the registry.
func (rr *ResourceRegistry) Register(entry ResourceEntry) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	rr.entries[entry.ResourceID] = &entry
}

// MarkCleanedUp marks a resource as cleaned up.
func (rr *ResourceRegistry) MarkCleanedUp(resourceID string) {
	rr.mu.Lock()
	defer rr.mu.Unlock()
	if e, ok := rr.entries[resourceID]; ok {
		e.CleanedUp = true
	}
}

// ListActive returns all entries that have not been cleaned up.
func (rr *ResourceRegistry) ListActive() []ResourceEntry {
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	var result []ResourceEntry
	for _, e := range rr.entries {
		if !e.CleanedUp {
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
		if !e.CleanedUp && e.CreatedAt.Before(cutoff) {
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

// Save writes the registry to disk as JSON.
func (rr *ResourceRegistry) Save() error {
	if rr.filePath == "" {
		return nil
	}
	rr.mu.RLock()
	defer rr.mu.RUnlock()
	data, err := json.MarshalIndent(rr.entries, "", "  ")
	if err != nil {
		return err
	}
	return os.WriteFile(rr.filePath, data, 0644)
}

// Load reads the registry from disk.
func (rr *ResourceRegistry) Load() error {
	if rr.filePath == "" {
		return nil
	}
	data, err := os.ReadFile(rr.filePath)
	if err != nil {
		if os.IsNotExist(err) {
			return nil
		}
		return err
	}
	rr.mu.Lock()
	defer rr.mu.Unlock()
	return json.Unmarshal(data, &rr.entries)
}
