package core

import (
	"archive/tar"
	"bytes"
	"io"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
)

// BUG-052: extractTar must propagate io.Copy errors.
func TestExtractTar_CopyError(t *testing.T) {
	// Create a tar with a file header claiming 1000 bytes but only 10 bytes of data.
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	_ = tw.WriteHeader(&tar.Header{
		Name: "test.txt",
		Size: 1000, // claim 1000 bytes
		Mode: 0644,
	})
	tw.Write([]byte("short data")) // only write 10 bytes
	// Don't close tw cleanly — this creates a truncated tar entry

	dir := t.TempDir()
	err := extractTar(&buf, dir)
	if err == nil {
		t.Fatal("expected error from extractTar with truncated tar entry")
	}
}

// BUG-052: extractTar succeeds with valid tar.
func TestExtractTar_Success(t *testing.T) {
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte("hello world")
	_ = tw.WriteHeader(&tar.Header{
		Name: "hello.txt",
		Size: int64(len(content)),
		Mode: 0644,
	})
	tw.Write(content)
	tw.Close()

	dir := t.TempDir()
	if err := extractTar(&buf, dir); err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	data, err := os.ReadFile(filepath.Join(dir, "hello.txt"))
	if err != nil {
		t.Fatalf("failed to read extracted file: %v", err)
	}
	if string(data) != "hello world" {
		t.Fatalf("expected 'hello world', got %q", string(data))
	}
}

// BUG-055: createTar returns error for nonexistent source.
func TestCreateTar_ErrorPath(t *testing.T) {
	var buf bytes.Buffer
	err := createTar(&buf, "/nonexistent/path/that/does/not/exist", "test")
	if err == nil {
		t.Fatal("expected error from createTar with nonexistent path")
	}
}

// BUG-055: createTar succeeds for valid source.
func TestCreateTar_Success(t *testing.T) {
	dir := t.TempDir()
	os.WriteFile(filepath.Join(dir, "a.txt"), []byte("aaa"), 0644)
	os.MkdirAll(filepath.Join(dir, "sub"), 0755)
	os.WriteFile(filepath.Join(dir, "sub", "b.txt"), []byte("bbb"), 0644)

	var buf bytes.Buffer
	err := createTar(&buf, dir, "root")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	// Verify tar contents
	tr := tar.NewReader(&buf)
	names := map[string]bool{}
	for {
		hdr, err := tr.Next()
		if err == io.EOF {
			break
		}
		if err != nil {
			t.Fatalf("tar read error: %v", err)
		}
		names[hdr.Name] = true
	}
	if !names["root/a.txt"] {
		t.Error("expected root/a.txt in tar")
	}
	if !names["root/sub/b.txt"] {
		t.Error("expected root/sub/b.txt in tar")
	}
}

// BUG-055: createTar for a single file.
func TestCreateTar_SingleFile(t *testing.T) {
	dir := t.TempDir()
	fpath := filepath.Join(dir, "single.txt")
	os.WriteFile(fpath, []byte("content"), 0644)

	var buf bytes.Buffer
	err := createTar(&buf, fpath, "single.txt")
	if err != nil {
		t.Fatalf("unexpected error: %v", err)
	}

	tr := tar.NewReader(&buf)
	hdr, err := tr.Next()
	if err != nil {
		t.Fatalf("tar read error: %v", err)
	}
	if hdr.Name != "single.txt" {
		t.Errorf("expected name 'single.txt', got %q", hdr.Name)
	}
	data, _ := io.ReadAll(tr)
	if string(data) != "content" {
		t.Errorf("expected 'content', got %q", string(data))
	}
}

// BUG-053: handlePutArchive returns 500 when driver fails.
func TestHandlePutArchive_DriverError(t *testing.T) {
	store := NewStore()
	s := &BaseServer{
		Store:    store,
		Logger:   zerolog.Nop(),
		Mux:      http.NewServeMux(),
		EventBus: NewEventBus(),
	}
	s.InitDrivers()
	s.self = s

	cID := "c1"
	store.Containers.Put(cID, api.Container{ID: cID, Name: "/test"})
	store.ContainerNames.Put("/test", cID)

	// Send invalid tar data — PutArchive will fail trying to extract
	body := strings.NewReader("this is not a tar archive")
	req := httptest.NewRequest("PUT", "/internal/v1/containers/c1/archive?path=/tmp", body)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handlePutArchive(w, req)

	if w.Code != http.StatusInternalServerError {
		t.Fatalf("expected 500, got %d: %s", w.Code, w.Body.String())
	}
}

// BUG-053: handlePutArchive returns 200 when driver succeeds.
func TestHandlePutArchive_Success(t *testing.T) {
	store := NewStore()
	s := &BaseServer{
		Store:    store,
		Logger:   zerolog.Nop(),
		Mux:      http.NewServeMux(),
		EventBus: NewEventBus(),
	}
	s.InitDrivers()
	s.self = s

	cID := "c1"
	store.Containers.Put(cID, api.Container{ID: cID, Name: "/test"})
	store.ContainerNames.Put("/test", cID)

	// Create a valid tar
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	content := []byte("test")
	tw.WriteHeader(&tar.Header{Name: "test.txt", Size: int64(len(content)), Mode: 0644})
	tw.Write(content)
	tw.Close()

	req := httptest.NewRequest("PUT", "/internal/v1/containers/c1/archive?path=/tmp", &buf)
	req.SetPathValue("id", "c1")
	w := httptest.NewRecorder()
	s.handlePutArchive(w, req)

	if w.Code != http.StatusNoContent {
		t.Fatalf("expected 204, got %d: %s", w.Code, w.Body.String())
	}
}

// BUG-059: handleContainerCommit returns 400 on malformed JSON body.
func TestCommit_MalformedBody(t *testing.T) {
	s := newCommitTestServer()

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/test",
		Config: api.ContainerConfig{
			Image:  "alpine:3.18",
			Cmd:    []string{"/bin/sh"},
			Labels: make(map[string]string),
		},
	})
	s.Store.ContainerNames.Put("/test", cID)

	body := `{this is not valid json}`
	req := httptest.NewRequest("POST", "/internal/v1/commit?container=c1&repo=test&tag=v1", strings.NewReader(body))
	w := httptest.NewRecorder()
	s.handleContainerCommit(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}

// BUG-059: handleContainerCommit succeeds with empty body.
func TestCommit_EmptyBody(t *testing.T) {
	s := newCommitTestServer()

	cID := "c1"
	s.Store.Containers.Put(cID, api.Container{
		ID:   cID,
		Name: "/test",
		Config: api.ContainerConfig{
			Image:  "alpine:3.18",
			Cmd:    []string{"/bin/sh"},
			Labels: make(map[string]string),
		},
	})
	s.Store.ContainerNames.Put("/test", cID)

	req := httptest.NewRequest("POST", "/internal/v1/commit?container=c1&repo=test&tag=v1", nil)
	w := httptest.NewRecorder()
	s.handleContainerCommit(w, req)

	if w.Code != http.StatusCreated {
		t.Fatalf("expected 201, got %d: %s", w.Code, w.Body.String())
	}
}

// BUG-060: handleImageBuild returns 400 on invalid buildargs JSON.
func TestBuild_InvalidBuildargs(t *testing.T) {
	store := NewStore()
	s := &BaseServer{
		Store:    store,
		Logger:   zerolog.Nop(),
		Mux:      http.NewServeMux(),
		EventBus: NewEventBus(),
	}
	s.InitDrivers()
	s.self = s

	// Create a valid tar with a Dockerfile
	var buf bytes.Buffer
	tw := tar.NewWriter(&buf)
	dfContent := []byte("FROM alpine\nCMD echo hello")
	tw.WriteHeader(&tar.Header{Name: "Dockerfile", Size: int64(len(dfContent)), Mode: 0644})
	tw.Write(dfContent)
	tw.Close()

	req := httptest.NewRequest("POST", "/internal/v1/images/build?buildargs=NOT_JSON", &buf)
	w := httptest.NewRecorder()
	s.handleImageBuild(w, req)

	if w.Code != http.StatusBadRequest {
		t.Fatalf("expected 400, got %d: %s", w.Code, w.Body.String())
	}
}
