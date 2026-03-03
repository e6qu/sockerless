package main

import (
	"os"
	"path/filepath"
	"testing"
	"time"
)

func TestScanOrphanedProcesses(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)

	// Create run directory with a stale PID file
	runDir := filepath.Join(tmp, "run")
	if err := os.MkdirAll(runDir, 0o755); err != nil {
		t.Fatal(err)
	}

	// Use PID 99999999 which almost certainly doesn't exist
	if err := os.WriteFile(filepath.Join(runDir, "stale.pid"), []byte("99999999"), 0o644); err != nil {
		t.Fatal(err)
	}

	items := ScanOrphanedProcesses()
	if len(items) != 1 {
		t.Fatalf("expected 1 orphaned process, got %d", len(items))
	}
	if items[0].Category != "process" {
		t.Errorf("expected category=process, got %s", items[0].Category)
	}
	if items[0].Name != "stale.pid" {
		t.Errorf("expected name=stale.pid, got %s", items[0].Name)
	}
}

func TestScanOrphanedProcessesNoRunDir(t *testing.T) {
	t.Setenv("SOCKERLESS_HOME", t.TempDir())
	items := ScanOrphanedProcesses()
	if items != nil {
		t.Errorf("expected nil, got %v", items)
	}
}

func TestScanStaleTmpFiles(t *testing.T) {
	// Create a stale temp dir in /tmp matching the sockerless-* pattern
	dir := filepath.Join("/tmp", "sockerless-test-cleanup-stale")
	if err := os.MkdirAll(dir, 0o755); err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(dir)

	// Write a file inside so it has size
	os.WriteFile(filepath.Join(dir, "data.txt"), []byte("hello"), 0o644)

	// Set modification time to 2 hours ago
	twoHoursAgo := time.Now().Add(-2 * time.Hour)
	if err := os.Chtimes(dir, twoHoursAgo, twoHoursAgo); err != nil {
		t.Fatal(err)
	}

	items := ScanStaleTmpFiles()
	found := false
	for _, item := range items {
		if item.Name == "sockerless-test-cleanup-stale" {
			found = true
			if item.Category != "tmp" {
				t.Errorf("expected category=tmp, got %s", item.Category)
			}
			if item.Size == 0 {
				t.Error("expected non-zero size")
			}
		}
	}
	if !found {
		t.Error("expected to find stale temp dir in scan results")
	}
}

func TestCleanOrphanedProcesses(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SOCKERLESS_HOME", tmp)

	runDir := filepath.Join(tmp, "run")
	os.MkdirAll(runDir, 0o755)
	os.WriteFile(filepath.Join(runDir, "dead.pid"), []byte("99999999"), 0o644)

	cleaned := CleanOrphanedProcesses()
	if cleaned != 1 {
		t.Errorf("expected 1 cleaned, got %d", cleaned)
	}

	// Verify file is gone
	if _, err := os.Stat(filepath.Join(runDir, "dead.pid")); !os.IsNotExist(err) {
		t.Error("expected PID file to be removed")
	}
}

func TestFormatDuration(t *testing.T) {
	tests := []struct {
		seconds  int
		expected string
	}{
		{30, "30s"},
		{90, "1m"},
		{3700, "1h"},
		{90000, "1d"},
	}
	for _, tt := range tests {
		got := formatDuration(time.Duration(tt.seconds) * time.Second)
		if got != tt.expected {
			t.Errorf("formatDuration(%ds) = %q, want %q", tt.seconds, got, tt.expected)
		}
	}
}

func TestFormatBytes(t *testing.T) {
	tests := []struct {
		input    int64
		expected string
	}{
		{500, "500 B"},
		{2048, "2.0 KB"},
		{1048576, "1.0 MB"},
		{1073741824, "1.0 GB"},
	}
	for _, tt := range tests {
		got := formatBytes(tt.input)
		if got != tt.expected {
			t.Errorf("formatBytes(%d) = %q, want %q", tt.input, got, tt.expected)
		}
	}
}
