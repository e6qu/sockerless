package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newListTestServer() *BaseServer {
	store := NewStore()
	s := &BaseServer{
		Store:    store,
		Logger:   zerolog.Nop(),
		Mux:      http.NewServeMux(),
		EventBus: NewEventBus(),
	}
	s.InitDrivers()
	return s
}

func addContainer(s *BaseServer, id string, name string, image string, running bool, createdOffset time.Duration) {
	created := time.Date(2025, 1, 1, 0, 0, 0, 0, time.UTC).Add(createdOffset)
	c := api.Container{
		ID:      id,
		Name:    "/" + name,
		Created: created.Format(time.RFC3339Nano),
		Config: api.ContainerConfig{
			Image:  image,
			Labels: make(map[string]string),
		},
		HostConfig: api.HostConfig{},
		State: api.ContainerState{
			Status:  "running",
			Running: true,
		},
		NetworkSettings: api.NetworkSettings{
			Networks: make(map[string]*api.EndpointSettings),
		},
		Mounts: make([]api.MountPoint, 0),
	}
	if !running {
		c.State.Status = "exited"
		c.State.Running = false
	}
	s.Store.Containers.Put(id, c)
	s.Store.ContainerNames.Put("/"+name, id)
}

func TestContainerList_LimitParameter(t *testing.T) {
	s := newListTestServer()
	for i := 0; i < 5; i++ {
		addContainer(s, fmt.Sprintf("c%d", i), fmt.Sprintf("test%d", i), "alpine:3.18", true, time.Duration(i)*time.Hour)
	}

	req := httptest.NewRequest("GET", "/internal/v1/containers?all=1&limit=2", nil)
	w := httptest.NewRecorder()
	s.handleContainerList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result []*api.ContainerSummary
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 2 {
		t.Fatalf("expected 2 results, got %d", len(result))
	}
	// Newest first
	if result[0].Created <= result[1].Created {
		t.Fatalf("expected descending order, got %d <= %d", result[0].Created, result[1].Created)
	}
}

func TestContainerList_LimitZeroReturnsAll(t *testing.T) {
	s := newListTestServer()
	for i := 0; i < 3; i++ {
		addContainer(s, fmt.Sprintf("c%d", i), fmt.Sprintf("test%d", i), "alpine:3.18", true, time.Duration(i)*time.Hour)
	}

	req := httptest.NewRequest("GET", "/internal/v1/containers?all=1&limit=0", nil)
	w := httptest.NewRecorder()
	s.handleContainerList(w, req)

	var result []*api.ContainerSummary
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
}

func TestContainerList_SortByCreated(t *testing.T) {
	s := newListTestServer()
	addContainer(s, "c0", "oldest", "alpine:3.18", true, 0)
	addContainer(s, "c1", "middle", "alpine:3.18", true, time.Hour)
	addContainer(s, "c2", "newest", "alpine:3.18", true, 2*time.Hour)

	req := httptest.NewRequest("GET", "/internal/v1/containers?all=1", nil)
	w := httptest.NewRecorder()
	s.handleContainerList(w, req)

	var result []*api.ContainerSummary
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 3 {
		t.Fatalf("expected 3 results, got %d", len(result))
	}
	for i := 0; i < len(result)-1; i++ {
		if result[i].Created < result[i+1].Created {
			t.Fatalf("expected descending order at index %d: %d < %d", i, result[i].Created, result[i+1].Created)
		}
	}
}

func TestContainerList_AllFalseExcludesStopped(t *testing.T) {
	s := newListTestServer()
	addContainer(s, "c0", "running1", "alpine:3.18", true, 0)
	addContainer(s, "c1", "stopped1", "alpine:3.18", false, time.Hour)
	addContainer(s, "c2", "running2", "alpine:3.18", true, 2*time.Hour)

	req := httptest.NewRequest("GET", "/internal/v1/containers?all=false", nil)
	w := httptest.NewRecorder()
	s.handleContainerList(w, req)

	var result []*api.ContainerSummary
	json.Unmarshal(w.Body.Bytes(), &result)
	if len(result) != 2 {
		t.Fatalf("expected 2 running containers, got %d", len(result))
	}
	for _, c := range result {
		if c.State != "running" {
			t.Fatalf("expected only running containers, got state %q", c.State)
		}
	}
}
