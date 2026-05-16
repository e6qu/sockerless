package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func setupDiagnosticsServer(t *testing.T) (*TopologyManager, *http.ServeMux, string) {
	t.Helper()
	tmp := t.TempDir()
	prev, _ := os.Getwd()
	if err := os.Chdir(tmp); err != nil {
		t.Fatalf("chdir: %v", err)
	}
	t.Cleanup(func() { _ = os.Chdir(prev) })

	mgr := NewTopologyManager(filepath.Join(tmp, "sockerless.yaml"))
	if err := mgr.Load(); err != nil {
		t.Fatalf("load: %v", err)
	}
	if err := mgr.Replace(Topology{Projects: []ProjectConfig{{
		Name: "p",
		Instances: []Instance{{
			Name:  "sim-aws",
			Kind:  InstanceKindSim,
			Cloud: CloudAWS,
			Port:  4500,
		}},
	}}}); err != nil {
		t.Fatalf("replace: %v", err)
	}

	if err := os.MkdirAll(filepath.Join(tmp, ".stack-pids"), 0o755); err != nil {
		t.Fatalf("mkdir: %v", err)
	}

	mux := http.NewServeMux()
	registerTopologyAPI(mux, mgr, nil)
	return mgr, mux, tmp
}

func TestDiagnosticsBasic(t *testing.T) {
	_, mux, dir := setupDiagnosticsServer(t)

	logPath := filepath.Join(dir, ".stack-pids", "sim-aws.log")
	if err := os.WriteFile(logPath, []byte("line1\nline2\nline3\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/sim-aws/diagnostics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d; body=%s", w.Code, w.Body.String())
	}

	var got diagnosticResponse
	if err := json.Unmarshal(w.Body.Bytes(), &got); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if got.Status.Name != "sim-aws" || got.Status.Project != "p" {
		t.Errorf("status identity wrong: %+v", got.Status)
	}
	if len(got.LogLines) != 3 {
		t.Errorf("expected 3 log lines, got %d", len(got.LogLines))
	}
}

func TestDiagnosticsNotFound(t *testing.T) {
	_, mux, _ := setupDiagnosticsServer(t)
	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/missing/diagnostics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestDiagnosticsMissingLog(t *testing.T) {
	_, mux, _ := setupDiagnosticsServer(t)
	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/sim-aws/diagnostics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("status = %d", w.Code)
	}
	var got diagnosticResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	// Missing log file is treated as "no logs yet" — empty array, status ok.
	if len(got.LogLines) != 0 {
		t.Errorf("expected empty log lines for missing file, got %v", got.LogLines)
	}
}

func TestDiagnosticsLinesParam(t *testing.T) {
	_, mux, dir := setupDiagnosticsServer(t)
	logPath := filepath.Join(dir, ".stack-pids", "sim-aws.log")
	if err := os.WriteFile(logPath, []byte("a\nb\nc\nd\ne\nf\n"), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/sim-aws/diagnostics?lines=2", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var got diagnosticResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.LogLines) != 2 || got.LogLines[0] != "e" || got.LogLines[1] != "f" {
		t.Errorf("lines=2: got %v", got.LogLines)
	}
}

func TestDiagnosticsLinesCapAt1000(t *testing.T) {
	_, mux, dir := setupDiagnosticsServer(t)
	logPath := filepath.Join(dir, ".stack-pids", "sim-aws.log")
	// Write 1500 lines.
	data := ""
	for i := 0; i < 1500; i++ {
		data += "line\n"
	}
	if err := os.WriteFile(logPath, []byte(data), 0o644); err != nil {
		t.Fatalf("write log: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/sim-aws/diagnostics?lines=99999", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var got diagnosticResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if len(got.LogLines) != 1000 {
		t.Errorf("lines=99999 should clamp to 1000, got %d", len(got.LogLines))
	}
}

func TestDiagnosticsExitSurfaced(t *testing.T) {
	_, mux, dir := setupDiagnosticsServer(t)
	exitPath := filepath.Join(dir, ".stack-pids", "sim-aws.exit")
	if err := os.WriteFile(exitPath, []byte("137 2026-05-10T12:00:00Z\n"), 0o644); err != nil {
		t.Fatalf("write exit: %v", err)
	}

	req := httptest.NewRequest("GET", "/api/v1/topology/projects/p/instances/sim-aws/diagnostics", nil)
	w := httptest.NewRecorder()
	mux.ServeHTTP(w, req)
	var got diagnosticResponse
	_ = json.Unmarshal(w.Body.Bytes(), &got)
	if got.Status.Exit == nil || got.Status.Exit.Code != 137 {
		t.Errorf("exit not surfaced: %+v", got.Status.Exit)
	}
}
