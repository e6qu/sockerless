package core

import (
	"testing"
	"time"
)

func TestTagSetAsMap(t *testing.T) {
	ts := TagSet{
		ContainerID: "abcdef123456789",
		Backend:     "ecs",
		Cluster:     "my-cluster",
		InstanceID:  "host-1",
		CreatedAt:   time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
		Name:        "/my-nginx",
		Network:     "testnet",
	}
	m := ts.AsMap()

	// Check required keys
	checks := map[string]string{
		"sockerless-managed":      "true",
		"sockerless-container-id": "abcdef123456789", // full ID, not truncated
		"sockerless-backend":      "ecs",
		"sockerless-cluster":      "my-cluster",
		"sockerless-instance":     "host-1",
		"sockerless-created-at":   "2025-01-15T10:30:00Z",
		"sockerless-name":         "/my-nginx",
		"sockerless-network":      "testnet",
	}

	for k, v := range checks {
		if m[k] != v {
			t.Errorf("key %q: expected %q, got %q", k, v, m[k])
		}
	}
}

func TestTagSetAsGCPLabels(t *testing.T) {
	ts := TagSet{
		ContainerID: "abcdef123456",
		Backend:     "cloudrun",
		InstanceID:  "host-1",
		CreatedAt:   time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	m := ts.AsGCPLabels()

	// Keys should use underscores instead of hyphens
	for k := range m {
		for _, c := range k {
			if c == '-' {
				// GCP labels can have hyphens, but our convention uses underscores for keys
				// Actually the plan says replace hyphens with underscores in keys
				t.Errorf("key should use underscores, not hyphens: %q", k)
			}
		}
	}

	if v, ok := m["sockerless_managed"]; !ok || v != "true" {
		t.Errorf("expected sockerless_managed=true, got %q", v)
	}
	if v, ok := m["sockerless_backend"]; !ok || v != "cloudrun" {
		t.Errorf("expected sockerless_backend=cloudrun, got %q", v)
	}
}

func TestTagSetAsGCPLabelsValueTruncation(t *testing.T) {
	longValue := ""
	for i := 0; i < 100; i++ {
		longValue += "a"
	}
	ts := TagSet{
		ContainerID: "abcdef123456",
		Backend:     "cloudrun",
		InstanceID:  longValue,
		CreatedAt:   time.Now(),
	}
	m := ts.AsGCPLabels()
	if len(m["sockerless_instance"]) > 63 {
		t.Errorf("GCP label value should be truncated to 63 chars, got %d", len(m["sockerless_instance"]))
	}
}

func TestTagSetAsAzurePtrMap(t *testing.T) {
	ts := TagSet{
		ContainerID: "abcdef123456",
		Backend:     "aca",
		InstanceID:  "host-1",
		CreatedAt:   time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	m := ts.AsAzurePtrMap()

	if len(m) != 5 {
		t.Fatalf("expected 5 keys, got %d", len(m))
	}

	for k, v := range m {
		if v == nil {
			t.Errorf("key %q has nil value", k)
		}
	}

	if *m["sockerless-managed"] != "true" {
		t.Errorf("expected sockerless-managed=true, got %q", *m["sockerless-managed"])
	}
	if *m["sockerless-backend"] != "aca" {
		t.Errorf("expected sockerless-backend=aca, got %q", *m["sockerless-backend"])
	}
}

func TestDefaultInstanceID(t *testing.T) {
	id := DefaultInstanceID()
	if id == "" {
		t.Error("DefaultInstanceID should return non-empty string")
	}
}

func TestAsGCPAnnotations_LongValue(t *testing.T) {
	// A 64-char container ID exceeds the 63-char GCP label limit,
	// so it should appear in annotations.
	longID := "abcdef1234567890abcdef1234567890abcdef1234567890abcdef1234567890" // 64 chars
	ts := TagSet{
		ContainerID: longID,
		Backend:     "cloudrun",
		InstanceID:  "host-1",
		CreatedAt:   time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	ann := ts.AsGCPAnnotations()
	if v, ok := ann["sockerless_container_id"]; !ok {
		t.Error("expected sockerless_container_id in annotations for 64-char ID")
	} else if v != longID {
		t.Errorf("expected full ID in annotation, got %q", v)
	}
}

func TestAsGCPAnnotations_AllShort(t *testing.T) {
	ts := TagSet{
		ContainerID: "abcdef123456", // 12 chars, well under 63
		Backend:     "cloudrun",
		InstanceID:  "host-1",
		CreatedAt:   time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	ann := ts.AsGCPAnnotations()
	if len(ann) != 0 {
		t.Errorf("expected empty annotations when all values <=63 chars, got %d entries", len(ann))
	}
}

func TestParseLabelFromTags_Simple(t *testing.T) {
	tags := map[string]string{
		"sockerless-labels": `{"env":"prod","app":"web"}`,
	}
	labels := ParseLabelsFromTags(tags)
	if labels == nil {
		t.Fatal("expected non-nil labels")
	}
	if labels["env"] != "prod" {
		t.Errorf("expected env=prod, got %q", labels["env"])
	}
	if labels["app"] != "web" {
		t.Errorf("expected app=web, got %q", labels["app"])
	}
}

func TestParseLabelFromTags_MultiChunk(t *testing.T) {
	// Build a labels JSON string > 256 chars to force splitting
	labels := map[string]string{
		"description": "This is a very long label value that is used to test the multi-chunk splitting behavior of the tag serialization code in sockerless",
		"another":     "Another long value to push us well over the two hundred and fifty six character limit for a single tag value field",
	}

	// Produce the tag map using TagSet
	ts := TagSet{
		ContainerID: "abc123",
		Backend:     "ecs",
		InstanceID:  "host-1",
		CreatedAt:   time.Now(),
		Labels:      labels,
	}
	tags := ts.AsMap()

	// Verify it was split (no single sockerless-labels key)
	if _, ok := tags["sockerless-labels"]; ok {
		t.Fatal("expected labels to be split across chunks, not stored in single key")
	}
	if _, ok := tags["sockerless-labels-0"]; !ok {
		t.Fatal("expected sockerless-labels-0 chunk")
	}

	// Round-trip through ParseLabelsFromTags
	parsed := ParseLabelsFromTags(tags)
	if parsed == nil {
		t.Fatal("expected non-nil labels from multi-chunk parse")
	}
	for k, v := range labels {
		if parsed[k] != v {
			t.Errorf("key %q: expected %q, got %q", k, v, parsed[k])
		}
	}
}

func TestFullContainerID(t *testing.T) {
	ts := TagSet{
		ContainerID: "abcdef1234567890extra",
		Backend:     "ecs",
		InstanceID:  "host",
		CreatedAt:   time.Now(),
	}
	m := ts.AsMap()
	// Full container ID stored in tags (not truncated — needed for stateless lookup)
	if m["sockerless-container-id"] != "abcdef1234567890extra" {
		t.Errorf("expected full container ID, got %q", m["sockerless-container-id"])
	}
}
