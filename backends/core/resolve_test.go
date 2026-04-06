package core

import (
	"context"
	"testing"

	"github.com/sockerless/api"
)

// mockCloudState is a test double for CloudStateProvider.
type mockCloudState struct {
	containers []api.Container
}

func (m *mockCloudState) GetContainer(_ context.Context, ref string) (api.Container, bool, error) {
	for _, c := range m.containers {
		if c.ID == ref || c.Name == ref || c.Name == "/"+ref {
			return c, true, nil
		}
		if len(ref) >= 3 && len(c.ID) >= len(ref) && c.ID[:len(ref)] == ref {
			return c, true, nil
		}
	}
	return api.Container{}, false, nil
}

func (m *mockCloudState) ListContainers(_ context.Context, _ bool, _ map[string][]string) ([]api.Container, error) {
	return m.containers, nil
}

func (m *mockCloudState) CheckNameAvailable(_ context.Context, name string) (bool, error) {
	for _, c := range m.containers {
		if c.Name == name || c.Name == "/"+name {
			return false, nil
		}
	}
	return true, nil
}

func (m *mockCloudState) WaitForExit(_ context.Context, _ string) (int, error) {
	return 0, nil
}

func TestResolveContainerAuto_PendingCreates(t *testing.T) {
	s := newSpecTestServer()

	id := GenerateID()
	c := api.Container{
		ID:   id,
		Name: "/pending-test",
		State: api.ContainerState{
			Status: "created",
		},
	}
	s.PendingCreates.Put(id, c)

	got, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		t.Fatal("expected to find container in PendingCreates")
	}
	if got.ID != id {
		t.Errorf("expected ID %q, got %q", id, got.ID)
	}
}

func TestResolveContainerAuto_CloudState(t *testing.T) {
	s := newSpecTestServer()

	id := GenerateID()
	cloud := &mockCloudState{
		containers: []api.Container{
			{
				ID:   id,
				Name: "/cloud-test",
				State: api.ContainerState{
					Status:  "running",
					Running: true,
				},
			},
		},
	}
	s.CloudState = cloud

	got, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		t.Fatal("expected to find container via CloudState")
	}
	if got.ID != id {
		t.Errorf("expected ID %q, got %q", id, got.ID)
	}
	if got.State.Status != "running" {
		t.Errorf("expected status running, got %q", got.State.Status)
	}
}

func TestResolveContainerAuto_StoreFallback(t *testing.T) {
	s := newSpecTestServer()

	id, err := createContainerInState(s, "running")
	if err != nil {
		t.Fatalf("setup failed: %v", err)
	}

	// No CloudState set — should fall back to Store
	got, ok := s.ResolveContainerAuto(context.Background(), id)
	if !ok {
		t.Fatal("expected to find container via Store fallback")
	}
	if got.ID != id {
		t.Errorf("expected ID %q, got %q", id, got.ID)
	}
}

func TestResolveContainerAuto_NotFound(t *testing.T) {
	s := newSpecTestServer()
	s.CloudState = &mockCloudState{} // empty

	_, ok := s.ResolveContainerAuto(context.Background(), "nonexistent")
	if ok {
		t.Error("expected container to not be found")
	}
}

func TestResolveContainerAuto_ShortID(t *testing.T) {
	s := newSpecTestServer()

	id := GenerateID()
	c := api.Container{
		ID:   id,
		Name: "/short-id-test",
		State: api.ContainerState{
			Status: "created",
		},
	}
	s.PendingCreates.Put(id, c)

	// Match by first 6 chars
	shortRef := id[:6]
	got, ok := s.ResolveContainerAuto(context.Background(), shortRef)
	if !ok {
		t.Fatalf("expected to find container by short ID %q", shortRef)
	}
	if got.ID != id {
		t.Errorf("expected ID %q, got %q", id, got.ID)
	}
}

func TestResolveContainerAuto_Name(t *testing.T) {
	s := newSpecTestServer()

	id := GenerateID()
	c := api.Container{
		ID:   id,
		Name: "/my-container",
		State: api.ContainerState{
			Status: "created",
		},
	}
	s.PendingCreates.Put(id, c)

	// Match with leading /
	got, ok := s.ResolveContainerAuto(context.Background(), "/my-container")
	if !ok {
		t.Fatal("expected to find container by name with /")
	}
	if got.ID != id {
		t.Errorf("expected ID %q, got %q", id, got.ID)
	}

	// Match without leading /
	got2, ok2 := s.ResolveContainerAuto(context.Background(), "my-container")
	if !ok2 {
		t.Fatal("expected to find container by name without /")
	}
	if got2.ID != id {
		t.Errorf("expected ID %q, got %q", id, got2.ID)
	}
}

func TestResolveContainerIDAuto(t *testing.T) {
	s := newSpecTestServer()

	id := GenerateID()
	c := api.Container{
		ID:   id,
		Name: "/id-auto-test",
		State: api.ContainerState{
			Status: "created",
		},
	}
	s.PendingCreates.Put(id, c)

	gotID, ok := s.ResolveContainerIDAuto(context.Background(), id)
	if !ok {
		t.Fatal("expected to find container ID")
	}
	if gotID != id {
		t.Errorf("expected %q, got %q", id, gotID)
	}

	_, notOK := s.ResolveContainerIDAuto(context.Background(), "nonexistent")
	if notOK {
		t.Error("expected not found for nonexistent ref")
	}
}
