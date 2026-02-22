package core

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestRegistryRegisterAndListActive(t *testing.T) {
	rr := NewResourceRegistry("")
	rr.Register(ResourceEntry{
		ContainerID:  "abc123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:aws:ecs:us-east-1:123:task/cluster/abc",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
	})
	rr.Register(ResourceEntry{
		ContainerID:  "def456",
		Backend:      "lambda",
		ResourceType: "function",
		ResourceID:   "arn:aws:lambda:us-east-1:123:function:skls-def456",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
	})

	active := rr.ListActive()
	if len(active) != 2 {
		t.Fatalf("expected 2 active, got %d", len(active))
	}
}

func TestRegistryMarkCleanedUp(t *testing.T) {
	rr := NewResourceRegistry("")
	id := "arn:aws:ecs:us-east-1:123:task/cluster/abc"
	rr.Register(ResourceEntry{
		ContainerID:  "abc123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   id,
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
	})

	rr.MarkCleanedUp(id)

	active := rr.ListActive()
	if len(active) != 0 {
		t.Fatalf("expected 0 active after cleanup, got %d", len(active))
	}

	// Should still be in ListAll
	all := rr.ListAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 total, got %d", len(all))
	}
}

func TestRegistryListOrphaned(t *testing.T) {
	rr := NewResourceRegistry("")

	// Old entry (2 hours ago)
	rr.Register(ResourceEntry{
		ContainerID:  "old123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:old",
		InstanceID:   "host-1",
		CreatedAt:    time.Now().Add(-2 * time.Hour),
	})

	// Recent entry
	rr.Register(ResourceEntry{
		ContainerID:  "new456",
		Backend:      "lambda",
		ResourceType: "function",
		ResourceID:   "arn:new",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
	})

	orphans := rr.ListOrphaned(time.Hour)
	if len(orphans) != 1 {
		t.Fatalf("expected 1 orphan, got %d", len(orphans))
	}
	if orphans[0].ResourceID != "arn:old" {
		t.Errorf("expected orphan arn:old, got %s", orphans[0].ResourceID)
	}
}

func TestRegistrySaveLoad(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	rr := NewResourceRegistry(path)
	rr.Register(ResourceEntry{
		ContainerID:  "abc123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:test",
		InstanceID:   "host-1",
		CreatedAt:    time.Date(2025, 1, 15, 10, 0, 0, 0, time.UTC),
	})

	if err := rr.Save(); err != nil {
		t.Fatalf("Save failed: %v", err)
	}

	// Verify file exists
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("registry file not found: %v", err)
	}

	// Load into new registry
	rr2 := NewResourceRegistry(path)
	if err := rr2.Load(); err != nil {
		t.Fatalf("Load failed: %v", err)
	}

	active := rr2.ListActive()
	if len(active) != 1 {
		t.Fatalf("expected 1 active after load, got %d", len(active))
	}
	if active[0].ContainerID != "abc123" {
		t.Errorf("expected container abc123, got %s", active[0].ContainerID)
	}
}

func TestRegistryLoadNonExistent(t *testing.T) {
	rr := NewResourceRegistry("/nonexistent/path/registry.json")
	if err := rr.Load(); err != nil {
		t.Fatalf("Load of nonexistent file should not error, got: %v", err)
	}
}
