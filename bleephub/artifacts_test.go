package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"testing"
)

func TestArtifactCreateUploadFinalize(t *testing.T) {
	s := newTestServer()

	// Create artifact
	body := `{"name":"test-artifact","version":4}`
	req := httptest.NewRequest("POST", "/twirp/github.actions.results.api.v1.ArtifactService/CreateArtifact", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.handleCreateArtifact(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, want 200", w.Code)
	}

	var createResp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &createResp)
	if createResp["ok"] != true {
		t.Error("create: ok should be true")
	}
	uploadURL, _ := createResp["signed_upload_url"].(string)
	if uploadURL == "" {
		t.Fatal("create: empty upload URL")
	}

	// Upload data
	uploadReq := httptest.NewRequest("PUT", "/_apis/v1/artifacts/1/upload", bytes.NewBufferString("hello world"))
	uploadReq.SetPathValue("artifactId", "1")
	uploadW := httptest.NewRecorder()
	s.handleUploadArtifact(uploadW, uploadReq)

	if uploadW.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want 200", uploadW.Code)
	}

	// Finalize
	finBody := `{"name":"test-artifact","size":11}`
	finReq := httptest.NewRequest("POST", "/twirp/github.actions.results.api.v1.ArtifactService/FinalizeArtifact", bytes.NewBufferString(finBody))
	finW := httptest.NewRecorder()
	s.handleFinalizeArtifact(finW, finReq)

	if finW.Code != http.StatusOK {
		t.Fatalf("finalize status = %d, want 200", finW.Code)
	}

	var finResp map[string]interface{}
	json.Unmarshal(finW.Body.Bytes(), &finResp)
	if finResp["ok"] != true {
		t.Error("finalize: ok should be true")
	}
}

func TestArtifactListReturnsFinalized(t *testing.T) {
	s := newTestServer()

	// Create and finalize an artifact
	s.artifactStore.mu.Lock()
	s.artifactStore.artifacts[1] = &Artifact{ID: 1, Name: "my-artifact", Size: 100, Finalized: true}
	s.artifactStore.artifacts[2] = &Artifact{ID: 2, Name: "unfinished", Size: 50, Finalized: false}
	s.artifactStore.mu.Unlock()

	req := httptest.NewRequest("POST", "/twirp/github.actions.results.api.v1.ArtifactService/ListArtifacts", bytes.NewBufferString("{}"))
	w := httptest.NewRecorder()
	s.handleListArtifacts(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	arts := resp["artifacts"].([]interface{})
	if len(arts) != 1 {
		t.Errorf("listed %d artifacts, want 1 (only finalized)", len(arts))
	}
}

func TestArtifactDownload(t *testing.T) {
	s := newTestServer()

	// Create finalized artifact with data
	s.artifactStore.mu.Lock()
	s.artifactStore.artifacts[1] = &Artifact{
		ID:        1,
		Name:      "my-artifact",
		Data:      []byte("artifact-data"),
		Size:      13,
		Finalized: true,
	}
	s.artifactStore.mu.Unlock()

	req := httptest.NewRequest("GET", "/_apis/v1/artifacts/1/download", nil)
	req.SetPathValue("artifactId", "1")
	w := httptest.NewRecorder()
	s.handleDownloadArtifact(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200", w.Code)
	}
	if w.Body.String() != "artifact-data" {
		t.Errorf("body = %q, want artifact-data", w.Body.String())
	}
}

func TestArtifactDownloadNotFound(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("GET", "/_apis/v1/artifacts/999/download", nil)
	req.SetPathValue("artifactId", "999")
	w := httptest.NewRecorder()
	s.handleDownloadArtifact(w, req)

	if w.Code != http.StatusNotFound {
		t.Errorf("status = %d, want 404", w.Code)
	}
}

func TestCacheLookupReturns204(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("GET", "/_apis/artifactcache/cache?keys=test-key&version=abc", nil)
	w := httptest.NewRecorder()
	s.handleCacheLookup(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("cache lookup status = %d, want 204", w.Code)
	}
}

func TestCacheReserveReturns204(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("POST", "/_apis/artifactcache/cache", bytes.NewBufferString(`{"key":"test","version":"abc"}`))
	w := httptest.NewRecorder()
	s.handleCacheReserve(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("cache reserve status = %d, want 204", w.Code)
	}
}

func TestGetSignedArtifactURL(t *testing.T) {
	s := newTestServer()

	s.artifactStore.mu.Lock()
	s.artifactStore.artifacts[1] = &Artifact{
		ID:        1,
		Name:      "my-artifact",
		Finalized: true,
	}
	s.artifactStore.mu.Unlock()

	body := `{"name":"my-artifact"}`
	req := httptest.NewRequest("POST", "/twirp/github.actions.results.api.v1.ArtifactService/GetSignedArtifactURL", bytes.NewBufferString(body))
	w := httptest.NewRecorder()
	s.handleGetSignedArtifactURL(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("status = %d, want 200", w.Code)
	}

	var resp map[string]interface{}
	json.Unmarshal(w.Body.Bytes(), &resp)
	url, _ := resp["signed_url"].(string)
	if url == "" {
		t.Error("signed_url is empty")
	}

	_ = fmt.Sprintf("url: %s", url) // use fmt to avoid import error
}
