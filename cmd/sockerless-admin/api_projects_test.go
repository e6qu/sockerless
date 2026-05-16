package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"strings"
	"testing"
)

func newTestProjectManager(t *testing.T) *ProjectManager {
	t.Helper()
	pm := NewProcessManager(nil)
	return NewProjectManager(pm, nil, t.TempDir())
}

func TestHandleProjectListEmpty(t *testing.T) {
	projMgr := newTestProjectManager(t)
	handler := handleProjectList(projMgr)

	req := httptest.NewRequest("GET", "/api/v1/projects", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var projects []ProjectStatus
	json.Unmarshal(w.Body.Bytes(), &projects)
	if len(projects) != 0 {
		t.Errorf("expected 0 projects, got %d", len(projects))
	}
}

func TestHandleProjectCreate(t *testing.T) {
	projMgr := newTestProjectManager(t)
	handler := handleProjectCreate(projMgr)

	body := `{"name":"test-aws","instances":[
		{"name":"sim","kind":"sim","cloud":"aws","port":0},
		{"name":"backend","kind":"backend","cloud":"aws","backend":"ecs","port":0,"sim":"sim"}
	]}`
	req := httptest.NewRequest("POST", "/api/v1/projects", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}

	var status ProjectStatus
	json.Unmarshal(w.Body.Bytes(), &status)
	if status.Name != "test-aws" {
		t.Errorf("name = %s, want test-aws", status.Name)
	}
	simInst := projectInstance(status, "sim")
	if simInst.Cloud != CloudAWS {
		t.Errorf("sim cloud = %s, want aws", simInst.Cloud)
	}
	if simInst.Port == 0 {
		t.Error("expected auto-assigned sim port")
	}
}

func TestHandleProjectCreateInvalidCloud(t *testing.T) {
	projMgr := newTestProjectManager(t)
	handler := handleProjectCreate(projMgr)

	body := `{"name":"bad","instances":[
		{"name":"sim","kind":"sim","cloud":"invalid","port":0},
		{"name":"backend","kind":"backend","cloud":"invalid","backend":"ecs","port":0,"sim":"sim"}
	]}`
	req := httptest.NewRequest("POST", "/api/v1/projects", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectCreateInvalidBackend(t *testing.T) {
	projMgr := newTestProjectManager(t)
	handler := handleProjectCreate(projMgr)

	body := `{"name":"bad","instances":[
		{"name":"sim","kind":"sim","cloud":"aws","port":0},
		{"name":"backend","kind":"backend","cloud":"aws","backend":"cloudrun","port":0,"sim":"sim"}
	]}`
	req := httptest.NewRequest("POST", "/api/v1/projects", strings.NewReader(body))
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusBadRequest {
		t.Errorf("expected 400, got %d", w.Code)
	}
}

func TestHandleProjectGet(t *testing.T) {
	projMgr := newTestProjectManager(t)
	_ = projMgr.Create(testProject("test-gcp", CloudGCP, BackendCloudRun, 0, 0))

	handler := handleProjectGet(projMgr)
	req := httptest.NewRequest("GET", "/api/v1/projects/test-gcp", nil)
	req.SetPathValue("name", "test-gcp")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var status ProjectStatus
	json.Unmarshal(w.Body.Bytes(), &status)
	if status.Name != "test-gcp" {
		t.Errorf("name = %s, want test-gcp", status.Name)
	}
}

func TestHandleProjectGetNotFound(t *testing.T) {
	projMgr := newTestProjectManager(t)

	handler := handleProjectGet(projMgr)
	req := httptest.NewRequest("GET", "/api/v1/projects/nope", nil)
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleProjectDelete(t *testing.T) {
	projMgr := newTestProjectManager(t)
	_ = projMgr.Create(testProject("del-me", CloudAzure, BackendACA, 0, 0))

	handler := handleProjectDelete(projMgr)
	req := httptest.NewRequest("DELETE", "/api/v1/projects/del-me", nil)
	req.SetPathValue("name", "del-me")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d: %s", w.Code, w.Body.String())
	}

	// Verify deleted
	_, ok := projMgr.Get("del-me")
	if ok {
		t.Error("project should be deleted")
	}
}

func TestHandleProjectConnection(t *testing.T) {
	projMgr := newTestProjectManager(t)
	_ = projMgr.Create(testProject("conn", CloudAWS, BackendLambda, 0, 0))

	handler := handleProjectConnection(projMgr)
	req := httptest.NewRequest("GET", "/api/v1/projects/conn/connection", nil)
	req.SetPathValue("name", "conn")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var conn ProjectConnection
	json.Unmarshal(w.Body.Bytes(), &conn)
	if conn.DockerHost == "" {
		t.Error("expected non-empty docker_host")
	}
	if conn.EnvExport == "" {
		t.Error("expected non-empty env_export")
	}
	if conn.PodmanConnection == "" {
		t.Error("expected non-empty podman_connection")
	}
}

func TestHandleProjectConnectionNotFound(t *testing.T) {
	projMgr := newTestProjectManager(t)

	handler := handleProjectConnection(projMgr)
	req := httptest.NewRequest("GET", "/api/v1/projects/nope/connection", nil)
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
