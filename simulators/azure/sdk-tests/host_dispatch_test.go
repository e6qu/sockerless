package azure_sdk_test

import (
	"os"
	"path/filepath"
	"strings"
	"testing"
)

// Phase 135d — workload-dispatch invariant. No sim handler may execute
// a workload via `os/exec`. See feedback_sim_host_model.md.
func TestNoOsExecOfWorkloads(t *testing.T) {
	allowList := map[string]string{
		// (no production azure/*.go files use os/exec.)
	}

	simDir, _ := filepath.Abs("..")
	entries, err := os.ReadDir(simDir)
	if err != nil {
		t.Fatalf("read sim dir: %v", err)
	}

	for _, e := range entries {
		if e.IsDir() || !strings.HasSuffix(e.Name(), ".go") {
			continue
		}
		if strings.HasSuffix(e.Name(), "_test.go") {
			continue
		}
		if reason, ok := allowList[e.Name()]; ok {
			t.Logf("allowlisted %s: %s", e.Name(), reason)
			continue
		}
		path := filepath.Join(simDir, e.Name())
		body, err := os.ReadFile(path)
		if err != nil {
			t.Fatalf("read %s: %v", path, err)
		}
		text := string(body)
		if strings.Contains(text, `"os/exec"`) || strings.Contains(text, "exec.Command") {
			t.Errorf("%s imports os/exec or calls exec.Command — workloads must dispatch via Docker (StartContainerSync), not host process. See feedback_sim_host_model.md.", e.Name())
		}
	}
}
