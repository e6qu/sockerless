package gcpcommon

import (
	"archive/tar"
	"bytes"
	"compress/gzip"
	"io"
	"os"
	"path/filepath"
	"testing"

	core "github.com/sockerless/backend-core"
)

func writeMaliciousTarGz(w io.Writer, relName string) error {
	gz := gzip.NewWriter(w)
	tw := tar.NewWriter(gz)
	hdr := &tar.Header{Name: relName, Typeflag: tar.TypeReg, Mode: 0o644, Size: 0}
	if err := tw.WriteHeader(hdr); err != nil {
		return err
	}
	if err := tw.Close(); err != nil {
		return err
	}
	return gz.Close()
}

// TestGCSSyncDriver_CloudSpec — verifies the driver emits an emptyDir
// spec (NOT a Volume_Gcs FUSE spec) for the receiver-side mount, plus
// fails fast on missing bucket. The GCS bucket flows via env hints in
// PreExec, NOT via the cloud volume spec — the JOB pod-Service mounts
// tmpfs and the bootstrap restores from GCS using the env hint.
func TestGCSSyncDriver_CloudSpec(t *testing.T) {
	d := &GCSSyncDriver{}
	spec, err := d.CloudSpec(core.SharedVolumeRef{
		Name:          "runner-workspace",
		ContainerPath: "/__w",
		Backing:       core.BackingGCSSync,
		GCSBucket:     "my-bucket",
	})
	if err != nil {
		t.Fatal(err)
	}
	if spec.Kind != core.BackingGCSSync {
		t.Errorf("Kind = %q, want gcs-sync", spec.Kind)
	}
	if spec.EmptyDir == nil {
		t.Error("EmptyDir spec must be set — JOB side mounts tmpfs; bootstrap syncs from GCS via env hint")
	}
	if spec.GCS != nil {
		t.Error("GCS spec must be nil — gcs-sync uses env-hint sync, not direct FUSE mount")
	}
}

func TestGCSSyncDriver_CloudSpec_FailsOnMissingBucket(t *testing.T) {
	d := &GCSSyncDriver{}
	_, err := d.CloudSpec(core.SharedVolumeRef{
		Name:          "x",
		ContainerPath: "/y",
		Backing:       core.BackingGCSSync,
	})
	if err == nil {
		t.Error("CloudSpec should fail when GCSBucket is empty")
	}
}

// TestTarRoundtrip — verifies writeTarGzFromDir + readTarGzIntoDir
// produce a byte-identical tree. Critical because the gcs-sync wire
// format is shared with the bootstrap-side `tarFrom`/`untarInto`
// helpers in agent/cmd/sockerless-{cloudrun,gcf}-bootstrap/persist.go;
// any divergence breaks cross-Service workspace propagation.
func TestTarRoundtrip(t *testing.T) {
	src := t.TempDir()
	dst := t.TempDir()

	// Build a small file tree: one regular file + one nested dir + one
	// file under the nested dir + one symlink (gh runner emits these).
	if err := os.WriteFile(filepath.Join(src, "top.txt"), []byte("hello"), 0o644); err != nil {
		t.Fatal(err)
	}
	if err := os.MkdirAll(filepath.Join(src, "sub"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.WriteFile(filepath.Join(src, "sub", "nested.sh"), []byte("#!/bin/sh\necho ok\n"), 0o755); err != nil {
		t.Fatal(err)
	}
	if err := os.Symlink("top.txt", filepath.Join(src, "link")); err != nil {
		t.Fatal(err)
	}

	var buf bytes.Buffer
	if err := writeTarGzFromDir(&buf, src); err != nil {
		t.Fatalf("writeTarGzFromDir: %v", err)
	}
	if err := readTarGzIntoDir(&buf, dst); err != nil {
		t.Fatalf("readTarGzIntoDir: %v", err)
	}

	// Verify entries.
	if got, _ := os.ReadFile(filepath.Join(dst, "top.txt")); string(got) != "hello" {
		t.Errorf("top.txt = %q, want %q", got, "hello")
	}
	if got, _ := os.ReadFile(filepath.Join(dst, "sub", "nested.sh")); string(got) != "#!/bin/sh\necho ok\n" {
		t.Errorf("sub/nested.sh content mismatch: got %q", got)
	}
	link, err := os.Readlink(filepath.Join(dst, "link"))
	if err != nil {
		t.Errorf("readlink dst/link: %v", err)
	}
	if link != "top.txt" {
		t.Errorf("link target = %q, want top.txt", link)
	}

	// Permissions on the executable file are preserved.
	if fi, err := os.Stat(filepath.Join(dst, "sub", "nested.sh")); err == nil {
		if fi.Mode()&0o100 == 0 {
			t.Errorf("nested.sh lost executable bit; mode=%v", fi.Mode())
		}
	}
}

func TestTarRoundtrip_EmptyDir(t *testing.T) {
	src := t.TempDir() // exists but empty
	dst := t.TempDir()

	var buf bytes.Buffer
	if err := writeTarGzFromDir(&buf, src); err != nil {
		t.Fatalf("writeTarGzFromDir on empty dir: %v", err)
	}
	if err := readTarGzIntoDir(&buf, dst); err != nil {
		t.Fatalf("readTarGzIntoDir of empty: %v", err)
	}
	entries, _ := os.ReadDir(dst)
	if len(entries) != 0 {
		t.Errorf("empty roundtrip produced %d entries, want 0", len(entries))
	}
}

func TestTarRoundtrip_NonexistentSourceProducesEmpty(t *testing.T) {
	dst := t.TempDir()
	var buf bytes.Buffer
	if err := writeTarGzFromDir(&buf, "/does/not/exist"); err != nil {
		t.Fatalf("writeTarGzFromDir on missing dir should not error (first-exec path): %v", err)
	}
	if err := readTarGzIntoDir(&buf, dst); err != nil {
		t.Fatalf("readTarGzIntoDir of empty: %v", err)
	}
}

func TestTarRoundtrip_RejectsPathEscape(t *testing.T) {
	dst := t.TempDir()
	// Hand-craft a tar stream with a malicious entry; verify untar refuses.
	var buf bytes.Buffer
	if err := writeMaliciousTarGz(&buf, "../escape.txt"); err != nil {
		t.Fatal(err)
	}
	err := readTarGzIntoDir(&buf, dst)
	if err == nil {
		t.Error("expected error for path-escape entry, got nil")
	}
}

func TestObjectName_StableAcrossCalls(t *testing.T) {
	a := objectName("runner-workspace", "exec-123")
	b := objectName("runner-workspace", "exec-123")
	if a != b {
		t.Errorf("objectName not deterministic: %q vs %q", a, b)
	}
	if a != "workspace/runner-workspace/exec-123.tar.gz" {
		t.Errorf("objectName shape changed: %q", a)
	}
}

func TestObjectName_SanitizesSlashesInVolumeName(t *testing.T) {
	got := objectName("foo/bar", "exec-1")
	if got != "workspace/foo-bar/exec-1.tar.gz" {
		t.Errorf("slash should collapse to dash: %q", got)
	}
}
