package core

import (
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newExportTestServer() *BaseServer {
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

func TestContainerExport_NotFound(t *testing.T) {
	s := newExportTestServer()

	req := httptest.NewRequest("GET", "/internal/v1/containers/missing/export", nil)
	req.SetPathValue("id", "missing")
	w := httptest.NewRecorder()
	s.handleContainerExport(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestContainerExport_SyntheticEmpty(t *testing.T) {
	s := newExportTestServer()

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/test",
		State: api.ContainerState{
			Status:  "running",
			Running: true,
		},
	})
	s.Store.ContainerNames.Put("/test", cID)

	req := httptest.NewRequest("GET", "/internal/v1/containers/c1/export", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerExport(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if ct := w.Header().Get("Content-Type"); ct != "application/x-tar" {
		t.Fatalf("expected Content-Type application/x-tar, got %q", ct)
	}
	// Empty tar is at least 1024 bytes (two 512-byte zero blocks)
	if w.Body.Len() == 0 {
		t.Fatal("expected non-empty body for empty tar archive")
	}
}
