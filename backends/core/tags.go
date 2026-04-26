package core

import (
	"encoding/base64"
	"encoding/json"
	"os"
	"strconv"
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
	Name         string            // Docker container name (e.g., "/my-nginx")
	Network      string            // Docker network name (empty = bridge)
	Pod          string            // Pod name (empty = no pod)
	Labels       map[string]string // Docker labels
	Tty          bool              // Allocate a pseudo-TTY
	RestartCount int               // Number of restarts this container has undergone
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
	if ts.RestartCount > 0 {
		m["sockerless-restart-count"] = strconv.Itoa(ts.RestartCount)
	}

	// Docker labels as URL-safe-base64-encoded JSON without padding.
	// Raw JSON contains `{`, `"`, `,`, `}` which AWS ECS tag values
	// reject (`UTF-8 letters, spaces, numbers and _ . / = + - : @`).
	// URL-safe base64 uses `-` and `_` instead of `+` and `/` and we
	// drop padding, so the output stays within GCP's label charset
	// `[a-z0-9_-]` too. That lets ParseLabelsFromTags round-trip an
	// arbitrary label set across every supported backend.
	if len(ts.Labels) > 0 {
		labelsJSON, _ := json.Marshal(ts.Labels)
		encoded := base64.RawURLEncoding.EncodeToString(labelsJSON)
		if len(encoded) <= 256 {
			m["sockerless-labels-b64"] = encoded
		} else {
			for i := 0; i*256 < len(encoded); i++ {
				end := (i + 1) * 256
				if end > len(encoded) {
					end = len(encoded)
				}
				m["sockerless-labels-b64-"+strconv.Itoa(i)] = encoded[i*256 : end]
			}
		}
	}

	return m
}

// ParseLabelsFromTags reconstructs Docker labels from tag map.
// Handles both the current base64(JSON) encoding and the legacy
// raw-JSON encoding for backward compatibility during rollout.
func ParseLabelsFromTags(tags map[string]string) map[string]string {
	// URL-safe-base64-encoded JSON (current format).
	if s, ok := tags["sockerless-labels-b64"]; ok {
		if raw, err := base64.RawURLEncoding.DecodeString(s); err == nil {
			var labels map[string]string
			if json.Unmarshal(raw, &labels) == nil {
				return labels
			}
		}
	}
	// Split base64.
	var b64Parts []string
	for i := 0; ; i++ {
		key := "sockerless-labels-b64-" + strconv.Itoa(i)
		s, ok := tags[key]
		if !ok {
			break
		}
		b64Parts = append(b64Parts, s)
	}
	if len(b64Parts) > 0 {
		if raw, err := base64.RawURLEncoding.DecodeString(strings.Join(b64Parts, "")); err == nil {
			var labels map[string]string
			if json.Unmarshal(raw, &labels) == nil {
				return labels
			}
		}
	}
	// Legacy raw JSON.
	if s, ok := tags["sockerless-labels"]; ok {
		var labels map[string]string
		if json.Unmarshal([]byte(s), &labels) == nil {
			return labels
		}
	}
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
// Previously every value was blindly truncated and pushed into
// labels, which caused GCP's ARM validator to reject the whole
// resource when the JSON blob appeared.
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
