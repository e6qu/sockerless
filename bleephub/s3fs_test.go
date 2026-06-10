package bleephub

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"sort"
	"strconv"
	"strings"
	"sync"
	"testing"
	"time"
)

// fakeS3Server is a minimal in-process S3 HTTP endpoint (path-style) backing
// the real aws-sdk-go-v2 client in tests: object get/put/head/delete plus
// paginated ListObjectsV2 with delimiter roll-up.
type fakeS3Server struct {
	mu        sync.Mutex
	objects   map[string][]byte
	pageSize  int
	listCalls int
	failList  bool
}

func newFakeS3Server() *fakeS3Server {
	return &fakeS3Server{objects: map[string][]byte{}, pageSize: 1000}
}

func (s *fakeS3Server) ServeHTTP(w http.ResponseWriter, r *http.Request) {
	trimmed := strings.TrimPrefix(r.URL.Path, "/")
	_, key, _ := strings.Cut(trimmed, "/")

	s.mu.Lock()
	defer s.mu.Unlock()

	switch {
	case key == "" && r.Method == http.MethodGet:
		s.handleList(w, r)
	case r.Method == http.MethodPut:
		body, err := io.ReadAll(r.Body)
		if err != nil {
			http.Error(w, err.Error(), http.StatusInternalServerError)
			return
		}
		s.objects[key] = body
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodGet:
		data, ok := s.objects[key]
		if !ok {
			writeS3Error(w, http.StatusNotFound, "NoSuchKey", key)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.WriteHeader(http.StatusOK)
		_, _ = w.Write(data)
	case r.Method == http.MethodHead:
		data, ok := s.objects[key]
		if !ok {
			w.WriteHeader(http.StatusNotFound)
			return
		}
		w.Header().Set("Content-Length", strconv.Itoa(len(data)))
		w.Header().Set("Last-Modified", time.Now().UTC().Format(http.TimeFormat))
		w.WriteHeader(http.StatusOK)
	case r.Method == http.MethodDelete:
		delete(s.objects, key)
		w.WriteHeader(http.StatusNoContent)
	default:
		http.Error(w, "unsupported method", http.StatusMethodNotAllowed)
	}
}

func (s *fakeS3Server) handleList(w http.ResponseWriter, r *http.Request) {
	s.listCalls++
	if s.failList {
		// 400 is non-retryable, so the SDK surfaces it without retry backoff.
		writeS3Error(w, http.StatusBadRequest, "InvalidRequest", "")
		return
	}

	q := r.URL.Query()
	prefix := q.Get("prefix")
	delim := q.Get("delimiter")
	token := q.Get("continuation-token")

	var keys []string
	for k := range s.objects {
		if strings.HasPrefix(k, prefix) {
			keys = append(keys, k)
		}
	}
	sort.Strings(keys)

	type entry struct {
		key      string
		size     int
		isPrefix bool
	}
	var entries []entry
	seen := map[string]bool{}
	for _, k := range keys {
		rest := k[len(prefix):]
		if delim != "" {
			if i := strings.Index(rest, delim); i >= 0 {
				cp := prefix + rest[:i+len(delim)]
				if !seen[cp] {
					seen[cp] = true
					entries = append(entries, entry{key: cp, isPrefix: true})
				}
				continue
			}
		}
		entries = append(entries, entry{key: k, size: len(s.objects[k])})
	}

	// The continuation token is the last key of the previous page, so the
	// listing stays correct when objects are deleted between pages.
	start := 0
	if token != "" {
		for start < len(entries) && entries[start].key <= token {
			start++
		}
	}
	end := start + s.pageSize
	if end > len(entries) {
		end = len(entries)
	}
	page := entries[start:end]
	truncated := end < len(entries)

	var b strings.Builder
	b.WriteString(`<?xml version="1.0" encoding="UTF-8"?><ListBucketResult>`)
	fmt.Fprintf(&b, "<Prefix>%s</Prefix><KeyCount>%d</KeyCount><IsTruncated>%t</IsTruncated>", prefix, len(page), truncated)
	if truncated && len(page) > 0 {
		fmt.Fprintf(&b, "<NextContinuationToken>%s</NextContinuationToken>", page[len(page)-1].key)
	}
	for _, e := range page {
		if e.isPrefix {
			fmt.Fprintf(&b, "<CommonPrefixes><Prefix>%s</Prefix></CommonPrefixes>", e.key)
		} else {
			fmt.Fprintf(&b, "<Contents><Key>%s</Key><Size>%d</Size><LastModified>2026-01-01T00:00:00.000Z</LastModified></Contents>", e.key, e.size)
		}
	}
	b.WriteString("</ListBucketResult>")

	w.Header().Set("Content-Type", "application/xml")
	_, _ = w.Write([]byte(b.String()))
}

func writeS3Error(w http.ResponseWriter, status int, code, key string) {
	w.Header().Set("Content-Type", "application/xml")
	w.WriteHeader(status)
	fmt.Fprintf(w, `<?xml version="1.0" encoding="UTF-8"?><Error><Code>%s</Code><Message>%s</Message><Key>%s</Key></Error>`, code, code, key)
}

func newS3FSForTest(t *testing.T, fake *fakeS3Server) *s3FS {
	t.Helper()
	srv := httptest.NewServer(fake)
	t.Cleanup(srv.Close)

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
	fs, err := newS3FS(ctx, srv.URL, "test-bucket", "git")
	if err != nil {
		t.Fatalf("newS3FS: %v", err)
	}
	return fs
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
	fake := newFakeS3Server()
	fs := newS3FSForTest(t, fake)

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
	fake := newFakeS3Server()
	fs := newS3FSForTest(t, fake)

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
	fake := newFakeS3Server()
	fs := newS3FSForTest(t, fake)

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
	fake := newFakeS3Server()
	fs := newS3FSForTest(t, fake)

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
	fake := newFakeS3Server()
	fs := newS3FSForTest(t, fake)

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
	fake := newFakeS3Server()
	fs := newS3FSForTest(t, fake)

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
	fake := newFakeS3Server()
	fs := newS3FSForTest(t, fake)

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
	fake := newFakeS3Server()
	fs := newS3FSForTest(t, fake)

	if _, err := fs.OpenFile("missing", os.O_WRONLY, 0o644); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("OpenFile(O_WRONLY) on missing file: err = %v, want os.ErrNotExist", err)
	}
	if _, err := fs.Open("missing"); !errors.Is(err, os.ErrNotExist) {
		t.Fatalf("Open on missing file: err = %v, want os.ErrNotExist", err)
	}
}

func TestS3FSReadDirPaginated(t *testing.T) {
	fake := newFakeS3Server()
	fake.pageSize = 2
	fs := newS3FSForTest(t, fake)

	for i := 0; i < 7; i++ {
		fake.objects[fmt.Sprintf("git/dir/file-%02d", i)] = []byte("data")
	}
	fake.objects["git/dir/sub/nested-a"] = []byte("data")
	fake.objects["git/dir/sub/nested-b"] = []byte("data")
	fake.objects["git/other/file"] = []byte("data")

	entries, err := fs.ReadDir("dir")
	if err != nil {
		t.Fatalf("ReadDir: %v", err)
	}
	if len(entries) != 8 {
		var names []string
		for _, e := range entries {
			names = append(names, e.Name())
		}
		t.Fatalf("ReadDir returned %d entries (%v), want 8", len(entries), names)
	}
	if fake.listCalls < 2 {
		t.Fatalf("ListObjectsV2 called %d times, want >= 2 (pagination not exercised)", fake.listCalls)
	}
	if !entries[0].IsDir() || entries[0].Name() != "sub" {
		t.Fatalf("first entry = %q (dir=%t), want dir %q", entries[0].Name(), entries[0].IsDir(), "sub")
	}
	for i := 0; i < 7; i++ {
		want := fmt.Sprintf("file-%02d", i)
		if entries[i+1].Name() != want {
			t.Fatalf("entry %d = %q, want %q", i+1, entries[i+1].Name(), want)
		}
	}
}

func TestS3FSDeleteRepoPrefix(t *testing.T) {
	fake := newFakeS3Server()
	fake.pageSize = 2
	fs := newS3FSForTest(t, fake)

	for i := 0; i < 5; i++ {
		fake.objects[fmt.Sprintf("git/owner/repo/objects/%d", i)] = []byte("data")
	}
	fake.objects["git/owner/other/keep"] = []byte("data")

	if err := fs.deleteRepoPrefix("owner/repo"); err != nil {
		t.Fatalf("deleteRepoPrefix: %v", err)
	}

	fake.mu.Lock()
	defer fake.mu.Unlock()
	for k := range fake.objects {
		if strings.HasPrefix(k, "git/owner/repo/") {
			t.Fatalf("object %q not deleted", k)
		}
	}
	if _, ok := fake.objects["git/owner/other/keep"]; !ok {
		t.Fatal("object outside the repo prefix was deleted")
	}
}

func TestS3FSDeleteRepoPrefixPropagatesListError(t *testing.T) {
	fake := newFakeS3Server()
	fake.failList = true
	fs := newS3FSForTest(t, fake)

	fake.objects["git/owner/repo/objects/1"] = []byte("data")

	if err := fs.deleteRepoPrefix("owner/repo"); err == nil {
		t.Fatal("deleteRepoPrefix returned nil, want list error propagated")
	}
}
