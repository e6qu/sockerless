package core

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"testing"

	"github.com/rs/zerolog"
)

func TestLocalFilesystemDriver_PutArchive(t *testing.T) {
	store := NewStore()
	d := &LocalFilesystemDriver{
		Store:  store,
		Logger: zerolog.Nop(),
	}

	containerID := GenerateID()

	// Create a tar archive with a test file
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte("hello from tar")
	if err := tw.WriteHeader(&tar.Header{
		Name: "test.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	// Use a temp dir as the destination path
	destDir := t.TempDir()
	if err := d.PutArchive(containerID, destDir, &buf); err != nil {
		t.Fatalf("PutArchive failed: %v", err)
	}

	// Verify the file was extracted
	extracted := filepath.Join(destDir, "test.txt")
	data, err := os.ReadFile(extracted)
	if err != nil {
		t.Fatalf("expected file %s to exist: %v", extracted, err)
	}
	if string(data) != "hello from tar" {
		t.Errorf("expected %q, got %q", "hello from tar", string(data))
	}
}

func TestLocalFilesystemDriver_PutArchive_Staging(t *testing.T) {
	store := NewStore()
	d := &LocalFilesystemDriver{
		Store:  store,
		Logger: zerolog.Nop(),
	}

	containerID := GenerateID()

	// Use a non-writable path to trigger staging
	destPath := "/usr/local/nonexistent-staging-test"

	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte("staged content")
	if err := tw.WriteHeader(&tar.Header{
		Name: "staged.txt",
		Mode: 0644,
		Size: int64(len(content)),
	}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	if err := d.PutArchive(containerID, destPath, &buf); err != nil {
		t.Fatalf("PutArchive (staging) failed: %v", err)
	}

	// Verify staging dir was created
	v, ok := store.StagingDirs.Load(containerID)
	if !ok {
		t.Fatal("expected staging dir to be recorded")
	}
	stagingDir := v.(string)
	t.Cleanup(func() { os.RemoveAll(stagingDir) })

	// Verify file was staged
	stagedFile := filepath.Join(stagingDir, destPath, "staged.txt")
	data, err := os.ReadFile(stagedFile)
	if err != nil {
		t.Fatalf("expected staged file %s to exist: %v", stagedFile, err)
	}
	if string(data) != "staged content" {
		t.Errorf("expected %q, got %q", "staged content", string(data))
	}
}

func TestLocalFilesystemDriver_StatPath(t *testing.T) {
	store := NewStore()
	d := &LocalFilesystemDriver{
		Store:  store,
		Logger: zerolog.Nop(),
	}

	containerID := GenerateID()

	// Create a temp file to stat
	dir := t.TempDir()
	testFile := filepath.Join(dir, "stat-test.txt")
	if err := os.WriteFile(testFile, []byte("stat me"), 0644); err != nil {
		t.Fatal(err)
	}

	// Add path mapping so StatPath can resolve it
	addPathMapping(store, containerID, "/app", dir)

	info, err := d.StatPath(containerID, "/app/stat-test.txt")
	if err != nil {
		t.Fatalf("StatPath failed: %v", err)
	}
	if info.Name() != "stat-test.txt" {
		t.Errorf("expected name stat-test.txt, got %q", info.Name())
	}
	if info.Size() != 7 {
		t.Errorf("expected size 7, got %d", info.Size())
	}
}
