package main

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"testing"
)

// CRUD endpoints — surgical add/remove/edit shortcuts over PUT
// /api/v1/topology. Verify happy paths + the right status codes for
// validation / conflict / not-found.

func TestAPIProjectAddRemove(t *testing.T) {
	_, mux := setupTopologyServer(t)

	// Add ok.
	body, _ := json.Marshal(ProjectConfig{Name: "p1"})
	req := httptest.NewRequest("POST", "/api/v1/topology/projects", bytes.NewReader(body))
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusCreated {
		t.Fatalf("add: %d %s", w.Code, w.Body.String())
	}

	// Add duplicate → 409.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/topology/projects", bytes.NewReader(body)))
	if w.Code != http.StatusConflict {
		t.Errorf("dup add: %d %s", w.Code, w.Body.String())
	}

	// Remove ok.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/v1/topology/projects/p1", nil))
	if w.Code != http.StatusOK {
		t.Errorf("remove: %d %s", w.Code, w.Body.String())
	}

	// Remove missing → 404.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/v1/topology/projects/p1", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("remove missing: %d %s", w.Code, w.Body.String())
	}
}

func TestAPIInstanceCRUD(t *testing.T) {
	mgr, mux := setupTopologyServer(t)
	if err := mgr.AddProject(ProjectConfig{Name: "p"}); err != nil {
		t.Fatalf("seed project: %v", err)
	}

	// Add instance.
	body, _ := json.Marshal(Instance{Name: "s", Kind: InstanceKindBleephub, Port: 5500})
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/topology/projects/p/instances", bytes.NewReader(body)))
	if w.Code != http.StatusCreated {
		t.Fatalf("add inst: %d %s", w.Code, w.Body.String())
	}

	// Add to unknown project → 404.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("POST", "/api/v1/topology/projects/missing/instances", bytes.NewReader(body)))
	if w.Code != http.StatusNotFound {
		t.Errorf("add inst missing project: %d %s", w.Code, w.Body.String())
	}

	// Update with mismatched name → 400.
	bodyMismatch, _ := json.Marshal(Instance{Name: "different", Kind: InstanceKindBleephub, Port: 5501})
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("PUT", "/api/v1/topology/projects/p/instances/s", bytes.NewReader(bodyMismatch)))
	if w.Code != http.StatusBadRequest {
		t.Errorf("update name mismatch: %d %s", w.Code, w.Body.String())
	}

	// Update ok (change port).
	bodyEdit, _ := json.Marshal(Instance{Name: "s", Kind: InstanceKindBleephub, Port: 5599})
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("PUT", "/api/v1/topology/projects/p/instances/s", bytes.NewReader(bodyEdit)))
	if w.Code != http.StatusOK {
		t.Errorf("update ok: %d %s", w.Code, w.Body.String())
	}

	// Confirm via GET.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/s", nil))
	var ref InstanceRef
	_ = json.Unmarshal(w.Body.Bytes(), &ref)
	if ref.Instance.Port != 5599 {
		t.Errorf("update didn't persist: port=%d", ref.Instance.Port)
	}

	// Remove ok.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/v1/topology/projects/p/instances/s", nil))
	if w.Code != http.StatusOK {
		t.Errorf("remove inst: %d %s", w.Code, w.Body.String())
	}

	// Remove again → 404.
	w = httptest.NewRecorder()
	mux.ServeHTTP(w, httptest.NewRequest("DELETE", "/api/v1/topology/projects/p/instances/s", nil))
	if w.Code != http.StatusNotFound {
		t.Errorf("remove missing: %d %s", w.Code, w.Body.String())
	}
}

func TestAPIInstanceStatusUnknown(t *testing.T) {
	mgr, mux := setupTopologyServer(t)
	_ = mgr.AddProject(ProjectConfig{Name: "p"})
	_ = mgr.AddInstance("p", Instance{Name: "absent", Kind: InstanceKindBleephub, Port: 56999})

	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/absent/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status: %d %s", w.Code, w.Body.String())
	}
	var s InstanceStatus
	_ = json.Unmarshal(w.Body.Bytes(), &s)
	// No PID file exists for this fake instance → not running, health unknown.
	if s.Running {
		t.Errorf("non-running instance reported running: %+v", s)
	}
	if s.Health != "unknown" {
		t.Errorf("non-running instance health = %q, want unknown", s.Health)
	}
	if s.Project != "p" || s.Name != "absent" {
		t.Errorf("project / name not echoed: %+v", s)
	}
}

func TestAPIInstanceStatus404(t *testing.T) {
	_, mux := setupTopologyServer(t)
	req := httptest.NewRequest("GET", "/api/v1/topology/projects/missing/instances/x/status", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status missing: %d", w.Code)
	}
}
