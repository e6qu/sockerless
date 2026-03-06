package api

import (
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// TestGeneratedFilesUpToDate re-runs the generator in a temp directory and
// verifies the output matches the committed files.
func TestGeneratedFilesUpToDate(t *testing.T) {
	tmpDir := t.TempDir()

	// Run generator with explicit paths.
	specPath, _ := filepath.Abs("openapi.yaml")
	cmd := exec.Command("go", "run", "./gen", specPath, tmpDir)
	cmd.Dir, _ = filepath.Abs(".")
	out, err := cmd.CombinedOutput()
	if err != nil {
		t.Fatalf("generator failed: %v\n%s", err, out)
	}

	for _, name := range []string{"types_gen.go", "backend_gen.go"} {
		t.Run(name, func(t *testing.T) {
			committed, err := os.ReadFile(name)
			if err != nil {
				t.Fatalf("read committed %s: %v", name, err)
			}
			generated, err := os.ReadFile(filepath.Join(tmpDir, name))
			if err != nil {
				t.Fatalf("read generated %s: %v", name, err)
			}
			if string(committed) != string(generated) {
				t.Errorf("%s is stale — run 'go generate ./...' in api/", name)
			}
		})
	}
}
