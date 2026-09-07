package core

import (
	"archive/tar"
	"bytes"
	"os"
	"os/exec"
	"path/filepath"
	"testing"
)

// The put-archive command must create a destination directory that does
// not exist yet, exactly like the Docker daemon does for `docker cp` —
// act relies on it when it copies its runtime files into /var/run/act/.
// The command is run as-is with a real shell and a real tar body.
func TestPutArchiveArgvCreatesMissingDestination(t *testing.T) {
	var body bytes.Buffer
	tw := tar.NewWriter(&body)
	content := []byte("#!/bin/sh\necho hi\n")
	if err := tw.WriteHeader(&tar.Header{Name: "run.sh", Mode: 0o755, Size: int64(len(content))}); err != nil {
		t.Fatal(err)
	}
	if _, err := tw.Write(content); err != nil {
		t.Fatal(err)
	}
	if err := tw.Close(); err != nil {
		t.Fatal(err)
	}

	dst := filepath.Join(t.TempDir(), "var", "run", "act with space") + "/"
	argv := putArchiveArgv(dst)
	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = &body
	if out, err := cmd.CombinedOutput(); err != nil {
		t.Fatalf("%v: %s", err, out)
	}
	got, err := os.ReadFile(filepath.Join(dst, "run.sh"))
	if err != nil {
		t.Fatal(err)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("extracted %q, want %q", got, content)
	}
}
