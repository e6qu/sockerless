package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"testing"
	"time"
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

func TestCacheRoundTrip(t *testing.T) {
	s := newTestServer()

	reserveReq := httptest.NewRequest("POST", "/_apis/artifactcache/cache", bytes.NewBufferString(`{"key":"linux-go-main","version":"abc"}`))
	reserveW := httptest.NewRecorder()
	s.handleCacheReserve(reserveW, reserveReq)

	if reserveW.Code != http.StatusOK {
		t.Fatalf("reserve status = %d, want 200; body=%s", reserveW.Code, reserveW.Body.String())
	}
	var reserveResp struct {
		CacheID int64 `json:"cacheId"`
	}
	if err := json.Unmarshal(reserveW.Body.Bytes(), &reserveResp); err != nil {
		t.Fatalf("decode reserve: %v", err)
	}
	if reserveResp.CacheID == 0 {
		t.Fatal("reserve returned cacheId=0")
	}

	uploadReq := httptest.NewRequest("PATCH", fmt.Sprintf("/_apis/artifactcache/caches/%d", reserveResp.CacheID), bytes.NewBufferString("cache-data"))
	uploadReq.SetPathValue("cacheId", strconv.FormatInt(reserveResp.CacheID, 10))
	uploadW := httptest.NewRecorder()
	s.handleCacheUpload(uploadW, uploadReq)
	if uploadW.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want 200; body=%s", uploadW.Code, uploadW.Body.String())
	}

	finalizeReq := httptest.NewRequest("POST", fmt.Sprintf("/_apis/artifactcache/caches/%d", reserveResp.CacheID), bytes.NewBufferString(`{"size":10}`))
	finalizeReq.SetPathValue("cacheId", strconv.FormatInt(reserveResp.CacheID, 10))
	finalizeW := httptest.NewRecorder()
	s.handleCacheFinalize(finalizeW, finalizeReq)
	if finalizeW.Code != http.StatusOK {
		t.Fatalf("finalize status = %d, want 200; body=%s", finalizeW.Code, finalizeW.Body.String())
	}

	req := httptest.NewRequest("GET", "/_apis/artifactcache/cache?keys=linux-go-main&version=abc", nil)
	w := httptest.NewRecorder()
	s.handleCacheLookup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var lookupResp struct {
		ArchiveLocation string `json:"archiveLocation"`
		CacheKey        string `json:"cacheKey"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &lookupResp); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if lookupResp.CacheKey != "linux-go-main" {
		t.Fatalf("lookup cacheKey = %q, want linux-go-main", lookupResp.CacheKey)
	}
	if lookupResp.ArchiveLocation == "" {
		t.Fatal("lookup archiveLocation is empty")
	}

	downloadReq := httptest.NewRequest("GET", fmt.Sprintf("/_apis/artifactcache/caches/%d", reserveResp.CacheID), nil)
	downloadReq.SetPathValue("cacheId", strconv.FormatInt(reserveResp.CacheID, 10))
	downloadW := httptest.NewRecorder()
	s.handleCacheDownload(downloadW, downloadReq)
	if downloadW.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200; body=%s", downloadW.Code, downloadW.Body.String())
	}
	if downloadW.Body.String() != "cache-data" {
		t.Fatalf("download body = %q, want cache-data", downloadW.Body.String())
	}
}

func TestCacheLookupMissReturns204(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("GET", "/_apis/artifactcache/cache?keys=test-key&version=abc", nil)
	w := httptest.NewRecorder()
	s.handleCacheLookup(w, req)

	if w.Code != http.StatusNoContent {
		t.Errorf("cache lookup status = %d, want 204", w.Code)
	}
}

func TestCacheRestoreKeyUsesNewestPrefixMatch(t *testing.T) {
	s := newTestServer()

	old := &CacheEntry{ID: 1, Key: "linux-go-old", Version: "abc", Data: []byte("old"), Finalized: true, CreatedAt: time.Now().Add(-time.Hour)}
	newer := &CacheEntry{ID: 2, Key: "linux-go-main", Version: "abc", Data: []byte("new"), Finalized: true, CreatedAt: time.Now()}
	s.artifactStore.mu.Lock()
	s.artifactStore.caches[old.ID] = old
	s.artifactStore.caches[newer.ID] = newer
	s.artifactStore.cacheIndex[cacheLookupKey(old.Key, old.Version)] = old.ID
	s.artifactStore.cacheIndex[cacheLookupKey(newer.Key, newer.Version)] = newer.ID
	s.artifactStore.mu.Unlock()

	req := httptest.NewRequest("GET", "/_apis/artifactcache/cache?keys=linux-go-missing,linux-go-&version=abc", nil)
	w := httptest.NewRecorder()
	s.handleCacheLookup(w, req)

	if w.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		CacheKey string `json:"cacheKey"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if resp.CacheKey != "linux-go-main" {
		t.Fatalf("cacheKey = %q, want newest prefix match linux-go-main", resp.CacheKey)
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
