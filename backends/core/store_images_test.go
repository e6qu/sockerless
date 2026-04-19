package core

import (
	"os"
	"path/filepath"
	"testing"

	"github.com/sockerless/api"
)

// TestStore_PersistAndRestoreImages covers BUG-697 — `docker pull`
// state must survive backend restart. Tests the minimum guarantee:
// serialize all image entries, restore from the same file into a
// fresh Store, every key returns the same image.
func TestStore_PersistAndRestoreImages(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "images.json")

	// Seed the store with a couple of images, using the alias pattern
	// StoreImageWithAliases produces for a Docker Hub ref.
	src := NewStore()
	src.ImageStatePath = path
	alpine := api.Image{ID: "sha256:abc123"}
	StoreImageWithAliases(src, "docker.io/library/alpine:latest", alpine)
	nginx := api.Image{ID: "sha256:def456"}
	StoreImageWithAliases(src, "docker.io/library/nginx:1.25", nginx)

	if len(src.Images.Keys()) == 0 {
		t.Fatal("seed: expected keys, got none")
	}

	// The persist call should have fired from inside StoreImageWithAliases.
	if _, err := os.Stat(path); err != nil {
		t.Fatalf("expected persisted file at %s: %v", path, err)
	}

	// Restore into a fresh Store — simulating backend restart.
	dst := NewStore()
	if err := dst.RestoreImages(path); err != nil {
		t.Fatalf("RestoreImages: %v", err)
	}

	for _, key := range []string{
		"sha256:abc123",
		"docker.io/library/alpine:latest",
		"docker.io/library/alpine",
		"alpine:latest",
		"alpine",
		"sha256:def456",
		"docker.io/library/nginx:1.25",
		"nginx:1.25",
		"nginx",
	} {
		img, ok := dst.Images.Get(key)
		if !ok {
			t.Errorf("key %q: not found in restored store", key)
			continue
		}
		if img.ID != "sha256:abc123" && img.ID != "sha256:def456" {
			t.Errorf("key %q: unexpected ID %q", key, img.ID)
		}
	}
}

// TestStore_RestoreImages_MissingFile ensures a missing file is not
// treated as an error — it's the expected first-startup state.
func TestStore_RestoreImages_MissingFile(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "nope.json")
	s := NewStore()
	if err := s.RestoreImages(path); err != nil {
		t.Fatalf("missing-file restore should not error: %v", err)
	}
	if s.Images.Len() != 0 {
		t.Fatalf("expected empty store, got %d entries", s.Images.Len())
	}
}

// TestStore_PersistImages_EmptyPathNoop documents that an empty path
// is a no-op — callers can safely leave ImageStatePath unset when
// persistence isn't wanted (e.g. unit tests).
func TestStore_PersistImages_EmptyPathNoop(t *testing.T) {
	s := NewStore()
	s.Images.Put("k", api.Image{ID: "x"})
	if err := s.PersistImages(""); err != nil {
		t.Fatalf("empty path should not error: %v", err)
	}
}

// TestStore_PersistImages_AtomicReplace verifies the temp-file pattern
// doesn't leak partial files if the rename step is interrupted (the
// assertion here is that only the target file exists after success —
// the temp file must be renamed, not left alongside).
func TestStore_PersistImages_AtomicReplace(t *testing.T) {
	dir := t.TempDir()
	path := filepath.Join(dir, "images.json")
	s := NewStore()
	s.Images.Put("k", api.Image{ID: "x"})

	if err := s.PersistImages(path); err != nil {
		t.Fatalf("PersistImages: %v", err)
	}

	entries, err := os.ReadDir(dir)
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	for _, e := range entries {
		if e.Name() != "images.json" {
			t.Errorf("unexpected leftover file: %s", e.Name())
		}
	}
}
