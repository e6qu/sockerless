package core

import (
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newEmitTestServer() *BaseServer {
	store := NewStore()
	s := &BaseServer{
		Store:         store,
		Logger:        zerolog.Nop(),
		Mux:           http.NewServeMux(),
		AgentRegistry: NewAgentRegistry(),
		EventBus:      NewEventBus(),
	}
	s.InitDrivers()
	store.RestartHook = s.handleRestartPolicy
	return s
}

func TestEventEmission_ContainerCreate(t *testing.T) {
	s := newEmitTestServer()
	ch := s.EventBus.Subscribe("test")
	defer s.EventBus.Unsubscribe("test")

	// Seed a minimal image so create doesn't fail on image lookup
	s.Store.Images.Put("sha256:abc", api.Image{
		ID:       "sha256:abc",
		RepoTags: []string{"alpine:latest"},
	})

	body := `{"Image":"alpine:latest","Cmd":["echo","hi"]}`
	req := httptest.NewRequest("POST", "/internal/v1/containers?name=test1", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleContainerCreate(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	select {
	case ev := <-ch:
		if ev.Type != "container" || ev.Action != "create" {
			t.Fatalf("unexpected event: type=%q action=%q", ev.Type, ev.Action)
		}
		if ev.Actor.Attributes["name"] != "test1" {
			t.Fatalf("expected name=test1, got %q", ev.Actor.Attributes["name"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}

func TestEventEmission_NetworkRemove(t *testing.T) {
	s := newEmitTestServer()
	ch := s.EventBus.Subscribe("test")
	defer s.EventBus.Unsubscribe("test")

	s.Store.Networks.Put("net1", api.Network{
		Name:       "mynet",
		ID:         "net1",
		Driver:     "bridge",
		Containers: map[string]api.EndpointResource{},
	})

	req := httptest.NewRequest("DELETE", "/internal/v1/networks/net1", nil)
	req.SetPathValue("id", "net1")
	w := httptest.NewRecorder()
	s.handleNetworkRemove(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d", w.Code)
	}

	select {
	case ev := <-ch:
		if ev.Type != "network" || ev.Action != "destroy" {
			t.Fatalf("unexpected event: type=%q action=%q", ev.Type, ev.Action)
		}
		if ev.Actor.Attributes["name"] != "mynet" {
			t.Fatalf("expected name=mynet, got %q", ev.Actor.Attributes["name"])
		}
	case <-time.After(time.Second):
		t.Fatal("timed out waiting for event")
	}
}
