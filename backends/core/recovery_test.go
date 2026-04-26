package core

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/sockerless/api"
)

func apiContainer(id, name, image string) api.Container {
	return api.Container{
		ID:    id,
		Name:  name,
		Image: image,
		State: api.ContainerState{Status: "running", Running: true},
	}
}

// mockScanner implements CloudScanner for testing.
type mockScanner struct {
	orphans []ResourceEntry
	err     error
}

func (m *mockScanner) ScanOrphanedResources(_ context.Context, _ string) ([]ResourceEntry, error) {
	return m.orphans, m.err
}

func (m *mockScanner) CleanupResource(_ context.Context, _ ResourceEntry) error {
	return nil
}

// Recovery tests now exercise the in-memory + cloud-scan path only —
// the disk-backed Save/Load was removed (stateless invariant).

func TestRecoverOnStartupLoadsCloudOrphans(t *testing.T) {
	rr := NewResourceRegistry("")
	scanner := &mockScanner{
		orphans: []ResourceEntry{
			{
				ContainerID:  "abc123",
				Backend:      "ecs",
				ResourceType: "task",
				ResourceID:   "arn:cloud",
				InstanceID:   "host-1",
				CreatedAt:    time.Now(),
			},
		},
	}
	if err := RecoverOnStartup(context.Background(), rr, scanner, "host-1"); err != nil {
		t.Fatalf("RecoverOnStartup failed: %v", err)
	}
	active := rr.ListActive()
	if len(active) != 1 {
		t.Fatalf("expected 1 active, got %d", len(active))
	}
	if active[0].ResourceID != "arn:cloud" {
		t.Errorf("expected arn:cloud, got %s", active[0].ResourceID)
	}
}

func TestRecoverOnStartupSkipsAlreadyKnown(t *testing.T) {
	rr := NewResourceRegistry("")
	rr.Register(ResourceEntry{
		ContainerID:  "abc123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:known",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
	})
	scanner := &mockScanner{
		orphans: []ResourceEntry{
			{ResourceID: "arn:known", ContainerID: "abc123", Backend: "ecs", ResourceType: "task", InstanceID: "host-1", CreatedAt: time.Now()},
			{ResourceID: "arn:new", ContainerID: "def456", Backend: "ecs", ResourceType: "task", InstanceID: "host-1", CreatedAt: time.Now()},
		},
	}
	if err := RecoverOnStartup(context.Background(), rr, scanner, "host-1"); err != nil {
		t.Fatalf("RecoverOnStartup failed: %v", err)
	}
	all := rr.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries (deduped), got %d", len(all))
	}
}

func TestRecoverOnStartupScanError(t *testing.T) {
	rr := NewResourceRegistry("")
	scanner := &mockScanner{err: fmt.Errorf("cloud scan failed")}
	err := RecoverOnStartup(context.Background(), rr, scanner, "host-1")
	if err == nil {
		t.Fatal("expected error from scanner, got nil")
	}
}

func TestReconstructContainerState(t *testing.T) {
	store := NewStore()
	rr := NewResourceRegistry("")
	rr.Register(ResourceEntry{
		ContainerID:  "abc123def456",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:reconstruct",
		InstanceID:   "host-1",
		CreatedAt:    time.Date(2025, 6, 1, 12, 0, 0, 0, time.UTC),
		Metadata:     map[string]string{"image": "alpine:3.18", "name": "/mycontainer"},
	})

	recovered := ReconstructContainerState(store, rr)
	if recovered != 1 {
		t.Fatalf("expected 1 recovered, got %d", recovered)
	}

	c, ok := store.Containers.Get("abc123def456")
	if !ok {
		t.Fatal("container not found in store")
	}
	if c.Image != "alpine:3.18" {
		t.Errorf("expected image alpine:3.18, got %s", c.Image)
	}
	if c.Name != "/mycontainer" {
		t.Errorf("expected name /mycontainer, got %s", c.Name)
	}
	if !c.State.Running {
		t.Error("expected container to be running")
	}
}

func TestReconstructSkipsExisting(t *testing.T) {
	store := NewStore()
	// Pre-populate store
	store.Containers.Put("existing123", apiContainer("existing123", "/existing", "nginx:latest"))

	rr := NewResourceRegistry("")
	rr.Register(ResourceEntry{
		ContainerID:  "existing123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:existing",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": "alpine:3.18", "name": "/overwrite"},
	})

	recovered := ReconstructContainerState(store, rr)
	if recovered != 0 {
		t.Fatalf("expected 0 recovered (skip existing), got %d", recovered)
	}

	c, _ := store.Containers.Get("existing123")
	if c.Image != "nginx:latest" {
		t.Errorf("expected original image nginx:latest, got %s", c.Image)
	}
}

func TestReconstructSkipsCleanedUp(t *testing.T) {
	store := NewStore()
	rr := NewResourceRegistry("")
	rr.Register(ResourceEntry{
		ContainerID:  "cleaned123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:cleaned",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
		Metadata:     map[string]string{"image": "alpine:3.18", "name": "/cleaned"},
	})
	rr.MarkCleanedUp("arn:cleaned")

	recovered := ReconstructContainerState(store, rr)
	if recovered != 0 {
		t.Fatalf("expected 0 recovered (skip cleaned up), got %d", recovered)
	}
}

func TestReconstructDefaultName(t *testing.T) {
	store := NewStore()
	rr := NewResourceRegistry("")
	rr.Register(ResourceEntry{
		ContainerID:  "abcdef123456789",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:noname",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
	})

	recovered := ReconstructContainerState(store, rr)
	if recovered != 1 {
		t.Fatalf("expected 1 recovered, got %d", recovered)
	}

	c, _ := store.Containers.Get("abcdef123456789")
	if c.Name != "/abcdef123456" {
		t.Errorf("expected default name /abcdef123456, got %s", c.Name)
	}
}
