package bleephub

import (
	"bytes"
	"encoding/json"
	"fmt"
	"net/http"
	"net/http/httptest"
	"strconv"
	"strings"
	"sync"
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

func TestArtifactUploadWritesObjectStore(t *testing.T) {
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: fs.bucket, prefix: "objects"}
	s := newTestServer()
	s.artifactStore = NewArtifactStoreWithByteStore("", &s3ActionsByteStore{fs: objectFS})

	req := httptest.NewRequest("POST", "/twirp/github.actions.results.api.v1.ArtifactService/CreateArtifact", bytes.NewBufferString(`{"name":"object-artifact","version":4}`))
	w := httptest.NewRecorder()
	s.handleCreateArtifact(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("create status = %d, body=%s", w.Code, w.Body.String())
	}

	uploadReq := httptest.NewRequest("PUT", "/_apis/v1/artifacts/1/upload", bytes.NewBufferString("object-backed artifact"))
	uploadReq.SetPathValue("artifactId", "1")
	uploadW := httptest.NewRecorder()
	s.handleUploadArtifact(uploadW, uploadReq)
	if uploadW.Code != http.StatusOK {
		t.Fatalf("upload status = %d, body=%s", uploadW.Code, uploadW.Body.String())
	}

	got := readS3TestFile(t, objectFS, "actions/artifacts/1/data")
	if string(got) != "object-backed artifact" {
		t.Fatalf("s3 artifact data = %q", string(got))
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

func TestArtifactFinalizeScopesByWorkflowRunBackendID(t *testing.T) {
	s := newTestServer()

	s.artifactStore.mu.Lock()
	s.artifactStore.artifacts[1] = &Artifact{ID: 1, Name: "shared", WorkflowRunBackendID: "run-a"}
	s.artifactStore.artifacts[2] = &Artifact{ID: 2, Name: "shared", WorkflowRunBackendID: "run-b"}
	s.artifactStore.mu.Unlock()

	req := httptest.NewRequest("POST",
		"/twirp/github.actions.results.api.v1.ArtifactService/FinalizeArtifact",
		bytes.NewBufferString(`{"name":"shared","size":0,"workflow_run_backend_id":"run-b"}`))
	w := httptest.NewRecorder()
	s.handleFinalizeArtifact(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("finalize status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		ArtifactID int64 `json:"artifact_id"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode finalize: %v", err)
	}
	if resp.ArtifactID != 2 {
		t.Fatalf("artifact_id = %d, want run-b artifact 2", resp.ArtifactID)
	}

	s.artifactStore.mu.RLock()
	runA := s.artifactStore.artifacts[1].Finalized
	runB := s.artifactStore.artifacts[2].Finalized
	s.artifactStore.mu.RUnlock()
	if runA || !runB {
		t.Fatalf("finalized flags: run-a=%v run-b=%v, want false/true", runA, runB)
	}
}

func TestGetSignedArtifactURLScopesByWorkflowRunBackendID(t *testing.T) {
	s := newTestServer()

	s.artifactStore.mu.Lock()
	s.artifactStore.artifacts[1] = &Artifact{ID: 1, Name: "shared", WorkflowRunBackendID: "run-a", Finalized: true}
	s.artifactStore.artifacts[2] = &Artifact{ID: 2, Name: "shared", WorkflowRunBackendID: "run-b", Finalized: true}
	s.artifactStore.mu.Unlock()

	req := httptest.NewRequest("POST",
		"/twirp/github.actions.results.api.v1.ArtifactService/GetSignedArtifactURL",
		bytes.NewBufferString(`{"name":"shared","workflowRunBackendId":"run-b"}`))
	w := httptest.NewRecorder()
	s.handleGetSignedArtifactURL(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("signed URL status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		SignedURL string `json:"signed_url"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode signed URL: %v", err)
	}
	if !strings.Contains(resp.SignedURL, "/_apis/v1/artifacts/2/download") {
		t.Fatalf("signed_url = %q, want run-b artifact 2 download URL", resp.SignedURL)
	}
}

func TestCacheUploadWritesObjectStore(t *testing.T) {
	fs := newS3FSForTest(t)
	objectFS := &s3FS{client: fs.client, bucket: fs.bucket, prefix: "objects"}
	store := NewArtifactStoreWithByteStore("", &s3ActionsByteStore{fs: objectFS})
	entry := &CacheEntry{ID: 7, Repo: "octo/repo", Key: "linux-go", Version: "v1"}
	entry.Data = []byte("cache archive bytes")

	if err := store.writeCacheDataAt(entry, entry.Data, 0); err != nil {
		t.Fatalf("writeCacheDataAt: %v", err)
	}
	got := readS3TestFile(t, objectFS, "actions/caches/7/data")
	if string(got) != "cache archive bytes" {
		t.Fatalf("s3 cache data = %q", string(got))
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

// seedCacheRunJob registers a dispatched job for repo and returns the runtime
// token the @actions/cache toolkit would send on cache requests: the
// SystemVssConnection AccessToken, whose sub claim is the plan
// scopeIdentifier recorded in the job message.
func seedCacheRunJob(t *testing.T, s *Server, repo string) string {
	t.Helper()
	scopeID := fmt.Sprintf("scope-%s-%d", repo, time.Now().UnixNano())
	msg := map[string]interface{}{
		"plan": map[string]interface{}{
			"scopeIdentifier": scopeID,
		},
		"contextData": map[string]interface{}{
			"github": dictContextData("repository", repo),
		},
	}
	msgJSON, err := json.Marshal(msg)
	if err != nil {
		t.Fatalf("marshal job message: %v", err)
	}
	s.store.mu.Lock()
	s.store.Jobs[scopeID] = &Job{ID: scopeID, Status: "running", Message: string(msgJSON)}
	s.store.mu.Unlock()
	return makeJWT(scopeID, "actions")
}

func cacheReserve(t *testing.T, s *Server, token, key, version string) int64 {
	t.Helper()
	body := fmt.Sprintf(`{"key":%q,"version":%q}`, key, version)
	req := httptest.NewRequest("POST", "/_apis/artifactcache/caches", bytes.NewBufferString(body))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.handleCacheReserve(w, req)
	if w.Code != http.StatusOK {
		t.Fatalf("reserve status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		CacheID int64 `json:"cacheId"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode reserve: %v", err)
	}
	if resp.CacheID == 0 {
		t.Fatal("reserve returned cacheId=0")
	}
	return resp.CacheID
}

func cacheUploadChunk(t *testing.T, s *Server, token string, cacheID, start int64, chunk []byte) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("PATCH", fmt.Sprintf("/_apis/artifactcache/caches/%d", cacheID), bytes.NewReader(chunk))
	req.SetPathValue("cacheId", strconv.FormatInt(cacheID, 10))
	req.Header.Set("Authorization", "Bearer "+token)
	req.Header.Set("Content-Range", fmt.Sprintf("bytes %d-%d/*", start, start+int64(len(chunk))-1))
	w := httptest.NewRecorder()
	s.handleCacheUpload(w, req)
	return w
}

func cacheFinalize(t *testing.T, s *Server, token string, cacheID, size int64) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("POST", fmt.Sprintf("/_apis/artifactcache/caches/%d", cacheID), bytes.NewBufferString(fmt.Sprintf(`{"size":%d}`, size)))
	req.SetPathValue("cacheId", strconv.FormatInt(cacheID, 10))
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.handleCacheFinalize(w, req)
	return w
}

// cacheDownload fetches an archiveLocation URL the way the cache toolkit
// does: a plain GET with no Authorization header.
func cacheDownload(t *testing.T, s *Server, archiveLocation string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", archiveLocation, nil)
	segments := strings.Split(req.URL.Path, "/")
	req.SetPathValue("cacheId", segments[len(segments)-1])
	w := httptest.NewRecorder()
	s.handleCacheDownload(w, req)
	return w
}

// cacheArchiveLocation looks up a finalized cache and returns its
// archiveLocation, fatal on miss.
func cacheArchiveLocation(t *testing.T, s *Server, token, keys, version string) string {
	t.Helper()
	w := cacheLookup(t, s, token, keys, version)
	if w.Code != http.StatusOK {
		t.Fatalf("lookup status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		ArchiveLocation string `json:"archiveLocation"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode lookup: %v", err)
	}
	if resp.ArchiveLocation == "" {
		t.Fatal("lookup archiveLocation is empty")
	}
	return resp.ArchiveLocation
}

func cacheLookup(t *testing.T, s *Server, token, keys, version string) *httptest.ResponseRecorder {
	t.Helper()
	req := httptest.NewRequest("GET", fmt.Sprintf("/_apis/artifactcache/cache?keys=%s&version=%s", keys, version), nil)
	req.Header.Set("Authorization", "Bearer "+token)
	w := httptest.NewRecorder()
	s.handleCacheLookup(w, req)
	return w
}

func seedFinalizedCache(s *Server, id int64, repo, key, version string, size int64) {
	s.artifactStore.mu.Lock()
	entry := &CacheEntry{
		ID: id, Repo: repo, Key: key, Version: version,
		Size: size, Data: make([]byte, size), Finalized: true, CreatedAt: time.Now(),
	}
	s.artifactStore.caches[id] = entry
	s.artifactStore.cacheIndex[cacheLookupKey(repo, key, version)] = id
	if id >= s.artifactStore.nextCacheID {
		s.artifactStore.nextCacheID = id + 1
	}
	s.artifactStore.mu.Unlock()
}

func TestRepoCaches_ListAndUsage(t *testing.T) {
	s := newTestServer()
	s.registerArtifactRoutes()
	seedFinalizedCache(s, 1, "octo/repo", "linux-go-main", "abc", 100)
	seedFinalizedCache(s, 2, "octo/repo", "linux-node", "def", 50)
	seedFinalizedCache(s, 3, "other/repo", "x", "y", 999)

	w := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/caches")
	if w.Code != http.StatusOK {
		t.Fatalf("list status = %d, body = %s", w.Code, w.Body.String())
	}
	var list struct {
		TotalCount    int `json:"total_count"`
		ActionsCaches []struct {
			ID          int64  `json:"id"`
			Key         string `json:"key"`
			Version     string `json:"version"`
			Ref         string `json:"ref"`
			SizeInBytes int64  `json:"size_in_bytes"`
			CreatedAt   string `json:"created_at"`
			LastAccess  string `json:"last_accessed_at"`
		} `json:"actions_caches"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &list); err != nil {
		t.Fatalf("decode list: %v", err)
	}
	if list.TotalCount != 2 || len(list.ActionsCaches) != 2 {
		t.Fatalf("list count = %d/%d, want 2 (other/repo filtered out)", list.TotalCount, len(list.ActionsCaches))
	}
	if list.ActionsCaches[0].Key != "linux-go-main" || list.ActionsCaches[0].SizeInBytes != 100 {
		t.Errorf("cache[0] = %+v", list.ActionsCaches[0])
	}
	if list.ActionsCaches[0].Ref == "" || list.ActionsCaches[0].LastAccess == "" {
		t.Errorf("cache[0] missing ref/last_accessed_at: %+v", list.ActionsCaches[0])
	}

	// key filter
	wf := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/caches?key=linux-node")
	var filtered struct {
		TotalCount int `json:"total_count"`
	}
	json.Unmarshal(wf.Body.Bytes(), &filtered)
	if filtered.TotalCount != 1 {
		t.Errorf("key filter total_count = %d, want 1", filtered.TotalCount)
	}

	usage := runRequest(s, "GET", "/api/v3/repos/octo/repo/actions/cache/usage")
	if usage.Code != http.StatusOK {
		t.Fatalf("usage status = %d", usage.Code)
	}
	var u struct {
		FullName string `json:"full_name"`
		Size     int64  `json:"active_caches_size_in_bytes"`
		Count    int    `json:"active_caches_count"`
	}
	json.Unmarshal(usage.Body.Bytes(), &u)
	if u.FullName != "octo/repo" || u.Size != 150 || u.Count != 2 {
		t.Errorf("usage = %+v, want full_name=octo/repo size=150 count=2", u)
	}
}

func TestRepoCaches_DeleteByID(t *testing.T) {
	s := newTestServer()
	s.registerArtifactRoutes()
	seedFinalizedCache(s, 1, "octo/repo", "k", "v", 10)

	w := runAuthedRequest(s, "DELETE", "/api/v3/repos/octo/repo/actions/caches/1")
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete status = %d, want 204; body=%s", w.Code, w.Body.String())
	}
	s.artifactStore.mu.RLock()
	_, exists := s.artifactStore.caches[1]
	s.artifactStore.mu.RUnlock()
	if exists {
		t.Error("cache 1 should be deleted")
	}

	missing := runAuthedRequest(s, "DELETE", "/api/v3/repos/octo/repo/actions/caches/999")
	if missing.Code != http.StatusNotFound {
		t.Errorf("delete missing status = %d, want 404", missing.Code)
	}
}

func TestRepoCaches_DeleteByKey(t *testing.T) {
	s := newTestServer()
	s.registerArtifactRoutes()
	seedFinalizedCache(s, 1, "octo/repo", "shared", "v1", 10)
	seedFinalizedCache(s, 2, "octo/repo", "shared", "v2", 20)
	seedFinalizedCache(s, 3, "octo/repo", "other", "v1", 5)

	w := runAuthedRequest(s, "DELETE", "/api/v3/repos/octo/repo/actions/caches?key=shared")
	if w.Code != http.StatusOK {
		t.Fatalf("delete-by-key status = %d, body=%s", w.Code, w.Body.String())
	}
	var resp struct {
		TotalCount    int `json:"total_count"`
		ActionsCaches []struct {
			Key string `json:"key"`
		} `json:"actions_caches"`
	}
	if err := json.Unmarshal(w.Body.Bytes(), &resp); err != nil {
		t.Fatalf("decode: %v", err)
	}
	if resp.TotalCount != 2 || len(resp.ActionsCaches) != 2 {
		t.Fatalf("deleted = %d, want 2", resp.TotalCount)
	}
	s.artifactStore.mu.RLock()
	remaining := len(s.artifactStore.caches)
	s.artifactStore.mu.RUnlock()
	if remaining != 1 {
		t.Errorf("remaining caches = %d, want 1 (the 'other' key)", remaining)
	}
}

func TestCacheRoundTrip(t *testing.T) {
	s := newTestServer()
	token := seedCacheRunJob(t, s, "octo/round-trip")

	cacheID := cacheReserve(t, s, token, "linux-go-main", "abc")

	if w := cacheUploadChunk(t, s, token, cacheID, 0, []byte("cache-data")); w.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if w := cacheFinalize(t, s, token, cacheID, 10); w.Code != http.StatusOK {
		t.Fatalf("finalize status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	w := cacheLookup(t, s, token, "linux-go-main", "abc")
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

	downloadW := cacheDownload(t, s, lookupResp.ArchiveLocation)
	if downloadW.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200; body=%s", downloadW.Code, downloadW.Body.String())
	}
	if downloadW.Body.String() != "cache-data" {
		t.Fatalf("download body = %q, want cache-data", downloadW.Body.String())
	}

	// Without the sig the lookup handed out, the archive URL must not serve.
	bare := strings.SplitN(lookupResp.ArchiveLocation, "?", 2)[0]
	if w := cacheDownload(t, s, bare); w.Code != http.StatusNotFound {
		t.Fatalf("download without sig status = %d, want 404", w.Code)
	}
	if w := cacheDownload(t, s, bare+"?sig=forged"); w.Code != http.StatusNotFound {
		t.Fatalf("download with forged sig status = %d, want 404", w.Code)
	}
}

func TestCacheLookupMissReturns204(t *testing.T) {
	s := newTestServer()
	token := seedCacheRunJob(t, s, "octo/miss")

	w := cacheLookup(t, s, token, "test-key", "abc")
	if w.Code != http.StatusNoContent {
		t.Errorf("cache lookup status = %d, want 204", w.Code)
	}
}

func TestCacheRequiresRuntimeToken(t *testing.T) {
	s := newTestServer()

	req := httptest.NewRequest("POST", "/_apis/artifactcache/caches", bytes.NewBufferString(`{"key":"k","version":"v"}`))
	w := httptest.NewRecorder()
	s.handleCacheReserve(w, req)
	if w.Code != http.StatusUnauthorized {
		t.Errorf("reserve without token status = %d, want 401", w.Code)
	}

	lookupReq := httptest.NewRequest("GET", "/_apis/artifactcache/cache?keys=k&version=v", nil)
	lookupReq.Header.Set("Authorization", "Bearer not-a-jwt")
	lookupW := httptest.NewRecorder()
	s.handleCacheLookup(lookupW, lookupReq)
	if lookupW.Code != http.StatusUnauthorized {
		t.Errorf("lookup with bogus token status = %d, want 401", lookupW.Code)
	}
}

func TestCacheRestoreKeyUsesNewestPrefixMatch(t *testing.T) {
	s := newTestServer()
	token := seedCacheRunJob(t, s, "octo/prefix")

	old := &CacheEntry{ID: 1, Repo: "octo/prefix", Key: "linux-go-old", Version: "abc", Data: []byte("old"), Finalized: true, CreatedAt: time.Now().Add(-time.Hour)}
	newer := &CacheEntry{ID: 2, Repo: "octo/prefix", Key: "linux-go-main", Version: "abc", Data: []byte("new"), Finalized: true, CreatedAt: time.Now()}
	s.artifactStore.mu.Lock()
	s.artifactStore.caches[old.ID] = old
	s.artifactStore.caches[newer.ID] = newer
	s.artifactStore.cacheIndex[cacheLookupKey(old.Repo, old.Key, old.Version)] = old.ID
	s.artifactStore.cacheIndex[cacheLookupKey(newer.Repo, newer.Key, newer.Version)] = newer.ID
	s.artifactStore.mu.Unlock()

	w := cacheLookup(t, s, token, "linux-go-missing,linux-go-", "abc")
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

func TestCacheUploadOutOfOrderChunks(t *testing.T) {
	s := newTestServer()
	token := seedCacheRunJob(t, s, "octo/out-of-order")

	cacheID := cacheReserve(t, s, token, "linux-go-ooo", "v1")

	// Reverse order with an overlapping boundary: the second chunk rewrites
	// byte 5 with the same value the first chunk put there.
	want := []byte("HELLOWORLD")
	if w := cacheUploadChunk(t, s, token, cacheID, 5, []byte("WORLD")); w.Code != http.StatusOK {
		t.Fatalf("chunk 5-9 status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if w := cacheUploadChunk(t, s, token, cacheID, 0, []byte("HELLOW")); w.Code != http.StatusOK {
		t.Fatalf("chunk 0-5 status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	if w := cacheFinalize(t, s, token, cacheID, int64(len(want))); w.Code != http.StatusOK {
		t.Fatalf("finalize status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	downloadW := cacheDownload(t, s, cacheArchiveLocation(t, s, token, "linux-go-ooo", "v1"))
	if downloadW.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200; body=%s", downloadW.Code, downloadW.Body.String())
	}
	if !bytes.Equal(downloadW.Body.Bytes(), want) {
		t.Fatalf("download body = %q, want %q", downloadW.Body.String(), want)
	}
}

func TestCacheUploadConcurrentChunks(t *testing.T) {
	s := newTestServer()
	token := seedCacheRunJob(t, s, "octo/concurrent")

	cacheID := cacheReserve(t, s, token, "linux-go-concurrent", "v1")

	// Deterministic 64 KiB archive split into 4 chunks uploaded concurrently,
	// matching the toolkit's concurrent ranged PATCHes.
	const chunkSize = 16 * 1024
	const chunks = 4
	want := make([]byte, chunks*chunkSize)
	for i := range want {
		want[i] = byte(i % 251)
	}

	var wg sync.WaitGroup
	codes := make([]int, chunks)
	for i := 0; i < chunks; i++ {
		wg.Add(1)
		go func(i int) {
			defer wg.Done()
			start := int64(i * chunkSize)
			w := cacheUploadChunk(t, s, token, cacheID, start, want[start:start+chunkSize])
			codes[i] = w.Code
		}(i)
	}
	wg.Wait()
	for i, code := range codes {
		if code != http.StatusOK {
			t.Fatalf("chunk %d status = %d, want 200", i, code)
		}
	}

	if w := cacheFinalize(t, s, token, cacheID, int64(len(want))); w.Code != http.StatusOK {
		t.Fatalf("finalize status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	downloadW := cacheDownload(t, s, cacheArchiveLocation(t, s, token, "linux-go-concurrent", "v1"))
	if downloadW.Code != http.StatusOK {
		t.Fatalf("download status = %d, want 200; body=%s", downloadW.Code, downloadW.Body.String())
	}
	if !bytes.Equal(downloadW.Body.Bytes(), want) {
		t.Fatalf("downloaded %d bytes do not match uploaded archive", downloadW.Body.Len())
	}
}

func TestCacheFinalizeSizeMismatchRejected(t *testing.T) {
	s := newTestServer()
	token := seedCacheRunJob(t, s, "octo/size-mismatch")

	cacheID := cacheReserve(t, s, token, "linux-go-mismatch", "v1")
	if w := cacheUploadChunk(t, s, token, cacheID, 0, []byte("cache-data")); w.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	if w := cacheFinalize(t, s, token, cacheID, 99); w.Code != http.StatusBadRequest {
		t.Fatalf("finalize with wrong size status = %d, want 400; body=%s", w.Code, w.Body.String())
	}

	// The entry must not have been finalized by the rejected request.
	if w := cacheLookup(t, s, token, "linux-go-mismatch", "v1"); w.Code != http.StatusNoContent {
		t.Fatalf("lookup after rejected finalize status = %d, want 204", w.Code)
	}
}

func TestCacheRepoIsolation(t *testing.T) {
	s := newTestServer()
	tokenA := seedCacheRunJob(t, s, "octo/repo-a")
	tokenB := seedCacheRunJob(t, s, "octo/repo-b")

	cacheID := cacheReserve(t, s, tokenA, "linux-go-shared", "v1")
	if w := cacheUploadChunk(t, s, tokenA, cacheID, 0, []byte("repo-a-data")); w.Code != http.StatusOK {
		t.Fatalf("upload status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if w := cacheFinalize(t, s, tokenA, cacheID, 11); w.Code != http.StatusOK {
		t.Fatalf("finalize status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Repo A sees its own entry.
	if w := cacheLookup(t, s, tokenA, "linux-go-shared", "v1"); w.Code != http.StatusOK {
		t.Fatalf("repo A lookup status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Repo B must not see repo A's entry for the same key+version.
	if w := cacheLookup(t, s, tokenB, "linux-go-shared", "v1"); w.Code != http.StatusNoContent {
		t.Fatalf("repo B lookup status = %d, want 204 miss; body=%s", w.Code, w.Body.String())
	}

	// Repo B must not be able to write into repo A's reserved entry.
	if w := cacheUploadChunk(t, s, tokenB, cacheID, 0, []byte("poisoned!!!")); w.Code != http.StatusNotFound {
		t.Fatalf("repo B upload to repo A cache status = %d, want 404; body=%s", w.Code, w.Body.String())
	}

	// Repo B reserving the same key+version gets its own entry, not a conflict
	// with repo A's.
	cacheIDB := cacheReserve(t, s, tokenB, "linux-go-shared", "v1")
	if cacheIDB == cacheID {
		t.Fatalf("repo B reservation reused repo A's cacheId %d", cacheID)
	}
	if w := cacheUploadChunk(t, s, tokenB, cacheIDB, 0, []byte("repo-b-data")); w.Code != http.StatusOK {
		t.Fatalf("repo B upload status = %d, want 200; body=%s", w.Code, w.Body.String())
	}
	if w := cacheFinalize(t, s, tokenB, cacheIDB, 11); w.Code != http.StatusOK {
		t.Fatalf("repo B finalize status = %d, want 200; body=%s", w.Code, w.Body.String())
	}

	// Each repo's lookup hands back its own archive.
	if w := cacheDownload(t, s, cacheArchiveLocation(t, s, tokenA, "linux-go-shared", "v1")); w.Body.String() != "repo-a-data" {
		t.Fatalf("repo A download = %q, want repo-a-data", w.Body.String())
	}
	if w := cacheDownload(t, s, cacheArchiveLocation(t, s, tokenB, "linux-go-shared", "v1")); w.Body.String() != "repo-b-data" {
		t.Fatalf("repo B download = %q, want repo-b-data", w.Body.String())
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
