package bleephub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net"
	"net/http"
	"os"
	"os/exec"
	"path/filepath"
	"strings"
	"sync"
	"testing"
	"time"

	"github.com/aws/aws-sdk-go-v2/aws"
	"github.com/aws/aws-sdk-go-v2/service/s3"
)

var (
	s3SimulatorBuildOnce sync.Once
	s3SimulatorBinary    string
	s3SimulatorBuildErr  error
)

func resetS3FSCacheForTest(t *testing.T) {
	t.Helper()
	s3FSMu.Lock()
	s3FSCache = nil
	s3FSErr = nil
	s3FSInited = false
	s3FSMu.Unlock()
	t.Cleanup(func() {
		s3FSMu.Lock()
		s3FSCache = nil
		s3FSErr = nil
		s3FSInited = false
		s3FSMu.Unlock()
	})
}

func newS3FSForTest(t *testing.T) *s3FS {
	t.Helper()
	endpoint := startS3SimulatorForTest(t)

	tmp := t.TempDir()
	t.Setenv("AWS_ACCESS_KEY_ID", "test-key")
	t.Setenv("AWS_SECRET_ACCESS_KEY", "test-secret")
	t.Setenv("AWS_PROFILE", "")
	t.Setenv("AWS_CONFIG_FILE", filepath.Join(tmp, "aws-config"))
	t.Setenv("AWS_SHARED_CREDENTIALS_FILE", filepath.Join(tmp, "aws-credentials"))
	t.Setenv("AWS_EC2_METADATA_DISABLED", "true")
	t.Setenv("AWS_REQUEST_CHECKSUM_CALCULATION", "when_required")
	t.Setenv("AWS_RESPONSE_CHECKSUM_VALIDATION", "when_required")

	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	fs, err := newS3FS(ctx, endpoint, "test-bucket", "git")
	if err != nil {
		t.Fatalf("newS3FS: %v", err)
	}
	if _, err := fs.client.CreateBucket(ctx, &s3.CreateBucketInput{Bucket: aws.String(fs.bucket)}); err != nil {
		t.Fatalf("CreateBucket: %v", err)
	}
	return fs
}

func newObjectByteStoreForTest(t *testing.T) (*s3FS, actionsByteStore) {
	t.Helper()
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: fs.bucket, prefix: "objects"}
	return objectFS, &s3ActionsByteStore{fs: objectFS}
}

func startS3SimulatorForTest(t *testing.T) string {
	t.Helper()
	bin := buildS3SimulatorForTest(t)
	addr := freeLocalAddr(t)
	endpoint := "http://" + addr
	cmd := exec.Command(bin)
	cmd.Env = append(os.Environ(),
		"SIM_RUNTIME=process",
		"SIM_LISTEN_ADDR="+addr,
		"SIM_LOG_LEVEL=error",
	)
	var output bytes.Buffer
	cmd.Stdout = &output
	cmd.Stderr = &output
	if err := cmd.Start(); err != nil {
		t.Fatalf("start simulator-aws: %v", err)
	}
	t.Cleanup(func() {
		if cmd.Process != nil {
			_ = cmd.Process.Kill()
		}
		if err := cmd.Wait(); err != nil && !strings.Contains(err.Error(), "signal: killed") {
			t.Logf("simulator-aws exit: %v\n%s", err, output.String())
		}
	})

	deadline := time.Now().Add(20 * time.Second)
	for time.Now().Before(deadline) {
		resp, err := http.Get(endpoint + "/health")
		if err == nil {
			_ = resp.Body.Close()
			if resp.StatusCode == http.StatusOK {
				return endpoint
			}
		}
		time.Sleep(100 * time.Millisecond)
	}
	t.Fatalf("simulator-aws did not become healthy at %s\n%s", endpoint, output.String())
	return ""
}

func buildS3SimulatorForTest(t *testing.T) string {
	t.Helper()
	s3SimulatorBuildOnce.Do(func() {
		dir, err := os.MkdirTemp("", "bleephub-s3fs-sim-*")
		if err != nil {
			s3SimulatorBuildErr = err
			return
		}
		s3SimulatorBinary = filepath.Join(dir, "simulator-aws")
		ctx, cancel := context.WithTimeout(context.Background(), 3*time.Minute)
		defer cancel()
		cmd := exec.CommandContext(ctx, "go", "build", "-tags", "noui", "-o", s3SimulatorBinary, ".")
		cmd.Dir = filepath.Join("..", "simulators", "aws")
		cmd.Env = append(os.Environ(), "GOWORK=off")
		var output bytes.Buffer
		cmd.Stdout = &output
		cmd.Stderr = &output
		if err := cmd.Run(); err != nil {
			s3SimulatorBuildErr = fmt.Errorf("go build simulator-aws: %w\n%s", err, output.String())
			return
		}
	})
	if s3SimulatorBuildErr != nil {
		t.Fatal(s3SimulatorBuildErr)
	}
	return s3SimulatorBinary
}

func freeLocalAddr(t *testing.T) string {
	t.Helper()
	ln, err := net.Listen("tcp", "127.0.0.1:0")
	if err != nil {
		t.Fatalf("listen free port: %v", err)
	}
	addr := ln.Addr().String()
	if err := ln.Close(); err != nil {
		t.Fatalf("close free port listener: %v", err)
	}
	return addr
}

func putS3RawObject(t *testing.T, fs *s3FS, key string, content []byte) {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	if _, err := fs.client.PutObject(ctx, &s3.PutObjectInput{
		Bucket: aws.String(fs.bucket),
		Key:    aws.String(key),
		Body:   bytes.NewReader(content),
	}); err != nil {
		t.Fatalf("PutObject %s: %v", key, err)
	}
}

func listS3RawKeys(t *testing.T, fs *s3FS, prefix string) []string {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 30*time.Second)
	defer cancel()
	var keys []string
	var continuation *string
	for {
		resp, err := fs.client.ListObjectsV2(ctx, &s3.ListObjectsV2Input{
			Bucket:            aws.String(fs.bucket),
			Prefix:            aws.String(prefix),
			ContinuationToken: continuation,
		})
		if err != nil {
			t.Fatalf("ListObjectsV2 %s: %v", prefix, err)
		}
		for _, obj := range resp.Contents {
			keys = append(keys, aws.ToString(obj.Key))
		}
		if !aws.ToBool(resp.IsTruncated) {
			return keys
		}
		continuation = resp.NextContinuationToken
	}
}

func writeS3TestFile(t *testing.T, fs *s3FS, name string, content []byte) {
	t.Helper()
	f, err := fs.Create(name)
	if err != nil {
		t.Fatalf("Create %s: %v", name, err)
	}
	if _, err := f.Write(content); err != nil {
		t.Fatalf("Write %s: %v", name, err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close %s: %v", name, err)
	}
}

func readS3TestFile(t *testing.T, fs *s3FS, name string) []byte {
	t.Helper()
	f, err := fs.Open(name)
	if err != nil {
		t.Fatalf("Open %s: %v", name, err)
	}
	defer f.Close()
	data, err := io.ReadAll(f)
	if err != nil {
		t.Fatalf("ReadAll %s: %v", name, err)
	}
	return data
}

func TestS3FileReadFullFileInChunks(t *testing.T) {
	fs := newS3FSForTest(t)

	content := make([]byte, 5000)
	for i := range content {
		content[i] = byte(i % 251)
	}
	writeS3TestFile(t, fs, "owner/repo/objects/pack/pack-1.pack", content)

	f, err := fs.Open("owner/repo/objects/pack/pack-1.pack")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	// io.ReadAll reads in small chunks; bound it with a timeout so a Read
	// that returns (0, nil) forever fails instead of hanging the suite.
	var got []byte
	var readErr error
	done := make(chan struct{})
	go func() {
		defer close(done)
		got, readErr = io.ReadAll(f)
	}()
	select {
	case <-done:
	case <-time.After(10 * time.Second):
		t.Fatal("io.ReadAll did not terminate: Read made no progress")
	}
	if readErr != nil {
		t.Fatalf("ReadAll: %v", readErr)
	}
	if !bytes.Equal(got, content) {
		t.Fatalf("ReadAll returned %d bytes, want %d (content mismatch)", len(got), len(content))
	}

	// Explicit fixed-size chunked reads must also drain the whole file.
	f2, err := fs.Open("owner/repo/objects/pack/pack-1.pack")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f2.Close()
	var chunked []byte
	buf := make([]byte, 512)
	for i := 0; ; i++ {
		if i > 2*len(content)/len(buf)+10 {
			t.Fatal("chunked read loop did not reach EOF")
		}
		n, err := f2.Read(buf)
		chunked = append(chunked, buf[:n]...)
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("Read: %v", err)
		}
	}
	if !bytes.Equal(chunked, content) {
		t.Fatalf("chunked read returned %d bytes, want %d (content mismatch)", len(chunked), len(content))
	}
}

func TestS3FileSeek(t *testing.T) {
	fs := newS3FSForTest(t)

	writeS3TestFile(t, fs, "f", []byte("hello world"))

	f, err := fs.Open("f")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	readAll := func() string {
		t.Helper()
		data, err := io.ReadAll(f)
		if err != nil {
			t.Fatalf("ReadAll: %v", err)
		}
		return string(data)
	}

	if pos, err := f.Seek(6, io.SeekStart); err != nil || pos != 6 {
		t.Fatalf("Seek(6, SeekStart) = %d, %v", pos, err)
	}
	if got := readAll(); got != "world" {
		t.Fatalf("after SeekStart: got %q, want %q", got, "world")
	}

	if pos, err := f.Seek(-5, io.SeekEnd); err != nil || pos != 6 {
		t.Fatalf("Seek(-5, SeekEnd) = %d, %v", pos, err)
	}
	if got := readAll(); got != "world" {
		t.Fatalf("after SeekEnd: got %q, want %q", got, "world")
	}

	if pos, err := f.Seek(0, io.SeekStart); err != nil || pos != 0 {
		t.Fatalf("Seek(0, SeekStart) = %d, %v", pos, err)
	}
	five := make([]byte, 5)
	if _, err := io.ReadFull(f, five); err != nil || string(five) != "hello" {
		t.Fatalf("ReadFull = %q, %v", five, err)
	}
	if pos, err := f.Seek(1, io.SeekCurrent); err != nil || pos != 6 {
		t.Fatalf("Seek(1, SeekCurrent) = %d, %v", pos, err)
	}
	if got := readAll(); got != "world" {
		t.Fatalf("after SeekCurrent: got %q, want %q", got, "world")
	}

	if _, err := f.Seek(-1, io.SeekStart); err == nil {
		t.Fatal("Seek to negative position succeeded, want error")
	}

	if pos, err := f.Seek(100, io.SeekStart); err != nil || pos != 100 {
		t.Fatalf("Seek(100, SeekStart) = %d, %v", pos, err)
	}
	if n, err := f.Read(make([]byte, 1)); n != 0 || err != io.EOF {
		t.Fatalf("Read past end = %d, %v, want 0, io.EOF", n, err)
	}
}

func TestS3FileReadAt(t *testing.T) {
	fs := newS3FSForTest(t)

	writeS3TestFile(t, fs, "f", []byte("hello world"))

	f, err := fs.Open("f")
	if err != nil {
		t.Fatalf("Open: %v", err)
	}
	defer f.Close()

	p := make([]byte, 5)
	if n, err := f.ReadAt(p, 6); n != 5 || err != nil || string(p) != "world" {
		t.Fatalf("ReadAt(6) = %d, %v, %q", n, err, p)
	}
	// ReadAt must not move the read position.
	data, err := io.ReadAll(f)
	if err != nil || string(data) != "hello world" {
		t.Fatalf("ReadAll after ReadAt = %q, %v", data, err)
	}
	if n, err := f.ReadAt(p, 11); n != 0 || err != io.EOF {
		t.Fatalf("ReadAt past end = %d, %v, want 0, io.EOF", n, err)
	}
}

func TestS3FSOpenFileWriteWithoutTruncPreservesContent(t *testing.T) {
	fs := newS3FSForTest(t)

	writeS3TestFile(t, fs, "f", []byte("hello world"))

	f, err := fs.OpenFile("f", os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(O_CREATE|O_WRONLY): %v", err)
	}
	if _, err := f.Write([]byte("HELLO")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := string(readS3TestFile(t, fs, "f")); got != "HELLO world" {
		t.Fatalf("after O_WRONLY overwrite: got %q, want %q", got, "HELLO world")
	}
}

func TestS3FSOpenFileAppend(t *testing.T) {
	fs := newS3FSForTest(t)

	writeS3TestFile(t, fs, "f", []byte("hello"))

	f, err := fs.OpenFile("f", os.O_WRONLY|os.O_APPEND, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(O_WRONLY|O_APPEND): %v", err)
	}
	if _, err := f.Write([]byte(" world")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := string(readS3TestFile(t, fs, "f")); got != "hello world" {
		t.Fatalf("after append: got %q, want %q", got, "hello world")
	}
}

func TestS3FSOpenFileTrunc(t *testing.T) {
	fs := newS3FSForTest(t)

	writeS3TestFile(t, fs, "f", []byte("hello world"))

	f, err := fs.OpenFile("f", os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(O_TRUNC): %v", err)
	}
	if _, err := f.Write([]byte("x")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := string(readS3TestFile(t, fs, "f")); got != "x" {
		t.Fatalf("after O_TRUNC rewrite: got %q, want %q", got, "x")
	}
}

func TestS3FSOpenFileExcl(t *testing.T) {
	fs := newS3FSForTest(t)

	writeS3TestFile(t, fs, "f", []byte("hello"))

	if _, err := fs.OpenFile("f", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644); !errors.Is(err, os.ErrExist) {
		t.Fatalf("OpenFile(O_CREATE|O_EXCL) on existing file: err = %v, want os.ErrExist", err)
	}
	if got := string(readS3TestFile(t, fs, "f")); got != "hello" {
		t.Fatalf("O_EXCL failure must not touch content: got %q", got)
	}

	f, err := fs.OpenFile("g", os.O_WRONLY|os.O_CREATE|os.O_EXCL, 0o644)
	if err != nil {
		t.Fatalf("OpenFile(O_CREATE|O_EXCL) on new file: %v", err)
	}
	if _, err := f.Write([]byte("new")); err != nil {
		t.Fatalf("Write: %v", err)
	}
	if err := f.Close(); err != nil {
		t.Fatalf("Close: %v", err)
	}
	if got := string(readS3TestFile(t, fs, "g")); got != "new" {
		t.Fatalf("got %q, want %q", got, "new")
	}
}

func TestS3FSOpenFileNonexistentWithoutCreate(t *testing.T) {
	fs := newS3FSForTest(t)

	if _, err := fs.OpenFile("missing", os.O_WRONLY, 0o644); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenFile(O_WRONLY) on missing file: err = %v, want os.ErrNotExist", err)
	}
	if _, err := fs.Open("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open on missing file: err = %v, want os.ErrNotExist", err)
	}
}

func TestS3FSReadDirPaginated(t *testing.T) {
	fs := newS3FSForTest(t)

	for i := 0; i < 1005; i++ {
		putS3RawObject(t, fs, fmt.Sprintf("git/dir/file-%04d", i), []byte("data"))
	}
	putS3RawObject(t, fs, "git/dir/sub/nested-a", []byte("data"))
	putS3RawObject(t, fs, "git/dir/sub/nested-b", []byte("data"))
	putS3RawObject(t, fs, "git/other/file", []byte("data"))

	entries, err := fs.ReadDir("dir")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 1006 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("ReadDir returned %d entries (%v), want 1006", len(entries), names)
	}
	if !entries[0].IsDir() || entries[0].Name() != "sub" {
		t.Fatalf("first entry = %q (dir=%t), want dir %q", entries[0].Name(), entries[0].IsDir(), "sub")
	}
	for i := 0; i < 1005; i++ {
		want := fmt.Sprintf("file-%04d", i)
		if entries[i+1].Name() != want {
			t.Fatalf("entry %d = %q, want %q", i+1, entries[i+1].Name(), want)
		}
	}
}

func TestS3FSDeleteRepoPrefix(t *testing.T) {
	fs := newS3FSForTest(t)

	for i := 0; i < 1005; i++ {
		putS3RawObject(t, fs, fmt.Sprintf("git/owner/repo/objects/%04d", i), []byte("data"))
	}
	putS3RawObject(t, fs, "git/owner/other/keep", []byte("data"))

	if err := fs.deleteRepoPrefix("owner/repo"); err != nil {
		t.Fatalf("deleteRepoPrefix: %v", err)
	}

	keys := listS3RawKeys(t, fs, "git/owner/")
	for _, k := range keys {
		if strings.HasPrefix(k, "git/owner/repo/") {
			t.Fatalf("object %q not deleted", k)
		}
	}
	if len(keys) != 1 || keys[0] != "git/owner/other/keep" {
		t.Fatal("object outside the repo prefix was deleted")
	}
}

func TestS3FSRenameRepoPrefix(t *testing.T) {
	fs := newS3FSForTest(t)

	putS3RawObject(t, fs, "git/owner/repo/objects/pack/a.pack", []byte("pack-a"))
	putS3RawObject(t, fs, "git/owner/repo/refs/heads/main", []byte("sha-main"))
	putS3RawObject(t, fs, "git/owner/other/refs/heads/main", []byte("keep"))

	if err := fs.renameRepoPrefix("owner/repo", "new-owner/new-repo"); err != nil {
		t.Fatalf("renameRepoPrefix: %v", err)
	}

	oldKeys := listS3RawKeys(t, fs, "git/owner/repo/")
	if len(oldKeys) != 0 {
		t.Fatalf("old repo keys survived rename: %v", oldKeys)
	}
	newKeys := listS3RawKeys(t, fs, "git/new-owner/new-repo/")
	wantNew := []string{
		"git/new-owner/new-repo/objects/pack/a.pack",
		"git/new-owner/new-repo/refs/heads/main",
	}
	if strings.Join(newKeys, "\n") != strings.Join(wantNew, "\n") {
		t.Fatalf("new repo keys = %v, want %v", newKeys, wantNew)
	}
	kept := listS3RawKeys(t, fs, "git/owner/other/")
	if len(kept) != 1 || kept[0] != "git/owner/other/refs/heads/main" {
		t.Fatalf("unrelated repo keys = %v, want owner/other preserved", kept)
	}
	if got := string(readS3TestFile(t, fs, "new-owner/new-repo/refs/heads/main")); got != "sha-main" {
		t.Fatalf("renamed ref content = %q, want sha-main", got)
	}
}

func TestS3FSDeleteRepoPrefixPropagatesListError(t *testing.T) {
	fs := newS3FSForTest(t)
	fs.bucket = "missing-bucket"

	if err := fs.deleteRepoPrefix("owner/repo"); err == nil {
		t.Fatal("deleteRepoPrefix returned nil, want list error propagated")
	}
}
