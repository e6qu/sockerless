package core

import (
	"encoding/binary"
	"net/http"
	"net/http/httptest"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newLogStreamTestServer() *BaseServer {
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

func createRunningContainer(s *BaseServer, id string, tty bool, logData string) {
	c := api.Container{
		ID:   id,
		Name: "/" + id,
		Config: api.ContainerConfig{
			Image:  "alpine",
			Labels: make(map[string]string),
			Tty:    tty,
		},
		State: api.ContainerState{
			Status:    "running",
			Running:   true,
			StartedAt: time.Now().UTC().Format(time.RFC3339Nano),
		},
		HostConfig:      api.HostConfig{},
		NetworkSettings: api.NetworkSettings{Networks: make(map[string]*api.EndpointSettings)},
		Mounts:          []api.MountPoint{},
	}
	s.Store.Containers.Put(id, c)
	s.Store.ContainerNames.Put("/"+id, id)
	if logData != "" {
		s.Store.LogBuffers.Store(id, []byte(logData))
	}
}

func TestContainerLogsNonTTYHasMuxHeaders(t *testing.T) {
	s := newLogStreamTestServer()
	createRunningContainer(s, "c1", false, "hello world\n")

	req := httptest.NewRequest("GET", "/containers/c1/logs?stdout=1", nil)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handleContainerLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/vnd.docker.multiplexed-stream" {
		t.Errorf("expected multiplexed content type, got %q", ct)
	}

	// First 8 bytes should be mux header
	body := w.Body.Bytes()
	if len(body) < 8 {
		t.Fatalf("expected at least 8 bytes (mux header), got %d", len(body))
	}
	if body[0] != 1 { // stdout stream type
		t.Errorf("expected stream type 1, got %d", body[0])
	}
	payloadLen := binary.BigEndian.Uint32(body[4:8])
	if int(payloadLen) != len(body)-8 {
		t.Errorf("payload length mismatch: header says %d, actual %d", payloadLen, len(body)-8)
	}
}

func TestContainerLogsTTYRawStream(t *testing.T) {
	s := newLogStreamTestServer()
	createRunningContainer(s, "c2", true, "hello tty\n")

	req := httptest.NewRequest("GET", "/containers/c2/logs?stdout=1", nil)
	req.SetPathValue("id", "c2")
	w := httptest.NewRecorder()
	s.handleContainerLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	ct := w.Header().Get("Content-Type")
	if ct != "application/vnd.docker.raw-stream" {
		t.Errorf("expected raw-stream content type, got %q", ct)
	}

	// Should NOT have mux headers — raw data directly
	body := w.Body.String()
	if len(body) > 0 && body[0] == 1 {
		// Check if it looks like a mux header — if byte 1-3 are 0 and byte 4+ is size
		if len(body) > 8 && body[1] == 0 && body[2] == 0 && body[3] == 0 {
			t.Error("TTY logs should not have mux headers")
		}
	}
}

func TestContainerLogsContentTypeMultiplexed(t *testing.T) {
	s := newLogStreamTestServer()
	createRunningContainer(s, "c3", false, "data\n")

	req := httptest.NewRequest("GET", "/containers/c3/logs?stdout=1", nil)
	req.SetPathValue("id", "c3")
	w := httptest.NewRecorder()
	s.handleContainerLogs(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/vnd.docker.multiplexed-stream" {
		t.Errorf("expected multiplexed for non-TTY, got %q", ct)
	}
}

func TestContainerLogsContentTypeRaw(t *testing.T) {
	s := newLogStreamTestServer()
	createRunningContainer(s, "c4", true, "data\n")

	req := httptest.NewRequest("GET", "/containers/c4/logs?stdout=1", nil)
	req.SetPathValue("id", "c4")
	w := httptest.NewRecorder()
	s.handleContainerLogs(w, req)

	if ct := w.Header().Get("Content-Type"); ct != "application/vnd.docker.raw-stream" {
		t.Errorf("expected raw-stream for TTY, got %q", ct)
	}
}

func TestContainerLogsEmptyContainer(t *testing.T) {
	s := newLogStreamTestServer()
	createRunningContainer(s, "c5", false, "")

	req := httptest.NewRequest("GET", "/containers/c5/logs?stdout=1", nil)
	req.SetPathValue("id", "c5")
	w := httptest.NewRecorder()
	s.handleContainerLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Should return empty body (no mux headers for empty data)
	if w.Body.Len() != 0 {
		t.Errorf("expected empty body, got %d bytes", w.Body.Len())
	}
}

func TestContainerLogsFollowNonTTYSendsMux(t *testing.T) {
	// This is a basic sanity check — follow with no subscriber returns immediately
	s := newLogStreamTestServer()
	createRunningContainer(s, "c6", false, "line\n")

	req := httptest.NewRequest("GET", "/containers/c6/logs?stdout=1&follow=false", nil)
	req.SetPathValue("id", "c6")
	w := httptest.NewRecorder()
	s.handleContainerLogs(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	// Non-TTY should have mux header
	body := w.Body.Bytes()
	if len(body) < 8 {
		t.Fatalf("expected mux data, got %d bytes", len(body))
	}
	if body[0] != 1 {
		t.Errorf("expected stdout stream type (1), got %d", body[0])
	}
}
