package realexec

import (
	"path/filepath"
	"strings"
	"testing"
)

func TestShortPathNameKeepsFirecrackerSocketPathShort(t *testing.T) {
	id := "gcp-projects-test-project-zones-us-central1-a-instances-sdk-vm-1"
	name := shortPathName(id)
	if strings.Contains(name, "/") {
		t.Fatalf("short path name contains path separator: %q", name)
	}
	socketPath := filepath.Join("/tmp", "sockerless-firecracker", name+"-123456", "firecracker.socket")
	if len(socketPath) >= 100 {
		t.Fatalf("Firecracker socket path is too long: len=%d path=%s", len(socketPath), socketPath)
	}
}
