package realexec

import (
	"os"
	"path/filepath"
	"sort"
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

func TestFirecrackerKernelAssetSelectionIgnoresSidecars(t *testing.T) {
	prefix := "firecracker-ci/v1.15/x86_64/"
	keys := []string{
		prefix + "debug/vmlinux-6.1.155",
		prefix + "debug/vmlinux-6.1.155.debug.gz",
		prefix + "vmlinux-5.10.245",
		prefix + "vmlinux-5.10.245.config",
		prefix + "vmlinux-6.1.155",
		prefix + "vmlinux-6.1.155.config",
	}
	var kernels []string
	for _, key := range keys {
		if isFirecrackerKernelAsset(prefix, key) {
			kernels = append(kernels, key)
		}
	}
	sort.Slice(kernels, func(i, j int) bool { return firecrackerAssetKeyLess(kernels[i], kernels[j]) })
	if len(kernels) == 0 {
		t.Fatal("no kernels selected")
	}
	if got, want := kernels[len(kernels)-1], prefix+"vmlinux-6.1.155"; got != want {
		t.Fatalf("selected kernel = %q, want %q", got, want)
	}
}

func TestVerifyELFKernelRejectsConfigFiles(t *testing.T) {
	dir := t.TempDir()
	kernelPath := filepath.Join(dir, "vmlinux-6.1.155")
	if err := os.WriteFile(kernelPath, []byte{0x7f, 'E', 'L', 'F', 0x02, 0x01}, 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyELFKernel(kernelPath); err != nil {
		t.Fatalf("valid ELF kernel rejected: %v", err)
	}

	configPath := filepath.Join(dir, "vmlinux-6.1.155.config")
	if err := os.WriteFile(configPath, []byte("CONFIG_X86_64=y\n"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := verifyELFKernel(configPath); err == nil {
		t.Fatal("config file accepted as Firecracker kernel")
	}
}
