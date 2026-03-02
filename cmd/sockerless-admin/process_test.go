package main

import (
	"runtime"
	"testing"
	"time"
)

func TestRingBufferWrite(t *testing.T) {
	rb := NewRingBuffer(5)
	rb.Write([]byte("line1\nline2\nline3\n"))

	lines := rb.Lines(10)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	if lines[0] != "line1" {
		t.Errorf("lines[0] = %q, want line1", lines[0])
	}
	if lines[2] != "line3" {
		t.Errorf("lines[2] = %q, want line3", lines[2])
	}
}

func TestRingBufferOverflow(t *testing.T) {
	rb := NewRingBuffer(3)
	rb.Write([]byte("a\nb\nc\nd\ne\n"))

	lines := rb.Lines(3)
	if len(lines) != 3 {
		t.Fatalf("expected 3 lines, got %d", len(lines))
	}
	// Should have the last 3 lines
	if lines[0] != "c" {
		t.Errorf("lines[0] = %q, want c", lines[0])
	}
	if lines[1] != "d" {
		t.Errorf("lines[1] = %q, want d", lines[1])
	}
	if lines[2] != "e" {
		t.Errorf("lines[2] = %q, want e", lines[2])
	}
}

func TestRingBufferEmpty(t *testing.T) {
	rb := NewRingBuffer(5)
	lines := rb.Lines(10)
	if lines != nil {
		t.Errorf("expected nil lines, got %v", lines)
	}
}

func TestRingBufferRequestFewer(t *testing.T) {
	rb := NewRingBuffer(10)
	rb.Write([]byte("a\nb\nc\nd\n"))

	lines := rb.Lines(2)
	if len(lines) != 2 {
		t.Fatalf("expected 2 lines, got %d", len(lines))
	}
	if lines[0] != "c" {
		t.Errorf("lines[0] = %q, want c", lines[0])
	}
	if lines[1] != "d" {
		t.Errorf("lines[1] = %q, want d", lines[1])
	}
}

func TestProcessManagerStartStop(t *testing.T) {
	if runtime.GOOS == "windows" {
		t.Skip("sleep binary not available on windows")
	}

	reg := NewRegistry()
	pm := NewProcessManager(reg)

	pm.AddProcess(ProcessConfig{
		Name:   "test-proc",
		Binary: "sleep",
		Args:   []string{"60"},
		Addr:   ":19999",
		Type:   "backend",
	})

	// Start
	if err := pm.Start("test-proc"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Give the process a moment to start
	time.Sleep(100 * time.Millisecond)

	// List should show running
	list := pm.List()
	if len(list) != 1 {
		t.Fatalf("expected 1 process, got %d", len(list))
	}
	if list[0].Status != "running" {
		t.Errorf("expected status=running, got %s", list[0].Status)
	}
	if list[0].PID == 0 {
		t.Error("expected non-zero PID")
	}

	// Should be auto-registered in registry
	comp, ok := reg.Get("test-proc")
	if !ok {
		t.Fatal("expected component to be registered")
	}
	if comp.Type != "backend" {
		t.Errorf("expected type=backend, got %s", comp.Type)
	}

	// Stop
	if err := pm.Stop("test-proc"); err != nil {
		t.Fatalf("Stop failed: %v", err)
	}

	info, ok := pm.Get("test-proc")
	if !ok {
		t.Fatal("expected process to still exist after stop")
	}
	if info.Status != "stopped" {
		t.Errorf("expected status=stopped, got %s", info.Status)
	}
}

func TestProcessManagerStartNotFound(t *testing.T) {
	pm := NewProcessManager(nil)
	err := pm.Start("nonexistent")
	if err == nil {
		t.Error("expected error for nonexistent process")
	}
}

func TestProcessManagerStopNotRunning(t *testing.T) {
	pm := NewProcessManager(nil)
	pm.AddProcess(ProcessConfig{
		Name:   "idle",
		Binary: "sleep",
		Args:   []string{"1"},
		Type:   "backend",
	})

	err := pm.Stop("idle")
	if err == nil {
		t.Error("expected error stopping non-running process")
	}
}

func TestProcessManagerGetLogs(t *testing.T) {
	pm := NewProcessManager(nil)
	pm.AddProcess(ProcessConfig{
		Name:   "echo-test",
		Binary: "echo",
		Args:   []string{"hello world"},
		Type:   "backend",
	})

	if err := pm.Start("echo-test"); err != nil {
		t.Fatalf("Start failed: %v", err)
	}

	// Wait for process to finish (echo exits immediately)
	time.Sleep(200 * time.Millisecond)

	logs, err := pm.GetLogs("echo-test", 10)
	if err != nil {
		t.Fatalf("GetLogs failed: %v", err)
	}
	if len(logs) == 0 {
		t.Error("expected at least one log line")
	}
}

func TestProcessManagerList(t *testing.T) {
	pm := NewProcessManager(nil)
	pm.AddProcess(ProcessConfig{Name: "a", Binary: "sleep", Args: []string{"1"}, Type: "backend"})
	pm.AddProcess(ProcessConfig{Name: "b", Binary: "sleep", Args: []string{"1"}, Type: "simulator"})

	list := pm.List()
	if len(list) != 2 {
		t.Fatalf("expected 2 processes, got %d", len(list))
	}

	names := map[string]bool{}
	for _, p := range list {
		names[p.Name] = true
		if p.Status != "stopped" {
			t.Errorf("expected initial status=stopped, got %s for %s", p.Status, p.Name)
		}
	}
	if !names["a"] || !names["b"] {
		t.Errorf("expected both processes in list, got %v", names)
	}
}
