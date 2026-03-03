package main

import (
	"os"
	"path/filepath"
	"testing"
)

func TestSockerlessDirFallback(t *testing.T) {
	// Clear SOCKERLESS_HOME so it doesn't short-circuit
	t.Setenv("SOCKERLESS_HOME", "")

	// Clear HOME to trigger os.UserHomeDir() failure
	t.Setenv("HOME", "")

	dir := sockerlessDir()
	if !filepath.IsAbs(dir) {
		t.Errorf("expected absolute path, got %q", dir)
	}
	if dir != filepath.Join(os.TempDir(), ".sockerless") {
		t.Errorf("expected TempDir fallback, got %q", dir)
	}
}

func TestSockerlessDirFromEnv(t *testing.T) {
	t.Setenv("SOCKERLESS_HOME", "/custom/path")
	dir := sockerlessDir()
	if dir != "/custom/path" {
		t.Errorf("expected /custom/path, got %q", dir)
	}
}
