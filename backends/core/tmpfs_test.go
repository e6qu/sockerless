package core

import (
	"os"
	"testing"
)

func TestResolveTmpfsMounts_CreatesTempDirs(t *testing.T) {
	tmpfs := map[string]string{
		"/tmp":    "rw,noexec",
		"/var/tmp": "",
	}
	result := resolveTmpfsMounts(tmpfs)
	if len(result) != 2 {
		t.Fatalf("expected 2 entries, got %d", len(result))
	}
	for containerPath, hostDir := range result {
		if containerPath != "/tmp" && containerPath != "/var/tmp" {
			t.Errorf("unexpected container path: %s", containerPath)
		}
		info, err := os.Stat(hostDir)
		if err != nil {
			t.Errorf("host dir %s does not exist: %v", hostDir, err)
			continue
		}
		if !info.IsDir() {
			t.Errorf("host dir %s is not a directory", hostDir)
		}
		os.RemoveAll(hostDir)
	}
}

func TestResolveTmpfsMounts_EmptyMap(t *testing.T) {
	result := resolveTmpfsMounts(nil)
	if result != nil {
		t.Errorf("expected nil, got %v", result)
	}
	result = resolveTmpfsMounts(map[string]string{})
	if result != nil {
		t.Errorf("expected nil for empty map, got %v", result)
	}
}

func TestResolveTmpfsMounts_MergesWithBinds(t *testing.T) {
	binds := map[string]string{
		"/data": "/host/data",
	}
	tmpfs := map[string]string{
		"/tmp": "",
	}
	tmpfsDirs := resolveTmpfsMounts(tmpfs)
	for k, v := range tmpfsDirs {
		binds[k] = v
	}

	if len(binds) != 2 {
		t.Fatalf("expected 2 entries after merge, got %d", len(binds))
	}
	if binds["/data"] != "/host/data" {
		t.Error("original bind lost after merge")
	}
	if binds["/tmp"] == "" {
		t.Error("tmpfs mount not merged")
	}
	// Cleanup
	os.RemoveAll(binds["/tmp"])
}
