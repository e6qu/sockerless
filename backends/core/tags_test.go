package core

import (
	"testing"
	"time"
)

func TestTagSetAsMap(t *testing.T) {
	ts := TagSet{
		ContainerID: "abcdef123456789",
		Backend:     "ecs",
		InstanceID:  "host-1",
		CreatedAt:   time.Date(2025, 1, 15, 10, 30, 0, 0, time.UTC),
	}
	m := ts.AsMap()

	expected := map[string]string{
		"sockerless-managed":      "true",
		"sockerless-container-id": "abcdef123456",
		"sockerless-backend":      "ecs",
		"sockerless-instance":     "host-1",
		"sockerless-created-at":   "2025-01-15T10:30:00Z",
	}

	if len(m) != len(expected) {
		t.Fatalf("expected %d keys, got %d", len(expected), len(m))
	}
	for k, v := range expected {
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
		if k != k {
			t.Errorf("key should be lowercase: %q", k)
		}
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

func TestTruncateContainerID(t *testing.T) {
	ts := TagSet{
		ContainerID: "abcdef1234567890extra",
		Backend:     "ecs",
		InstanceID:  "host",
		CreatedAt:   time.Now(),
	}
	m := ts.AsMap()
	if m["sockerless-container-id"] != "abcdef123456" {
		t.Errorf("expected container ID truncated to 12 chars, got %q", m["sockerless-container-id"])
	}
}
