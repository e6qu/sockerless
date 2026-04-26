package core

import (
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

// Persistence tests removed: Save/Load collapsed to no-ops to keep
// the registry purely in-memory (stateless invariant). The surviving
// tests cover the in-memory semantics.

func TestRegistryStatusTransitions(t *testing.T) {
	rr := NewResourceRegistry("")
	rr.Register(ResourceEntry{
		ContainerID:  "abc123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:status",
		InstanceID:   "host-1",
		Status:       "pending",
		CreatedAt:    time.Now(),
	})

	// Pending is active
	if active := rr.ListActive(); len(active) != 1 {
		t.Fatalf("pending should be active, got %d", len(active))
	}

	// Mark active
	rr.MarkActive("arn:status")
	all := rr.ListAll()
	if all[0].Status != "active" {
		t.Fatalf("expected status active, got %s", all[0].Status)
	}
	if active := rr.ListActive(); len(active) != 1 {
		t.Fatalf("active should be active, got %d", len(active))
	}

	// Clean up
	rr.MarkCleanedUp("arn:status")
	if active := rr.ListActive(); len(active) != 0 {
		t.Fatalf("cleaned up should not be active, got %d", len(active))
	}
}

func TestRegistryBackwardCompatibility(t *testing.T) {
	rr := NewResourceRegistry("")
	// Entry with empty status (as would be loaded from old format)
	rr.Register(ResourceEntry{
		ContainerID:  "abc123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:compat",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
	})

	// Empty status should be treated as active
	if active := rr.ListActive(); len(active) != 1 {
		t.Fatalf("empty status should be active, got %d", len(active))
	}
}

func TestRegistryMetadataInMemory(t *testing.T) {
	rr := NewResourceRegistry("")
	rr.Register(ResourceEntry{
		ContainerID:  "abc123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:meta",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": "alpine:3.18", "name": "/mycontainer"},
	})

	all := rr.ListAll()
	if len(all) != 1 {
		t.Fatalf("expected 1 entry, got %d", len(all))
	}
	if all[0].Metadata["image"] != "alpine:3.18" {
		t.Errorf("expected image alpine:3.18, got %s", all[0].Metadata["image"])
	}
	if all[0].Metadata["name"] != "/mycontainer" {
		t.Errorf("expected name /mycontainer, got %s", all[0].Metadata["name"])
	}
}
