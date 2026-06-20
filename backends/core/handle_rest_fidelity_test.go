package core

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newFidelityTestServer() *BaseServer {
	store := NewStore()
	s := &BaseServer{
		Store:          store,
		Logger:         zerolog.Nop(),
		Mux:            http.NewServeMux(),
		Desc:           BackendDescriptor{Driver: "test"},
		Registry:       NewResourceRegistry(""),
		EventBus:       NewEventBus(),
		PendingCreates: NewStateStore[api.Container](),
	}
	s.InitDrivers()
	s.self = s
	s.registerRoutes()
	s.InitDefaultNetwork()
	return s
}

// HEAD /containers/{id}/archive for a missing path returns 404 (not 500)
// so docker cp's existence probe gets "create" semantics.
func TestHeadArchive_MissingPath_Returns404(t *testing.T) {
	s := newFidelityTestServer()
	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{ID: cID, Name: "/test"})
	s.Store.ContainerNames.Put("/test", cID)

	req := httptest.NewRequest("HEAD", "/internal/v1/containers/c1/archive?path=/no/such/path", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing path, got %d: %s", w.Code, w.Body.String())
	}
}

// GET /containers/{id}/logs with neither stdout nor stderr selected
// returns 400 instead of silently streaming stdout.
func TestContainerLogs_NoStream_Returns400(t *testing.T) {
	s := newFidelityTestServer()
	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{ID: cID, Name: "/test"})
	s.Store.ContainerNames.Put("/test", cID)

	req := httptest.NewRequest("GET", "/internal/v1/containers/c1/logs", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 when no stream selected, got %d: %s", w.Code, w.Body.String())
	}
}

// GET /containers/{id}/logs?stdout=1 is accepted (does not 400).
func TestContainerLogs_StdoutSelected_NotBadRequest(t *testing.T) {
	s := newFidelityTestServer()
	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{ID: cID, Name: "/test"})
	s.Store.ContainerNames.Put("/test", cID)

	req := httptest.NewRequest("GET", "/internal/v1/containers/c1/logs?stdout=1", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code == http.StatusBadRequest {
		t.Fatalf("stdout=1 should not 400: %s", w.Body.String())
	}
}

// POST /containers/{id}/wait?condition=<bad> returns 400.
func TestContainerWait_InvalidCondition_Returns400(t *testing.T) {
	s := newFidelityTestServer()
	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{ID: cID, Name: "/test"})
	s.Store.ContainerNames.Put("/test", cID)

	req := httptest.NewRequest("POST", "/internal/v1/containers/c1/wait?condition=bogus", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400 for invalid condition, got %d: %s", w.Code, w.Body.String())
	}
}

// Each documented condition value is accepted (not 400).
func TestContainerWait_ValidConditions_NotBadRequest(t *testing.T) {
	for _, cond := range []string{"", "not-running", "next-exit", "removed"} {
		s := newFidelityTestServer()
		cID := "c1"
		s.Store.Containers.Put(cID, api.Container{
			ID: cID, Name: "/test",
			State: api.ContainerState{Status: "exited"},
		})
		s.Store.ContainerNames.Put("/test", cID)

		url := "/internal/v1/containers/c1/wait"
		if cond != "" {
			url += "?condition=" + cond
		}
		req := httptest.NewRequest("POST", url, nil)
		w := httptest.NewRecorder()
		s.Mux.ServeHTTP(w, req)

		if w.Code == http.StatusBadRequest {
			t.Fatalf("condition %q should not 400: %s", cond, w.Body.String())
		}
	}
}

// POST /containers/{id}/attach for a missing container returns 404 before
// hijacking, rather than 101 + empty stream.
func TestContainerAttach_MissingContainer_Returns404(t *testing.T) {
	s := newFidelityTestServer()

	req := httptest.NewRequest("POST", "/internal/v1/containers/nope/attach?stream=1&stdout=1", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusNotFound {
		t.Fatalf("expected 404 for missing container, got %d: %s", w.Code, w.Body.String())
	}
}

// GET /libpod/exec/{id}/json resolves the exec-inspect handler (not 404).
func TestLibpodExecInspect_Routed(t *testing.T) {
	s := newFidelityTestServer()
	execID := "exec-1"
	s.Store.Execs.Put(execID, api.ExecInstance{ID: execID, ContainerID: "c1", ExitCode: 7})

	req := httptest.NewRequest("GET", "/libpod/exec/exec-1/json", nil)
	w := httptest.NewRecorder()
	s.Mux.ServeHTTP(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 from libpod exec inspect, got %d: %s", w.Code, w.Body.String())
	}
	var inst api.ExecInstance
	if err := json.Unmarshal(w.Body.Bytes(), &inst); err != nil {
		t.Fatalf("invalid exec JSON: %v", err)
	}
	if inst.ExitCode != 7 {
		t.Errorf("expected exit code 7, got %d", inst.ExitCode)
	}
}

// writePodActionResponse emits 200 + a report with populated Errs on a
// per-container failure (podman PodStopReport shape), not a 409.
func TestWritePodActionResponse_PartialFailure_Returns200WithErrs(t *testing.T) {
	w := httptest.NewRecorder()
	writePodActionResponse(w, &api.PodActionResponse{
		ID:   "pod-1",
		Errs: []string{"container c1: no such process"},
	})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200 on partial failure, got %d", w.Code)
	}
	var report struct {
		ID   string   `json:"Id"`
		Errs []string `json:"Errs"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &report); err != nil {
		t.Fatalf("invalid report JSON: %v", err)
	}
	if report.ID != "pod-1" {
		t.Errorf("expected Id pod-1, got %q", report.ID)
	}
	if len(report.Errs) != 1 || report.Errs[0] != "container c1: no such process" {
		t.Errorf("expected populated Errs, got %v", report.Errs)
	}
}

// writePodActionResponse success path always emits a non-null empty Errs.
func TestWritePodActionResponse_Success_EmptyErrs(t *testing.T) {
	w := httptest.NewRecorder()
	writePodActionResponse(w, &api.PodActionResponse{ID: "pod-1"})

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}
	if !bytes.Contains(w.Body.Bytes(), []byte(`"Errs":[]`)) {
		t.Errorf("expected non-null empty Errs array, got %s", w.Body.String())
	}
}
