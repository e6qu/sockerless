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

func newCommitTestServer() *BaseServer {
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

func TestCommit_Basic(t *testing.T) {
	s := newCommitTestServer()

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/test",
		Config: api.ContainerConfig{
			Image:  "alpine:3.18",
			Cmd:    []string{"/bin/sh"},
			Labels: make(map[string]string),
		},
	})
	s.Store.ContainerNames.Put("/test", cID)

	req := httptest.NewRequest("POST", "/internal/v1/commit?container=c1&repo=myimage&tag=v1", nil)
	w := httptest.NewRecorder()
	s.handleContainerCommit(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp api.ContainerCommitResponse
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp.ID == "" {
		t.Fatal("expected non-empty image ID")
	}

	// Verify image was stored
	img, ok := s.Store.ResolveImage("myimage:v1")
	if !ok {
		t.Fatal("expected image myimage:v1 to exist")
	}
	if img.Config.Image != "alpine:3.18" {
		t.Fatalf("expected image config to have alpine:3.18, got %q", img.Config.Image)
	}
}

func TestCommit_NotFound(t *testing.T) {
	s := newCommitTestServer()

	req := httptest.NewRequest("POST", "/internal/v1/commit?container=missing", nil)
	w := httptest.NewRecorder()
	s.handleContainerCommit(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404, got %d", w.Code)
	}
}

func TestCommit_ConfigOverride(t *testing.T) {
	s := newCommitTestServer()

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/test",
		Config: api.ContainerConfig{
			Image:  "alpine:3.18",
			Cmd:    []string{"/bin/sh"},
			Labels: make(map[string]string),
		},
	})
	s.Store.ContainerNames.Put("/test", cID)

	body := `{"Cmd":["/bin/bash","-c","echo hello"]}`
	req := httptest.NewRequest("POST", "/internal/v1/commit?container=c1&repo=custom&tag=latest", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleContainerCommit(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var resp api.ContainerCommitResponse
	json.Unmarshal(w.Body.Bytes(), &resp)

	img, ok := s.Store.ResolveImage("custom:latest")
	if !ok {
		t.Fatal("expected image custom:latest to exist")
	}
	if len(img.Config.Cmd) != 3 || img.Config.Cmd[0] != "/bin/bash" {
		t.Fatalf("expected overridden CMD [/bin/bash -c echo hello], got %v", img.Config.Cmd)
	}
}
