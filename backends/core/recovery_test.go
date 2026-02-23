package core

import (
	"context"
	"fmt"
	"path/filepath"
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

func TestRecoverOnStartupLoadsFromDisk(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	// Seed registry file
	rr := NewResourceRegistry(path)
	rr.Register(ResourceEntry{
		ContainerID:  "abc123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:seeded",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
	})

	// New registry, load via RecoverOnStartup
	rr2 := NewResourceRegistry(path)
	scanner := &mockScanner{}
	if err := RecoverOnStartup(context.Background(), rr2, scanner, "host-1"); err != nil {
		t.Fatalf("RecoverOnStartup failed: %v", err)
	}

	active := rr2.ListActive()
	if len(active) != 1 {
		t.Fatalf("expected 1 active, got %d", len(active))
	}
	if active[0].ResourceID != "arn:seeded" {
		t.Errorf("expected arn:seeded, got %s", active[0].ResourceID)
	}
}

func TestRecoverOnStartupMergesOrphans(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	// Seed one entry
	rr := NewResourceRegistry(path)
	rr.Register(ResourceEntry{
		ContainerID:  "abc123",
		Backend:      "ecs",
		ResourceType: "task",
		ResourceID:   "arn:known",
		InstanceID:   "host-1",
		CreatedAt:    time.Now(),
	})

	// Recover with scanner that finds an orphan
	rr2 := NewResourceRegistry(path)
	scanner := &mockScanner{
		orphans: []ResourceEntry{
			{
				ContainerID:  "orphan1",
				Backend:      "ecs",
				ResourceType: "task",
				ResourceID:   "arn:orphan",
				InstanceID:   "host-1",
				CreatedAt:    time.Now().Add(-1 * time.Hour),
			},
		},
	}
	if err := RecoverOnStartup(context.Background(), rr2, scanner, "host-1"); err != nil {
		t.Fatalf("RecoverOnStartup failed: %v", err)
	}

	all := rr2.ListAll()
	if len(all) != 2 {
		t.Fatalf("expected 2 entries (known + orphan), got %d", len(all))
	}
}

func TestRecoverOnStartupScanError(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "registry.json")

	rr := NewResourceRegistry(path)
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
