package main

import (
	"bytes"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"strings"
	"testing"
)

func TestSplitSharePath(t *testing.T) {
	cases := []struct {
		in        string
		wantShare string
		wantRel   string
	}{
		{"/share-1/dir/file.txt", "share-1", "dir/file.txt"},
		{"/share-1/file.txt", "share-1", "file.txt"},
		{"/share-1/", "share-1", ""},
		{"/share-1", "", ""},
		{"", "", ""},
	}
	for _, tc := range cases {
		gs, gr := splitSharePath(tc.in)
		if gs != tc.wantShare || gr != tc.wantRel {
			t.Errorf("splitSharePath(%q) = (%q, %q); want (%q, %q)",
				tc.in, gs, gr, tc.wantShare, tc.wantRel)
		}
	}
}

func TestHandleAzureFilesPath_PutGetDeleteRoundTrip(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SIM_AZURE_FILES_DATA_DIR", tmp)
	hostPath := filepath.Join(FileShareHostDir("acct", "share-1"), "dir/file.txt")

	// PUT directory
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/share-1/dir?restype=directory", nil)
	if err := handleAzureFilesPath(rec, req, filepath.Dir(hostPath), "directory", ""); err != nil {
		t.Fatalf("mkdir: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("mkdir code = %d, want 201", rec.Code)
	}

	// PUT file (empty placeholder)
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/share-1/dir/file.txt", nil)
	req.Header.Set("x-ms-content-length", "12")
	if err := handleAzureFilesPath(rec, req, hostPath, "", ""); err != nil {
		t.Fatalf("create file: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("create file code = %d, want 201", rec.Code)
	}
	if info, err := os.Stat(hostPath); err != nil || info.Size() != 12 {
		t.Errorf("placeholder size: stat=%v size=%d want 12", err, infoSize(info))
	}

	// PUT range write
	rec = httptest.NewRecorder()
	body := []byte("hello world!")
	req = httptest.NewRequest(http.MethodPut, "/share-1/dir/file.txt?comp=range", bytes.NewReader(body))
	req.Header.Set("Content-Range", "bytes=0-11/*")
	if err := handleAzureFilesPath(rec, req, hostPath, "", "range"); err != nil {
		t.Fatalf("write range: %v", err)
	}
	if rec.Code != http.StatusCreated {
		t.Errorf("write range code = %d, want 201", rec.Code)
	}

	// GET
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodGet, "/share-1/dir/file.txt", nil)
	if err := handleAzureFilesPath(rec, req, hostPath, "", ""); err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Code != http.StatusOK {
		t.Errorf("get code = %d, want 200", rec.Code)
	}
	if got := rec.Body.String(); got != "hello world!" {
		t.Errorf("get body = %q, want %q", got, "hello world!")
	}

	// DELETE
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodDelete, "/share-1/dir/file.txt", nil)
	if err := handleAzureFilesPath(rec, req, hostPath, "", ""); err != nil {
		t.Fatalf("delete: %v", err)
	}
	if rec.Code != http.StatusAccepted {
		t.Errorf("delete code = %d, want 202", rec.Code)
	}
	if _, err := os.Stat(hostPath); !os.IsNotExist(err) {
		t.Errorf("file still exists after delete: %v", err)
	}
}

func TestHandleAzureFilesPath_GetMissingReturnsNotFound(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SIM_AZURE_FILES_DATA_DIR", tmp)
	hostPath := filepath.Join(FileShareHostDir("acct", "share-1"), "missing.txt")

	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodGet, "/share-1/missing.txt", nil)
	if err := handleAzureFilesPath(rec, req, hostPath, "", ""); err != nil {
		t.Fatalf("get: %v", err)
	}
	if rec.Code != http.StatusNotFound {
		t.Errorf("get code = %d, want 404", rec.Code)
	}
	if !strings.Contains(rec.Body.String(), "ResourceNotFound") {
		t.Errorf("get body = %q, want ResourceNotFound", rec.Body.String())
	}
}

func TestHandleAzureFilesPath_RangeOffsetWrite(t *testing.T) {
	tmp := t.TempDir()
	t.Setenv("SIM_AZURE_FILES_DATA_DIR", tmp)
	hostPath := filepath.Join(FileShareHostDir("acct", "share-1"), "f.bin")

	// Create empty 16-byte file
	rec := httptest.NewRecorder()
	req := httptest.NewRequest(http.MethodPut, "/share-1/f.bin", nil)
	req.Header.Set("x-ms-content-length", "16")
	if err := handleAzureFilesPath(rec, req, hostPath, "", ""); err != nil {
		t.Fatalf("create: %v", err)
	}

	// Write 4 bytes at offset 4
	rec = httptest.NewRecorder()
	req = httptest.NewRequest(http.MethodPut, "/share-1/f.bin?comp=range", bytes.NewReader([]byte("ABCD")))
	req.Header.Set("Content-Range", "bytes=4-7/*")
	if err := handleAzureFilesPath(rec, req, hostPath, "", "range"); err != nil {
		t.Fatalf("range write: %v", err)
	}

	got, err := os.ReadFile(hostPath)
	if err != nil {
		t.Fatalf("read back: %v", err)
	}
	if len(got) != 16 {
		t.Errorf("size = %d, want 16", len(got))
	}
	if string(got[4:8]) != "ABCD" {
		t.Errorf("bytes[4:8] = %q, want ABCD", string(got[4:8]))
	}
}

func infoSize(fi os.FileInfo) int64 {
	if fi == nil {
		return -1
	}
	return fi.Size()
}
