package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newVolTestServer() *BaseServer {
	store := NewStore()
	s := &BaseServer{
		Store:  store,
		Logger: zerolog.Nop(),
		Mux:    http.NewServeMux(),
	}
	s.InitDrivers()
	return s
}

func TestVolumeList_FilterByName(t *testing.T) {
	s := newVolTestServer()

	s.Store.Volumes.Put("data", api.Volume{Name: "data", Driver: "local", Labels: map[string]string{}})
	s.Store.Volumes.Put("cache", api.Volume{Name: "cache", Driver: "local", Labels: map[string]string{}})

	req := httptest.NewRequest("GET", `/internal/v1/volumes?filters={"name":["data"]}`, nil)
	w := httptest.NewRecorder()
	s.handleVolumeList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp api.VolumeListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(resp.Volumes))
	}
	if resp.Volumes[0].Name != "data" {
		t.Fatalf("expected volume 'data', got %q", resp.Volumes[0].Name)
	}
}

func TestVolumeList_FilterByLabel(t *testing.T) {
	s := newVolTestServer()

	s.Store.Volumes.Put("v1", api.Volume{Name: "v1", Driver: "local", Labels: map[string]string{"env": "prod"}})
	s.Store.Volumes.Put("v2", api.Volume{Name: "v2", Driver: "local", Labels: map[string]string{"env": "dev"}})

	req := httptest.NewRequest("GET", `/internal/v1/volumes?filters={"label":["env=prod"]}`, nil)
	w := httptest.NewRecorder()
	s.handleVolumeList(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp api.VolumeListResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if len(resp.Volumes) != 1 {
		t.Fatalf("expected 1 volume, got %d", len(resp.Volumes))
	}
	if resp.Volumes[0].Name != "v1" {
		t.Fatalf("expected volume 'v1', got %q", resp.Volumes[0].Name)
	}
}

func TestContainerChanges_Empty(t *testing.T) {
	s := newVolTestServer()

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{ID: cID, Name: "/test"})
	s.Store.ContainerNames.Put("/test", cID)

	req := httptest.NewRequest("GET", "/internal/v1/containers/c1/changes", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerChanges(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var changes []api.ContainerChangeItem
	json.Unmarshal(w.Body.Bytes(), &changes)
	if len(changes) != 0 {
		t.Fatalf("expected 0 changes, got %d", len(changes))
	}
}
