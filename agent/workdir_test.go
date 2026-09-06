package agent

import (
	"os"
	"path/filepath"
	"testing"
)

func TestEnsureWorkdirCreatesAMissingDirectory(t *testing.T) {
	dir := filepath.Join(t.TempDir(), "src", "nested")
	if got := EnsureWorkdir(dir); got != dir {
		t.Fatalf("EnsureWorkdir = %q, want %q", got, dir)
	}
	info, err := os.Stat(dir)
	if err != nil || !info.IsDir() {
		t.Fatalf("directory not created: %v", err)
	}
	if got := EnsureWorkdir(""); got != "" {
		t.Errorf("an empty working directory stays empty, got %q", got)
	}
}
