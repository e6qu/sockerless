package core

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newUpdateTestServer() *BaseServer {
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

func TestContainerUpdate_RestartPolicy(t *testing.T) {
	s := newUpdateTestServer()

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/test",
		HostConfig: api.HostConfig{
			RestartPolicy: api.RestartPolicy{Name: "no"},
		},
	})
	s.Store.ContainerNames.Put("/test", cID)

	body := `{"RestartPolicy":{"Name":"always","MaximumRetryCount":0}}`
	req := httptest.NewRequest("POST", "/internal/v1/containers/c1/update", strings.NewReader(body))
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp api.ContainerUpdateResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.Warnings == nil {
		t.Fatal("expected non-nil warnings slice")
	}

	c, _ := s.Store.Containers.Get(cID)
	if c.HostConfig.RestartPolicy.Name != "always" {
		t.Fatalf("expected restart policy 'always', got %q", c.HostConfig.RestartPolicy.Name)
	}
}

func TestContainerUpdate_NotFound(t *testing.T) {
	s := newUpdateTestServer()

	body := `{"RestartPolicy":{"Name":"always"}}`
	req := httptest.NewRequest("POST", "/internal/v1/containers/missing/update", strings.NewReader(body))
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()
	s.handleContainerUpdate(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestContainerUpdate_EmptyBody(t *testing.T) {
	s := newUpdateTestServer()

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/test",
		HostConfig: api.HostConfig{
			RestartPolicy: api.RestartPolicy{Name: "no"},
		},
	})
	s.Store.ContainerNames.Put("/test", cID)

	req := httptest.NewRequest("POST", "/internal/v1/containers/c1/update", strings.NewReader(""))
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerUpdate(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
}
