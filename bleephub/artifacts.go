package bleephub

import (
	"crypto/rand"
	"encoding/hex"
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"sync"
	"time"
)

// ArtifactStore holds artifacts for @actions/artifact v4 Twirp API.
// When dataDir is set, artifacts are persisted to disk; otherwise they're in-memory.
type ArtifactStore struct {
	mu          sync.RWMutex
	artifacts   map[int64]*Artifact
	nextID      int64
	caches      map[int64]*CacheEntry
	cacheIndex  map[string]int64
	nextCacheID int64
	dataDir     string // empty = in-memory mode
}

// Artifact represents an uploaded artifact.
type Artifact struct {
	ID                   int64     `json:"id"`
	Name                 string    `json:"name"`
	Size                 int64     `json:"size"`
	Data                 []byte    `json:"-"`
	Finalized            bool      `json:"finalized"`
	RunID                string    `json:"runId"`
	GitHubRunID          int       `json:"githubRunId"`
	RepoFullName         string    `json:"repoFullName"`
	WorkflowRunBackendID string    `json:"workflowRunBackendId"`
	CreatedAt            time.Time `json:"createdAt"`
}

// CacheEntry represents one immutable Actions dependency cache archive.
// Entries are scoped to the repository whose run created them, mirroring
// GitHub's per-repository cache isolation. DownloadToken plays the role of
// GitHub's pre-signed archive URL: the cache toolkit fetches archiveLocation
// with an unauthenticated client, so the URL itself must be unguessable.
type CacheEntry struct {
	ID            int64     `json:"id"`
	Repo          string    `json:"repo"`
	Key           string    `json:"key"`
	Version       string    `json:"version"`
	Size          int64     `json:"size"`
	Data          []byte    `json:"-"`
	Finalized     bool      `json:"finalized"`
	DownloadToken string    `json:"downloadToken"`
	CreatedAt     time.Time `json:"createdAt"`
}

// NewArtifactStore creates an artifact store. If dataDir is non-empty,
// artifacts are persisted to disk.
func NewArtifactStore(dataDir ...string) *ArtifactStore {
	dir := ""
	if len(dataDir) > 0 {
		dir = dataDir[0]
	}
	store := &ArtifactStore{
		artifacts:   make(map[int64]*Artifact),
		nextID:      1,
		caches:      make(map[int64]*CacheEntry),
		cacheIndex:  make(map[string]int64),
		nextCacheID: 1,
		dataDir:     dir,
	}
	if dir != "" {
		store.recoverFromDisk()
	}
	return store
}

// recoverFromDisk scans the artifacts directory and rebuilds the in-memory map.
func (as *ArtifactStore) recoverFromDisk() {
	as.recoverArtifactsFromDisk()
	as.recoverCachesFromDisk()
}

func (as *ArtifactStore) recoverArtifactsFromDisk() {
	artDir := filepath.Join(as.dataDir, "artifacts")
	entries, err := os.ReadDir(artDir)
	if err != nil {
		return // Directory doesn't exist yet
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := strconv.ParseInt(entry.Name(), 10, 64)
		if err != nil {
			continue
		}
		metaPath := filepath.Join(artDir, entry.Name(), "meta.json")
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var art Artifact
		if err := json.Unmarshal(metaBytes, &art); err != nil {
			continue
		}
		// Load data
		dataPath := filepath.Join(artDir, entry.Name(), "data")
		data, _ := os.ReadFile(dataPath)
		art.Data = data
		as.artifacts[id] = &art
		if id >= as.nextID {
			as.nextID = id + 1
		}
	}
}

func (as *ArtifactStore) recoverCachesFromDisk() {
	cacheDir := filepath.Join(as.dataDir, "caches")
	entries, err := os.ReadDir(cacheDir)
	if err != nil {
		return
	}
	for _, entry := range entries {
		if !entry.IsDir() {
			continue
		}
		id, err := strconv.ParseInt(entry.Name(), 10, 64)
		if err != nil {
			continue
		}
		metaPath := filepath.Join(cacheDir, entry.Name(), "meta.json")
		metaBytes, err := os.ReadFile(metaPath)
		if err != nil {
			continue
		}
		var cacheEntry CacheEntry
		if err := json.Unmarshal(metaBytes, &cacheEntry); err != nil {
			continue
		}
		dataPath := filepath.Join(cacheDir, entry.Name(), "data")
		data, _ := os.ReadFile(dataPath)
		cacheEntry.Data = data
		as.caches[id] = &cacheEntry
		as.cacheIndex[cacheLookupKey(cacheEntry.Repo, cacheEntry.Key, cacheEntry.Version)] = id
		if id >= as.nextCacheID {
			as.nextCacheID = id + 1
		}
	}
}

// persistMeta writes artifact metadata to disk.
func (as *ArtifactStore) persistMeta(art *Artifact) {
	if as.dataDir == "" {
		return
	}
	dir := filepath.Join(as.dataDir, "artifacts", strconv.FormatInt(art.ID, 10))
	os.MkdirAll(dir, 0o755)
	data, err := json.Marshal(art)
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o644)
}

// appendData appends data to the artifact's data file.
func (as *ArtifactStore) appendData(art *Artifact, chunk []byte) {
	if as.dataDir == "" {
		return
	}
	dir := filepath.Join(as.dataDir, "artifacts", strconv.FormatInt(art.ID, 10))
	os.MkdirAll(dir, 0o755)
	f, err := os.OpenFile(filepath.Join(dir, "data"), os.O_CREATE|os.O_APPEND|os.O_WRONLY, 0o644)
	if err != nil {
		return
	}
	f.Write(chunk)
	f.Close()
}

func (as *ArtifactStore) finalizedArtifacts() []*Artifact {
	as.mu.RLock()
	defer as.mu.RUnlock()

	out := make([]*Artifact, 0, len(as.artifacts))
	for _, art := range as.artifacts {
		if !art.Finalized {
			continue
		}
		copyArt := *art
		copyArt.Data = append([]byte(nil), art.Data...)
		out = append(out, &copyArt)
	}
	return out
}

func (as *ArtifactStore) artifactByID(id int64) (*Artifact, bool) {
	as.mu.RLock()
	defer as.mu.RUnlock()

	art, ok := as.artifacts[id]
	if !ok || !art.Finalized {
		return nil, false
	}
	copyArt := *art
	copyArt.Data = append([]byte(nil), art.Data...)
	return &copyArt, true
}

func (as *ArtifactStore) deleteArtifact(id int64) bool {
	as.mu.Lock()
	_, ok := as.artifacts[id]
	if ok {
		delete(as.artifacts, id)
	}
	as.mu.Unlock()

	if ok && as.dataDir != "" {
		_ = os.RemoveAll(filepath.Join(as.dataDir, "artifacts", strconv.FormatInt(id, 10)))
	}
	return ok
}

func (as *ArtifactStore) persistCacheMeta(entry *CacheEntry) {
	if as.dataDir == "" {
		return
	}
	dir := filepath.Join(as.dataDir, "caches", strconv.FormatInt(entry.ID, 10))
	os.MkdirAll(dir, 0o755)
	data, err := json.Marshal(entry)
	if err != nil {
		return
	}
	os.WriteFile(filepath.Join(dir, "meta.json"), data, 0o644)
}

// writeCacheDataAt writes a ranged chunk to the cache's data file at the
// offset its Content-Range declared, so out-of-order chunks land in place.
// The in-memory entry.Data is authoritative for this process; this is the
// on-disk copy used for restart recovery, so the error is returned for the
// caller to surface rather than silently dropped.
func (as *ArtifactStore) writeCacheDataAt(entry *CacheEntry, chunk []byte, offset int64) error {
	if as.dataDir == "" {
		return nil
	}
	dir := filepath.Join(as.dataDir, "caches", strconv.FormatInt(entry.ID, 10))
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return err
	}
	f, err := os.OpenFile(filepath.Join(dir, "data"), os.O_CREATE|os.O_WRONLY, 0o644)
	if err != nil {
		return err
	}
	defer f.Close()
	if _, err := f.WriteAt(chunk, offset); err != nil {
		return err
	}
	return nil
}

func (s *Server) registerArtifactRoutes() {
	// Twirp-style artifact service (JSON over HTTP, @actions/artifact v4)
	s.mux.HandleFunc("POST /twirp/github.actions.results.api.v1.ArtifactService/CreateArtifact", s.handleCreateArtifact)
	s.mux.HandleFunc("POST /twirp/github.actions.results.api.v1.ArtifactService/FinalizeArtifact", s.handleFinalizeArtifact)
	s.mux.HandleFunc("POST /twirp/github.actions.results.api.v1.ArtifactService/ListArtifacts", s.handleListArtifacts)
	s.mux.HandleFunc("POST /twirp/github.actions.results.api.v1.ArtifactService/GetSignedArtifactURL", s.handleGetSignedArtifactURL)

	// Artifact upload/download blob endpoints
	s.mux.HandleFunc("PUT /_apis/v1/artifacts/{artifactId}/upload", s.handleUploadArtifact)
	s.mux.HandleFunc("GET /_apis/v1/artifacts/{artifactId}/download", s.handleDownloadArtifact)

	// Actions cache API used by actions/cache. The @actions/cache toolkit
	// reserves at the plural `caches` path (getCacheApiUrl('caches')) and
	// looks up at the singular `cache?keys=`.
	s.mux.HandleFunc("POST /_apis/artifactcache/caches", s.handleCacheReserve)
	s.mux.HandleFunc("GET /_apis/artifactcache/cache", s.handleCacheLookup)
	s.mux.HandleFunc("PATCH /_apis/artifactcache/caches/{cacheId}", s.handleCacheUpload)
	s.mux.HandleFunc("POST /_apis/artifactcache/caches/{cacheId}", s.handleCacheFinalize)
	s.mux.HandleFunc("GET /_apis/artifactcache/caches/{cacheId}", s.handleCacheDownload)
}

// --- Artifact Twirp handlers ---

func (s *Server) handleCreateArtifact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkflowRunBackendID string `json:"workflow_run_backend_id"`
		Name                 string `json:"name"`
		Version              int    `json:"version"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	wf := s.findWorkflowByBackendID(req.WorkflowRunBackendID)
	repoFullName := ""
	githubRunID := 0
	if wf != nil {
		repoFullName = wf.RepoFullName
		githubRunID = wf.RunID
	}

	s.artifactStore.mu.Lock()
	id := s.artifactStore.nextID
	s.artifactStore.nextID++
	art := &Artifact{
		ID:                   id,
		Name:                 req.Name,
		RunID:                req.WorkflowRunBackendID,
		GitHubRunID:          githubRunID,
		RepoFullName:         repoFullName,
		WorkflowRunBackendID: req.WorkflowRunBackendID,
		CreatedAt:            time.Now(),
	}
	s.artifactStore.artifacts[id] = art
	s.artifactStore.persistMeta(art)
	s.artifactStore.mu.Unlock()

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	uploadURL := fmt.Sprintf("%s://%s/_apis/v1/artifacts/%d/upload", scheme, r.Host, id)

	s.logger.Debug().Str("name", req.Name).Int64("id", id).Msg("artifact created")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":                true,
		"signed_upload_url": uploadURL,
	})
}

func (s *Server) handleUploadArtifact(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("artifactId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid artifact ID", http.StatusBadRequest)
		return
	}

	data, err := io.ReadAll(r.Body)
	if err != nil {
		http.Error(w, "read body: "+err.Error(), http.StatusInternalServerError)
		return
	}

	s.artifactStore.mu.Lock()
	art, ok := s.artifactStore.artifacts[id]
	if ok {
		art.Data = append(art.Data, data...)
		art.Size = int64(len(art.Data))
		s.artifactStore.appendData(art, data)
	}
	s.artifactStore.mu.Unlock()

	if !ok {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	s.logger.Debug().Int64("id", id).Int("bytes", len(data)).Msg("artifact chunk uploaded")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleFinalizeArtifact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
		Size int64  `json:"size"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	s.artifactStore.mu.Lock()
	var found *Artifact
	for _, art := range s.artifactStore.artifacts {
		if art.Name == req.Name && !art.Finalized {
			art.Finalized = true
			found = art
			s.artifactStore.persistMeta(art)
			break
		}
	}
	s.artifactStore.mu.Unlock()

	if found == nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	s.logger.Debug().Str("name", req.Name).Int64("id", found.ID).Msg("artifact finalized")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":          true,
		"artifact_id": found.ID,
	})
}

func (s *Server) handleListArtifacts(w http.ResponseWriter, r *http.Request) {
	s.artifactStore.mu.RLock()
	var list []map[string]interface{}
	for _, art := range s.artifactStore.artifacts {
		if art.Finalized {
			list = append(list, map[string]interface{}{
				"name":        art.Name,
				"id":          art.ID,
				"size":        art.Size,
				"created_at":  art.CreatedAt.UTC().Format(time.RFC3339),
				"database_id": art.ID,
			})
		}
	}
	s.artifactStore.mu.RUnlock()

	if list == nil {
		list = []map[string]interface{}{}
	}

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"artifacts": list,
	})
}

func (s *Server) handleGetSignedArtifactURL(w http.ResponseWriter, r *http.Request) {
	var req struct {
		Name string `json:"name"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}

	s.artifactStore.mu.RLock()
	var found *Artifact
	for _, art := range s.artifactStore.artifacts {
		if art.Name == req.Name && art.Finalized {
			found = art
			break
		}
	}
	s.artifactStore.mu.RUnlock()

	if found == nil {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	downloadURL := fmt.Sprintf("%s://%s/_apis/v1/artifacts/%d/download", scheme, r.Host, found.ID)

	writeJSON(w, http.StatusOK, map[string]interface{}{
		"name":       found.Name,
		"signed_url": downloadURL,
	})
}

func (s *Server) handleDownloadArtifact(w http.ResponseWriter, r *http.Request) {
	idStr := r.PathValue("artifactId")
	id, err := strconv.ParseInt(idStr, 10, 64)
	if err != nil {
		http.Error(w, "invalid artifact ID", http.StatusBadRequest)
		return
	}

	s.artifactStore.mu.RLock()
	art, ok := s.artifactStore.artifacts[id]
	s.artifactStore.mu.RUnlock()

	if !ok || !art.Finalized {
		http.Error(w, "artifact not found", http.StatusNotFound)
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(art.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(art.Data)
}

// --- Actions cache ---

func (s *Server) handleCacheReserve(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.cacheScopeRepo(w, r)
	if !ok {
		return
	}
	var req struct {
		Key     string `json:"key"`
		Version string `json:"version"`
	}
	if !decodeJSONBody(w, r, &req) {
		return
	}
	if req.Key == "" || req.Version == "" {
		writeGHValidationError(w, "Cache", "key", "missing_field")
		return
	}

	s.artifactStore.mu.Lock()
	if id, ok := s.artifactStore.cacheIndex[cacheLookupKey(repo, req.Key, req.Version)]; ok {
		entry := s.artifactStore.caches[id]
		s.artifactStore.mu.Unlock()
		if entry != nil && entry.Finalized {
			writeGHError(w, http.StatusConflict, "Cache already exists")
			return
		}
		writeGHError(w, http.StatusConflict, "Cache reservation already exists")
		return
	}
	id := s.artifactStore.nextCacheID
	s.artifactStore.nextCacheID++
	entry := &CacheEntry{
		ID:            id,
		Repo:          repo,
		Key:           req.Key,
		Version:       req.Version,
		DownloadToken: newCacheDownloadToken(),
		CreatedAt:     time.Now(),
	}
	s.artifactStore.caches[id] = entry
	s.artifactStore.cacheIndex[cacheLookupKey(repo, req.Key, req.Version)] = id
	s.artifactStore.persistCacheMeta(entry)
	s.artifactStore.mu.Unlock()

	s.logger.Debug().Int64("id", id).Str("repo", repo).Str("key", req.Key).Msg("cache reserved")
	writeJSON(w, http.StatusOK, map[string]interface{}{"cacheId": id})
}

func (s *Server) handleCacheLookup(w http.ResponseWriter, r *http.Request) {
	repo, ok := s.cacheScopeRepo(w, r)
	if !ok {
		return
	}
	version := r.URL.Query().Get("version")
	keys := splitCacheKeys(r.URL.Query().Get("keys"))
	if version == "" || len(keys) == 0 {
		writeGHValidationError(w, "Cache", "keys", "missing_field")
		return
	}

	s.artifactStore.mu.RLock()
	entry := s.lookupFinalizedCacheLocked(repo, keys, version)
	s.artifactStore.mu.RUnlock()
	if entry == nil {
		s.logger.Debug().Str("repo", repo).Strs("keys", keys).Str("version", version).Msg("cache lookup miss")
		w.WriteHeader(http.StatusNoContent)
		return
	}

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	archiveURL := fmt.Sprintf("%s://%s/_apis/artifactcache/caches/%d?sig=%s", scheme, r.Host, entry.ID, entry.DownloadToken)
	s.logger.Debug().Int64("id", entry.ID).Str("key", entry.Key).Msg("cache lookup hit")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"archiveLocation": archiveURL,
		"cacheKey":        entry.Key,
	})
}

func (s *Server) lookupFinalizedCacheLocked(repo string, keys []string, version string) *CacheEntry {
	for _, key := range keys {
		if id, ok := s.artifactStore.cacheIndex[cacheLookupKey(repo, key, version)]; ok {
			entry := s.artifactStore.caches[id]
			if entry != nil && entry.Finalized {
				return entry
			}
		}
	}
	for _, key := range keys {
		var newest *CacheEntry
		for _, entry := range s.artifactStore.caches {
			if entry.Repo != repo || entry.Version != version || !entry.Finalized || !strings.HasPrefix(entry.Key, key) {
				continue
			}
			if newest == nil || entry.CreatedAt.After(newest.CreatedAt) {
				newest = entry
			}
		}
		if newest != nil {
			return newest
		}
	}
	return nil
}

func (s *Server) handleCacheUpload(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCacheID(w, r)
	if !ok {
		return
	}
	repo, ok := s.cacheScopeRepo(w, r)
	if !ok {
		return
	}
	start, end, err := parseContentRange(r.Header.Get("Content-Range"))
	if err != nil {
		writeGHError(w, http.StatusBadRequest, err.Error())
		return
	}
	chunk, err := io.ReadAll(r.Body)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "read body: "+err.Error())
		return
	}
	if int64(len(chunk)) != end-start+1 {
		writeGHError(w, http.StatusBadRequest, fmt.Sprintf("Content-Range bytes %d-%d does not match body length %d", start, end, len(chunk)))
		return
	}

	s.artifactStore.mu.Lock()
	entry := s.artifactStore.caches[id]
	if entry == nil || entry.Repo != repo {
		s.artifactStore.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Cache not found")
		return
	}
	if entry.Finalized {
		s.artifactStore.mu.Unlock()
		writeGHError(w, http.StatusConflict, "Cache already finalized")
		return
	}
	if needed := end + 1; int64(len(entry.Data)) < needed {
		entry.Data = append(entry.Data, make([]byte, needed-int64(len(entry.Data)))...)
	}
	copy(entry.Data[start:end+1], chunk)
	entry.Size = int64(len(entry.Data))
	if err := s.artifactStore.writeCacheDataAt(entry, chunk, start); err != nil {
		s.logger.Error().Err(err).Int64("id", id).Msg("cache chunk on-disk persistence failed (in-memory copy intact)")
	}
	s.artifactStore.persistCacheMeta(entry)
	s.artifactStore.mu.Unlock()

	s.logger.Debug().Int64("id", id).Int64("start", start).Int64("end", end).Msg("cache chunk uploaded")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleCacheFinalize(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCacheID(w, r)
	if !ok {
		return
	}
	repo, ok := s.cacheScopeRepo(w, r)
	if !ok {
		return
	}
	var req struct {
		Size int64 `json:"size"`
	}
	if r.Body != nil {
		if err := json.NewDecoder(r.Body).Decode(&req); err != nil && err != io.EOF {
			writeGHError(w, http.StatusBadRequest, "Problems parsing JSON")
			return
		}
	}

	s.artifactStore.mu.Lock()
	entry := s.artifactStore.caches[id]
	if entry == nil || entry.Repo != repo {
		s.artifactStore.mu.Unlock()
		writeGHError(w, http.StatusNotFound, "Cache not found")
		return
	}
	if !entry.Finalized {
		if req.Size != int64(len(entry.Data)) {
			assembled := int64(len(entry.Data))
			s.artifactStore.mu.Unlock()
			writeGHError(w, http.StatusBadRequest, fmt.Sprintf("Cache size %d does not match %d bytes uploaded", req.Size, assembled))
			return
		}
		entry.Finalized = true
		s.artifactStore.persistCacheMeta(entry)
	}
	s.artifactStore.mu.Unlock()

	s.logger.Debug().Int64("id", id).Int64("size", entry.Size).Msg("cache finalized")
	w.WriteHeader(http.StatusOK)
}

// handleCacheDownload serves the archiveLocation URL handed out by lookup.
// The cache toolkit fetches it without the runtime token (on real GitHub it
// is a pre-signed blob URL), so access is gated by the unguessable sig query
// parameter instead of bearer auth.
func (s *Server) handleCacheDownload(w http.ResponseWriter, r *http.Request) {
	id, ok := parseCacheID(w, r)
	if !ok {
		return
	}

	s.artifactStore.mu.RLock()
	entry := s.artifactStore.caches[id]
	s.artifactStore.mu.RUnlock()
	if entry == nil || !entry.Finalized {
		writeGHError(w, http.StatusNotFound, "Cache not found")
		return
	}
	if sig := r.URL.Query().Get("sig"); entry.DownloadToken == "" || sig != entry.DownloadToken {
		writeGHError(w, http.StatusNotFound, "Cache not found")
		return
	}

	w.Header().Set("Content-Type", "application/octet-stream")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(entry.Data)))
	w.WriteHeader(http.StatusOK)
	_, _ = w.Write(entry.Data)
}

func cacheLookupKey(repo, key, version string) string {
	return repo + "\x00" + key + "\x00" + version
}

func newCacheDownloadToken() string {
	b := make([]byte, 16)
	rand.Read(b)
	return hex.EncodeToString(b)
}

// cacheScopeRepo resolves the repository an Actions cache request acts for
// and writes a 401 when it can't. The @actions/cache toolkit authenticates
// every cache call with the job's runtime token (Authorization: Bearer);
// that token's sub claim is the plan scopeIdentifier of the dispatched job,
// and the job message records the repository the run executes as.
func (s *Server) cacheScopeRepo(w http.ResponseWriter, r *http.Request) (string, bool) {
	repo, err := s.repoForRuntimeToken(r)
	if err != nil {
		s.logger.Debug().Err(err).Str("path", r.URL.Path).Msg("cache request rejected")
		writeGHError(w, http.StatusUnauthorized, "Must authenticate to access cache")
		return "", false
	}
	return repo, true
}

func (s *Server) repoForRuntimeToken(r *http.Request) (string, error) {
	auth := r.Header.Get("Authorization")
	token, ok := strings.CutPrefix(auth, "Bearer ")
	if !ok {
		return "", fmt.Errorf("missing bearer token")
	}
	scopeID, err := jwtSubject(token)
	if err != nil {
		return "", err
	}

	s.store.mu.RLock()
	defer s.store.mu.RUnlock()
	for _, job := range s.store.Jobs {
		if repo, ok := jobMessageRepo(job.Message, scopeID); ok {
			return repo, nil
		}
	}
	return "", fmt.Errorf("no job with plan scope %q", scopeID)
}

// jwtSubject extracts the sub claim from a JWT without verifying the
// signature; bleephub issues runtime tokens with alg:none (see makeJWT).
func jwtSubject(token string) (string, error) {
	parts := strings.SplitN(token, ".", 3)
	if len(parts) != 3 {
		return "", fmt.Errorf("malformed JWT: expected 3 parts")
	}
	payloadBytes, err := base64urlDecode(parts[1])
	if err != nil {
		return "", fmt.Errorf("decode JWT payload: %w", err)
	}
	var payload struct {
		Sub string `json:"sub"`
	}
	if err := json.Unmarshal(payloadBytes, &payload); err != nil {
		return "", fmt.Errorf("parse JWT payload: %w", err)
	}
	if payload.Sub == "" {
		return "", fmt.Errorf("missing sub claim")
	}
	return payload.Sub, nil
}

// jobMessageRepo reports the github.repository context value of a dispatched
// job message when its plan scopeIdentifier matches scopeID.
func jobMessageRepo(message, scopeID string) (string, bool) {
	var msg struct {
		Plan struct {
			ScopeIdentifier string `json:"scopeIdentifier"`
		} `json:"plan"`
		ContextData struct {
			GitHub struct {
				D []struct {
					K string          `json:"k"`
					V json.RawMessage `json:"v"`
				} `json:"d"`
			} `json:"github"`
		} `json:"contextData"`
	}
	if err := json.Unmarshal([]byte(message), &msg); err != nil {
		return "", false
	}
	if msg.Plan.ScopeIdentifier == "" || msg.Plan.ScopeIdentifier != scopeID {
		return "", false
	}
	for _, kv := range msg.ContextData.GitHub.D {
		if kv.K != "repository" {
			continue
		}
		var repo string
		if err := json.Unmarshal(kv.V, &repo); err != nil || repo == "" {
			return "", false
		}
		return repo, true
	}
	return "", false
}

// parseContentRange parses the "bytes <start>-<end>/<total>" header the
// @actions/cache toolkit sends on every ranged chunk PATCH (total is "*").
func parseContentRange(header string) (start, end int64, err error) {
	if header == "" {
		return 0, 0, fmt.Errorf("Content-Range header is required")
	}
	spec, ok := strings.CutPrefix(header, "bytes ")
	if !ok {
		return 0, 0, fmt.Errorf("invalid Content-Range %q: expected bytes <start>-<end>/<total>", header)
	}
	spec, _, _ = strings.Cut(spec, "/")
	startStr, endStr, ok := strings.Cut(spec, "-")
	if !ok {
		return 0, 0, fmt.Errorf("invalid Content-Range %q: expected bytes <start>-<end>/<total>", header)
	}
	start, err = strconv.ParseInt(startStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid Content-Range start %q", startStr)
	}
	end, err = strconv.ParseInt(endStr, 10, 64)
	if err != nil {
		return 0, 0, fmt.Errorf("invalid Content-Range end %q", endStr)
	}
	if start < 0 || end < start {
		return 0, 0, fmt.Errorf("invalid Content-Range %d-%d", start, end)
	}
	return start, end, nil
}

func splitCacheKeys(raw string) []string {
	parts := strings.Split(raw, ",")
	keys := make([]string, 0, len(parts))
	for _, part := range parts {
		key := strings.TrimSpace(part)
		if key != "" {
			keys = append(keys, key)
		}
	}
	return keys
}

func parseCacheID(w http.ResponseWriter, r *http.Request) (int64, bool) {
	id, err := strconv.ParseInt(r.PathValue("cacheId"), 10, 64)
	if err != nil {
		writeGHError(w, http.StatusBadRequest, "Invalid cache id")
		return 0, false
	}
	return id, true
}
