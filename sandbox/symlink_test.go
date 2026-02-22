package sandbox_test

import (
	"bytes"
	"context"
	"os"
	"path/filepath"
	"testing"

	"github.com/sockerless/sandbox"
)

func TestVolumeSymlink(t *testing.T) {
	ctx := context.Background()
	rt, err := sandbox.NewRuntime(ctx)
	if err != nil {
		t.Fatal(err)
	}
	defer rt.Close(ctx)

	// Create a "volume" directory
	volDir, err := os.MkdirTemp("", "test-vol-*")
	if err != nil {
		t.Fatal(err)
	}
	defer os.RemoveAll(volDir)

	// Create a process with a bind mount
	binds := map[string]string{"/cache": volDir}
	proc, err := sandbox.NewProcess(rt, []string{"sh", "-c", "echo 'build artifacts' > /cache/artifacts.txt && cat /cache/artifacts.txt"}, nil, binds)
	if err != nil {
		t.Fatal(err)
	}
	defer proc.Close()

	code := proc.Wait()
	t.Logf("exit code: %d", code)
	t.Logf("logs: %q", string(proc.LogBytes()))

	// Check the volume dir for the file
	content, err := os.ReadFile(filepath.Join(volDir, "artifacts.txt"))
	if err != nil {
		t.Logf("files in volDir: ")
		entries, _ := os.ReadDir(volDir)
		for _, e := range entries {
			t.Logf("  %s", e.Name())
		}
		t.Fatalf("failed to read artifacts.txt from volume: %v", err)
	}
	t.Logf("volume file content: %q", string(content))

	if code != 0 {
		t.Fatalf("expected exit code 0, got %d", code)
	}

	// Now create a second process that reads from the same volume
	var stdout bytes.Buffer
	proc2, err := sandbox.NewProcess(rt, []string{"cat", "/cache/artifacts.txt"}, nil, binds)
	if err != nil {
		t.Fatal(err)
	}
	defer proc2.Close()

	code2 := proc2.Wait()
	t.Logf("proc2 exit code: %d", code2)
	t.Logf("proc2 logs: %q", string(proc2.LogBytes()))
	_ = stdout

	if code2 != 0 {
		t.Fatalf("expected exit code 0 for second process, got %d", code2)
	}
}
