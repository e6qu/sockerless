package core

import (
	"context"
	"net/http"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// mockCloudState is defined in resolve_test.go — reused here.

func newExtendedTestServer() *BaseServer {
	store := NewStore()
	s := &BaseServer{
		Store:          store,
		Logger:         zerolog.Nop(),
		Mux:            http.NewServeMux(),
		EventBus:       NewEventBus(),
		PendingCreates: NewStateStore[api.Container](),
	}
	s.InitDrivers()
	s.self = s
	return s
}

func TestCollectAllContainers_StoreOnly(t *testing.T) {
	s := newExtendedTestServer()
	s.Store.Containers.Put("store-1", api.Container{ID: "store-1", Name: "/store1"})
	s.Store.Containers.Put("store-2", api.Container{ID: "store-2", Name: "/store2"})

	all := s.collectAllContainers(context.Background())
	if len(all) != 2 {
		t.Fatalf("expected 2 containers, got %d", len(all))
	}
}

func TestCollectAllContainers_MergesStoreAndCloudState(t *testing.T) {
	s := newExtendedTestServer()

	// Add container to Store
	s.Store.Containers.Put("store-id", api.Container{ID: "store-id", Name: "/store"})

	// Add container to PendingCreates
	s.PendingCreates.Put("pending-id", api.Container{ID: "pending-id", Name: "/pending"})

	// Set mock CloudState
	s.CloudState = &mockCloudState{containers: []api.Container{
		{ID: "cloud-id", Name: "/cloud"},
	}}

	all := s.collectAllContainers(context.Background())
	if len(all) != 3 {
		t.Fatalf("expected 3 containers, got %d", len(all))
	}

	ids := make(map[string]bool)
	for _, c := range all {
		ids[c.ID] = true
	}
	for _, expected := range []string{"store-id", "cloud-id", "pending-id"} {
		if !ids[expected] {
			t.Fatalf("expected container %q in results", expected)
		}
	}
}

func TestCollectAllContainers_DeduplicatesByID(t *testing.T) {
	s := newExtendedTestServer()

	// Same ID in both store and cloud state
	s.Store.Containers.Put("dup-id", api.Container{ID: "dup-id", Name: "/dup-store"})
	s.CloudState = &mockCloudState{containers: []api.Container{
		{ID: "dup-id", Name: "/dup-cloud"},
	}}

	all := s.collectAllContainers(context.Background())
	if len(all) != 1 {
		t.Fatalf("expected 1 container (deduplicated), got %d", len(all))
	}
	// Store takes precedence (added first)
	if all[0].Name != "/dup-store" {
		t.Fatalf("expected store container to take precedence, got name %q", all[0].Name)
	}
}

func TestCollectAllContainers_NilCloudState(t *testing.T) {
	s := newExtendedTestServer()
	s.CloudState = nil
	s.Store.Containers.Put("local-id", api.Container{ID: "local-id", Name: "/local"})

	all := s.collectAllContainers(context.Background())
	if len(all) != 1 {
		t.Fatalf("expected 1 container, got %d", len(all))
	}
}

func TestCollectAllContainers_EmptyAllSources(t *testing.T) {
	s := newExtendedTestServer()

	all := s.collectAllContainers(context.Background())
	if len(all) != 0 {
		t.Fatalf("expected 0 containers, got %d", len(all))
	}
}

func TestCollectAllContainers_PendingCreateDedup(t *testing.T) {
	s := newExtendedTestServer()

	// Same ID in store and pending creates
	s.Store.Containers.Put("same-id", api.Container{ID: "same-id", Name: "/in-store"})
	s.PendingCreates.Put("same-id", api.Container{ID: "same-id", Name: "/in-pending"})

	all := s.collectAllContainers(context.Background())
	if len(all) != 1 {
		t.Fatalf("expected 1 container (deduplicated), got %d", len(all))
	}
}
