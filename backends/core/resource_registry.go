package core

import (
	"encoding/json"
	"os"
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

// autoSave persists the registry to disk, logging warnings on failure.
func (rr *ResourceRegistry) autoSave() {
	if err := rr.Save(); err != nil {
		rr.logger.Warn().Err(err).Msg("failed to auto-save resource registry")
	}
}

// Save writes the registry to disk as JSON using atomic rename.
func (rr *ResourceRegistry) Save() error {
	if rr.filePath == "" {
		return nil
	}
	rr.mu.RLock()
	data, err := json.MarshalIndent(rr.entries, "", "  ")
	rr.mu.RUnlock()
	if err != nil {
		return err
	}
	tmpPath := rr.filePath + ".tmp"
	if err := os.WriteFile(tmpPath, data, 0644); err != nil {
		return err
	}
	return os.Rename(tmpPath, rr.filePath)
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
