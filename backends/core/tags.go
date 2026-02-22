package core

import (
	"os"
	"strings"
	"time"
)

// TagSet holds the standard Sockerless tags for a cloud resource.
type TagSet struct {
	ContainerID string
	Backend     string
	InstanceID  string
	CreatedAt   time.Time
}

// AsMap returns tags as map[string]string (for AWS, general use).
func (ts TagSet) AsMap() map[string]string {
	return map[string]string{
		"sockerless-managed":      "true",
		"sockerless-container-id": truncate(ts.ContainerID, 12),
		"sockerless-backend":      ts.Backend,
		"sockerless-instance":     ts.InstanceID,
		"sockerless-created-at":   ts.CreatedAt.UTC().Format(time.RFC3339),
	}
}

// AsGCPLabels returns tags with GCP-safe key format (underscores, lowercase).
// GCP labels: keys and values must be lowercase, only [a-z0-9_-], values max 63 chars.
func (ts TagSet) AsGCPLabels() map[string]string {
	m := ts.AsMap()
	result := make(map[string]string, len(m))
	for k, v := range m {
		gcpKey := strings.ReplaceAll(k, "-", "_")
		result[gcpKey] = truncate(v, 63)
	}
	return result
}

// AsAzurePtrMap returns tags as map[string]*string (Azure SDK convention).
func (ts TagSet) AsAzurePtrMap() map[string]*string {
	m := ts.AsMap()
	result := make(map[string]*string, len(m))
	for k, v := range m {
		v := v // copy for pointer
		result[k] = &v
	}
	return result
}

// DefaultInstanceID returns the hostname or "unknown" if unavailable.
func DefaultInstanceID() string {
	h, err := os.Hostname()
	if err != nil || h == "" {
		return "unknown"
	}
	return h
}

func truncate(s string, n int) string {
	if len(s) <= n {
		return s
	}
	return s[:n]
}
