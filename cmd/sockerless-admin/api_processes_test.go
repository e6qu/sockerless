package main

import (
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"runtime"
	"testing"
	"time"
)

func TestHandleProcessList(t *testing.T) {
	pm := NewProcessManager(nil)
	pm.AddProcess(ProcessConfig{Name: "a", Binary: "sleep", Args: []string{"1"}, Type: "backend"})
	pm.AddProcess(ProcessConfig{Name: "b", Binary: "echo", Args: []string{"hi"}, Type: "simulator"})

	handler := handleProcessList(pm)
	req := httptest.NewRequest("GET", "/api/v1/processes", nil)
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var procs []ProcessInfo
	if err := json.Unmarshal(w.Body.Bytes(), &procs); err != nil {
		t.Fatalf("failed to parse response: %v", err)
	}
	if len(procs) != 2 {
		t.Errorf("expected 2 processes, got %d", len(procs))
	}
}

func TestHandleProcessStartStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep binary not available on windows")
	}

	reg := NewRegistry()
	pm := NewProcessManager(reg)
	pm.AddProcess(ProcessConfig{Name: "test", Binary: "sleep", Args: []string{"60"}, Addr: ":19998", Type: "backend"})

	// Start
	handler := handleProcessStart(pm)
	req := httptest.NewRequest("POST", "/api/v1/processes/test/start", nil)
	req.SetPathValue("name", "test")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("start: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	var info ProcessInfo
	json.Unmarshal(w.Body.Bytes(), &info)
	if info.Status != "running" {
		t.Errorf("expected status=running, got %s", info.Status)
	}

	time.Sleep(100 * time.Millisecond)

	// Stop
	handler = handleProcessStop(pm)
	req = httptest.NewRequest("POST", "/api/v1/processes/test/stop", nil)
	req.SetPathValue("name", "test")
	w = httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("stop: expected 200, got %d: %s", w.Code, w.Body.String())
	}

	json.Unmarshal(w.Body.Bytes(), &info)
	if info.Status != "stopped" {
		t.Errorf("expected status=stopped, got %s", info.Status)
	}
}

func TestHandleProcessStartNotFound(t *testing.T) {
	pm := NewProcessManager(nil)

	handler := handleProcessStart(pm)
	req := httptest.NewRequest("POST", "/api/v1/processes/nope/start", nil)
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}

func TestHandleProcessLogs(t *testing.T) {
	pm := NewProcessManager(nil)
	pm.AddProcess(ProcessConfig{Name: "log-test", Binary: "echo", Args: []string{"hello"}, Type: "backend"})

	// Start and wait for output
	pm.Start("log-test")
	time.Sleep(200 * time.Millisecond)

	handler := handleProcessLogs(pm)
	req := httptest.NewRequest("GET", "/api/v1/processes/log-test/logs?lines=50", nil)
	req.SetPathValue("name", "log-test")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("expected 200, got %d", w.Code)
	}

	var logs []string
	json.Unmarshal(w.Body.Bytes(), &logs)
	if len(logs) == 0 {
		t.Error("expected at least one log line")
	}
}

func TestProcessStopThenStartRace(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep binary not available on windows")
	}

	pm := NewProcessManager(nil)
	pm.AddProcess(ProcessConfig{Name: "race-test", Binary: "sleep", Args: []string{"60"}, Type: "backend"})

	// Start initial process
	if err := pm.Start("race-test"); err != nil {
		t.Fatalf("initial start failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Get initial PID
	info, _ := pm.Get("race-test")
	if info.PID == 0 {
		t.Fatal("expected non-zero PID after start")
	}

	// Stop in background
	stopDone := make(chan error, 1)
	go func() {
		stopDone <- pm.Stop("race-test")
	}()

	// Wait for stop to complete
	if err := <-stopDone; err != nil {
		t.Fatalf("stop failed: %v", err)
	}

	// Re-start with a new process
	if err := pm.Start("race-test"); err != nil {
		t.Fatalf("re-start failed: %v", err)
	}
	time.Sleep(100 * time.Millisecond)

	// Verify new process is still running with valid PID
	info, _ = pm.Get("race-test")
	if info.Status != "running" {
		t.Errorf("expected status=running after re-start, got %s", info.Status)
	}
	if info.PID == 0 {
		t.Error("expected non-zero PID after re-start, got 0 (Stop clobbered new process)")
	}

	// Cleanup
	_ = pm.Stop("race-test")
}

func TestHandleProcessLogsNotFound(t *testing.T) {
	pm := NewProcessManager(nil)

	handler := handleProcessLogs(pm)
	req := httptest.NewRequest("GET", "/api/v1/processes/nope/logs", nil)
	req.SetPathValue("name", "nope")
	w := httptest.NewRecorder()
	handler(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("expected 404, got %d", w.Code)
	}
}
