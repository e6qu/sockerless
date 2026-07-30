package core

import (
	"archive/tar"
	"bytes"
	"os"
	"path/filepath"
	"strconv"
	"sync"
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

	// A child of a regular file cannot be created on any platform or under any
	// user, so direct extraction must take the staging path.
	blocker := filepath.Join(t.TempDir(), "not-a-directory")
	if err := os.WriteFile(blocker, []byte("block mkdir"), 0600); err != nil {
		t.Fatal(err)
	}
	destPath := filepath.Join(blocker, "staging-target")

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

// TestAddPathMapping_ConcurrentNoCrash drives concurrent writers and
// readers against the same container's path mappings. Before the
// copy-on-write fix, addPathMapping mutated the inner map in place while
// resolveContainerPath ranged over it, so the race detector (and, in
// production, the runtime) reported "concurrent map writes" /
// "concurrent map iteration and map write" — a fatal, unrecoverable
// crash of the whole process. With the fix every writer Stores a fresh
// snapshot under PathMappingsMu and readers only range over immutable
// snapshots.
func TestAddPathMapping_ConcurrentNoCrash(t *testing.T) {
	store := NewStore()
	containerID := GenerateID()

	const writers = 8
	const readers = 8
	const iterations = 200

	var wg sync.WaitGroup
	wg.Add(writers + readers)

	for w := 0; w < writers; w++ {
		go func(w int) {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				cp := "/data/" + strconv.Itoa(w) + "/" + strconv.Itoa(i)
				addPathMapping(store, containerID, cp, "/host"+cp)
			}
		}(w)
	}

	for r := 0; r < readers; r++ {
		go func() {
			defer wg.Done()
			for i := 0; i < iterations; i++ {
				_ = resolveContainerPath(containerID, "/data/0/0/file", store)
			}
		}()
	}

	wg.Wait()
}
