package core

import (
	"encoding/json"
	"os"
	"strings"
	"time"
)

// TagSet holds the standard Sockerless tags for a cloud resource.
// Tags carry Docker-specific metadata that has no cloud-native equivalent.
// Cloud resource config (image, cmd, env, etc.) is the primary data source.
type TagSet struct {
	// Identity
	ContainerID string // Full 64-char Docker container ID
	Backend     string // ecs, lambda, cloudrun, etc.
	InstanceID  string // Deprecated: use Cluster instead for stateless model
	Cluster     string // Cluster/project/resource-group identifier
	CreatedAt   time.Time

	// Docker-specific (no cloud equivalent)
	Name    string            // Docker container name (e.g., "/my-nginx")
	Network string            // Docker network name (empty = bridge)
	Pod     string            // Pod name (empty = no pod)
	Labels  map[string]string // Docker labels
	Tty     bool              // Allocate a pseudo-TTY
}

// AsMap returns tags as map[string]string for AWS.
func (ts TagSet) AsMap() map[string]string {
	m := map[string]string{
		"sockerless-managed":      "true",
		"sockerless-backend":      ts.Backend,
		"sockerless-container-id": ts.ContainerID,
		"sockerless-created-at":   ts.CreatedAt.UTC().Format(time.RFC3339),
	}

	// Identity: prefer cluster, fall back to instance
	if ts.Cluster != "" {
		m["sockerless-cluster"] = ts.Cluster
	}
	if ts.InstanceID != "" {
		m["sockerless-instance"] = ts.InstanceID
	}

	// Docker-specific metadata (only set when non-empty)
	if ts.Name != "" {
		m["sockerless-name"] = ts.Name
	}
	if ts.Network != "" && ts.Network != "bridge" {
		m["sockerless-network"] = ts.Network
	}
	if ts.Pod != "" {
		m["sockerless-pod"] = ts.Pod
	}
	if ts.Tty {
		m["sockerless-tty"] = "true"
	}

	// Docker labels as JSON (split across multiple tags if >256 chars)
	if len(ts.Labels) > 0 {
		labelsJSON, _ := json.Marshal(ts.Labels)
		s := string(labelsJSON)
		if len(s) <= 256 {
			m["sockerless-labels"] = s
		} else {
			// Split across multiple tags
			for i := 0; i*256 < len(s); i++ {
				end := (i + 1) * 256
				if end > len(s) {
					end = len(s)
				}
				m["sockerless-labels-"+string(rune('0'+i))] = s[i*256 : end]
			}
		}
	}

	return m
}

// ParseLabelsFromTags reconstructs Docker labels from tag map.
func ParseLabelsFromTags(tags map[string]string) map[string]string {
	// Try single tag first
	if s, ok := tags["sockerless-labels"]; ok {
		var labels map[string]string
		if json.Unmarshal([]byte(s), &labels) == nil {
			return labels
		}
	}
	// Try split tags
	var parts []string
	for i := 0; ; i++ {
		key := "sockerless-labels-" + string(rune('0'+i))
		s, ok := tags[key]
		if !ok {
			break
		}
		parts = append(parts, s)
	}
	if len(parts) > 0 {
		var labels map[string]string
		if json.Unmarshal([]byte(strings.Join(parts, "")), &labels) == nil {
			return labels
		}
	}
	return nil
}

// AsGCPLabels returns tags as GCP-compatible labels (max 63 chars,
// charset: [a-z0-9_-]). Values containing characters outside the GCP
// label charset (e.g. the sockerless-labels JSON blob's `{`, `:`, `"`)
// are dropped from labels — callers should pair this with
// AsGCPAnnotations which captures the same data without charset limits.
// Phase 97 (BUG-746): previously every value was blindly truncated and
// pushed into labels, which caused GCP's ARM validator to reject the
// whole resource when the JSON blob appeared.
func (ts TagSet) AsGCPLabels() map[string]string {
	m := ts.AsMap()
	result := make(map[string]string, len(m))
	for k, v := range m {
		if !gcpLabelValueValid(v) {
			continue
		}
		gcpKey := strings.ReplaceAll(k, "-", "_")
		result[gcpKey] = truncate(v, 63)
	}
	return result
}

// AsGCPAnnotations returns tags that can't fit in GCP labels — either
// they contain characters outside the GCP label charset, or they exceed
// the 63-char label-value limit. GCP annotations allow up to 32768
// chars per value with arbitrary UTF-8, so Docker labels (which can
// carry anything) always land here.
func (ts TagSet) AsGCPAnnotations() map[string]string {
	m := ts.AsMap()
	result := make(map[string]string)
	for k, v := range m {
		if !gcpLabelValueValid(v) || len(v) > 63 {
			gcpKey := strings.ReplaceAll(k, "-", "_")
			result[gcpKey] = v
		}
	}
	return result
}

// gcpLabelValueValid reports whether a value is acceptable as a GCP
// resource label value: only lowercase letters, digits, dashes, and
// underscores are allowed. Values violating this must go into
// annotations instead.
func gcpLabelValueValid(v string) bool {
	for _, r := range v {
		switch {
		case r >= 'a' && r <= 'z',
			r >= '0' && r <= '9',
			r == '-', r == '_':
			continue
		default:
			return false
		}
	}
	return true
}

// AsAzurePtrMap returns tags as map[string]*string (Azure SDK convention).
func (ts TagSet) AsAzurePtrMap() map[string]*string {
	m := ts.AsMap()
	result := make(map[string]*string, len(m))
	for k, v := range m {
		v := v
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
