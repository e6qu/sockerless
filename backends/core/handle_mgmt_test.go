package core

import (
	"context"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

func newMgmtTestServer() *BaseServer {
	store := NewStore()
	desc := BackendDescriptor{
		ID:         "test-backend",
		Name:       "memory",
		InstanceID: "test-instance-1",
	}
	return &BaseServer{
		Store:     store,
		Logger:    zerolog.Nop(),
		Desc:      desc,
		Mux:       http.NewServeMux(),
		Registry:  NewResourceRegistry(""),
		StartedAt: time.Now().Add(-10 * time.Second),
	}
}

func TestHandleHealthz(t *testing.T) {
	s := newMgmtTestServer()
	req := httptest.NewRequest("GET", "/internal/v1/healthz", nil)
	w := httptest.NewRecorder()
	s.handleHealthz(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	if resp["component"] != "backend" {
		t.Errorf("component = %v, want backend", resp["component"])
	}
	if _, ok := resp["uptime_seconds"]; !ok {
		t.Error("missing uptime_seconds")
	}
}

func TestHandleMgmtStatus(t *testing.T) {
	s := newMgmtTestServer()
	// Set context for deterministic output
	t.Setenv("SOCKERLESS_CONTEXT", "test-ctx")

	req := httptest.NewRequest("GET", "/internal/v1/status", nil)
	w := httptest.NewRecorder()
	s.handleMgmtStatus(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	if resp["backend_type"] != "memory" {
		t.Errorf("backend_type = %v, want memory", resp["backend_type"])
	}
	if resp["instance_id"] != "test-instance-1" {
		t.Errorf("instance_id = %v, want test-instance-1", resp["instance_id"])
	}
	if resp["context"] != "test-ctx" {
		t.Errorf("context = %v, want test-ctx", resp["context"])
	}
}

func TestHandleContainerSummary(t *testing.T) {
	s := newMgmtTestServer()
	s.Store.Containers.Put("abc123", api.Container{
		ID:      "abc123",
		Name:    "/test-container",
		Created: "2025-01-01T00:00:00Z",
		Config:  api.ContainerConfig{Image: "alpine:latest"},
		State:   api.ContainerState{Status: "running", Running: true},
	})

	req := httptest.NewRequest("GET", "/internal/v1/containers/summary", nil)
	w := httptest.NewRecorder()
	s.handleContainerSummary(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var entries []ContainerSummaryEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 1 {
		t.Fatalf("len = %d, want 1", len(entries))
	}
	e := entries[0]
	if e.ID != "abc123" {
		t.Errorf("id = %q, want abc123", e.ID)
	}
	if e.Image != "alpine:latest" {
		t.Errorf("image = %q, want alpine:latest", e.Image)
	}
	if e.State != "running" {
		t.Errorf("state = %q, want running", e.State)
	}
}

func TestHandleContainerSummaryEmpty(t *testing.T) {
	s := newMgmtTestServer()

	req := httptest.NewRequest("GET", "/internal/v1/containers/summary", nil)
	w := httptest.NewRecorder()
	s.handleContainerSummary(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var entries []ContainerSummaryEntry
	if err := json.Unmarshal(w.Body.Bytes(), &entries); err != nil {
		t.Fatal(err)
	}
	if len(entries) != 0 {
		t.Fatalf("len = %d, want 0", len(entries))
	}
}

type mockHealthChecker struct {
	results []CheckResult
}

func (m *mockHealthChecker) RunChecks(ctx context.Context) []CheckResult {
	return m.results
}

func TestHandleCheck(t *testing.T) {
	s := newMgmtTestServer()
	s.HealthChecker = &mockHealthChecker{
		results: []CheckResult{
			{Name: "cloud_api", Status: "ok", Detail: "reachable"},
		},
	}

	req := httptest.NewRequest("GET", "/internal/v1/check", nil)
	w := httptest.NewRecorder()
	s.handleCheck(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Checks []CheckResult `json:"checks"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatal(err)
	}
	// Should have store + registry + cloud_api = 3 checks
	if len(resp.Checks) != 3 {
		t.Fatalf("checks = %d, want 3", len(resp.Checks))
	}
	if resp.Checks[0].Name != "store" {
		t.Errorf("checks[0].name = %q, want store", resp.Checks[0].Name)
	}
	if resp.Checks[2].Name != "cloud_api" {
		t.Errorf("checks[2].name = %q, want cloud_api", resp.Checks[2].Name)
	}
}

func TestHandleCheckNoChecker(t *testing.T) {
	s := newMgmtTestServer()

	req := httptest.NewRequest("GET", "/internal/v1/check", nil)
	w := httptest.NewRecorder()
	s.handleCheck(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp struct {
		Checks []CheckResult `json:"checks"`
	}
	json.Unmarshal(w.Body.Bytes(), &resp)
	// store + registry = 2 checks (no backend checker)
	if len(resp.Checks) != 2 {
		t.Fatalf("checks = %d, want 2", len(resp.Checks))
	}
}

func TestHandleReload(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)
	t.Setenv("SOCKERLESS_CONTEXT", "reload-ctx")

	// Create context config
	dir := filepath.Join(tmp, "contexts", "reload-ctx")
	os.MkdirAll(dir, 0o755)
	cfg := contextConfig{
		Backend: "memory",
		Env: map[string]string{
			"RELOAD_TEST_VAR": "new-value",
		},
	}
	data, _ := json.Marshal(cfg)
	os.WriteFile(filepath.Join(dir, "config.json"), data, 0o644)

	// Clear the var so reload applies it
	os.Unsetenv("RELOAD_TEST_VAR")

	s := newMgmtTestServer()
	req := httptest.NewRequest("POST", "/internal/v1/reload", nil)
	w := httptest.NewRecorder()
	s.handleReload(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want %d", w.Code, http.StatusOK)
	}

	var resp map[string]any
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["status"] != "ok" {
		t.Errorf("status = %v, want ok", resp["status"])
	}
	changed, _ := resp["changed"].(float64)
	if changed != 1 {
		t.Errorf("changed = %v, want 1", changed)
	}
	if got := os.Getenv("RELOAD_TEST_VAR"); got != "new-value" {
		t.Errorf("RELOAD_TEST_VAR = %q, want new-value", got)
	}
}
