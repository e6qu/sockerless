package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"testing"
)

func TestHandleCleanupScan(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)

	// Create a stale PID file
	runDir := filepath.Join(tmp, "run")
	os.MkdirAll(runDir, 0o755)
	os.WriteFile(filepath.Join(runDir, "dead.pid"), []byte("99999999"), 0o644)

	reg := NewRegistry()
	client := &http.Client{}

	handler := handleCleanupScan(reg, client)
	req := httptest.NewRequest("GET", "/api/v1/cleanup/scan", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result CleanupScanResult
	if err := json.Unmarshal(w.Body.Bytes(), &result); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}

	if result.ScannedAt == "" {
		t.Error("expected scanned_at to be set")
	}

	// Should find at least the orphaned PID file
	found := false
	for _, item := range result.Items {
		if item.Category == "process" && item.Name == "dead.pid" {
			found = true
		}
	}
	if !found {
		t.Error("expected orphaned PID file in scan results")
	}
}

func TestHandleCleanProcesses(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)

	runDir := filepath.Join(tmp, "run")
	os.MkdirAll(runDir, 0o755)
	os.WriteFile(filepath.Join(runDir, "dead.pid"), []byte("99999999"), 0o644)

	handler := handleCleanProcesses()
	req := httptest.NewRequest("POST", "/api/v1/cleanup/processes", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var resp map[string]int
	json.Unmarshal(w.Body.Bytes(), &resp)
	if resp["cleaned"] != 1 {
		t.Errorf("expected cleaned=1, got %d", resp["cleaned"])
	}
}

func TestHandleCleanupScanNoOrphans(t *testing.T) {
	t.Setenv("SOCKERLESS_HOME", t.TempDir())

	reg := NewRegistry()
	client := &http.Client{}

	handler := handleCleanupScan(reg, client)
	req := httptest.NewRequest("GET", "/api/v1/cleanup/scan", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var result CleanupScanResult
	json.Unmarshal(w.Body.Bytes(), &result)
	// Should not contain any process orphans (tmp files may exist on system)
	for _, item := range result.Items {
		if item.Category == "process" {
			t.Errorf("unexpected orphaned process: %s", item.Name)
		}
	}
	if result.ScannedAt == "" {
		t.Error("expected scanned_at to be set")
	}
}
