package bleephub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strconv"
	"sync"
	"time"
)

// ArtifactStore holds in-memory artifacts for @actions/artifact v4 Twirp API.
type ArtifactStore struct {
	mu        sync.RWMutex
	artifacts map[int64]*Artifact
	nextID    int64
}

// Artifact represents an uploaded artifact.
type Artifact struct {
	ID        int64     `json:"id"`
	Name      string    `json:"name"`
	Size      int64     `json:"size"`
	Data      []byte    `json:"-"`
	Finalized bool      `json:"finalized"`
	RunID     string    `json:"runId"`
	CreatedAt time.Time `json:"createdAt"`
}

// NewArtifactStore creates an empty artifact store.
func NewArtifactStore() *ArtifactStore {
	return &ArtifactStore{
		artifacts: make(map[int64]*Artifact),
		nextID:    1,
	}
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

	// Cache stubs (accept and discard / return miss)
	s.mux.HandleFunc("POST /_apis/artifactcache/cache", s.handleCacheReserve)
	s.mux.HandleFunc("GET /_apis/artifactcache/cache", s.handleCacheLookup)
	s.mux.HandleFunc("PATCH /_apis/artifactcache/caches/{cacheId}", s.handleCacheUpload)
	s.mux.HandleFunc("POST /_apis/artifactcache/caches/{cacheId}", s.handleCacheFinalize)
}

// --- Artifact Twirp handlers ---

func (s *Server) handleCreateArtifact(w http.ResponseWriter, r *http.Request) {
	var req struct {
		WorkflowRunBackendID string `json:"workflow_run_backend_id"`
		Name                 string `json:"name"`
		Version              int    `json:"version"`
	}
	if err := json.NewDecoder(r.Body).Decode(&req); err != nil {
		http.Error(w, "invalid JSON", http.StatusBadRequest)
		return
	}

	s.artifactStore.mu.Lock()
	id := s.artifactStore.nextID
	s.artifactStore.nextID++
	art := &Artifact{
		ID:        id,
		Name:      req.Name,
		RunID:     req.WorkflowRunBackendID,
		CreatedAt: time.Now(),
	}
	s.artifactStore.artifacts[id] = art
	s.artifactStore.mu.Unlock()

	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	uploadURL := fmt.Sprintf("%s://%s/_apis/v1/artifacts/%d/upload", scheme, r.Host, id)

	s.logger.Debug().Str("name", req.Name).Int64("id", id).Msg("artifact created")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"ok":              true,
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
	json.NewDecoder(r.Body).Decode(&req)

	s.artifactStore.mu.Lock()
	var found *Artifact
	for _, art := range s.artifactStore.artifacts {
		if art.Name == req.Name && !art.Finalized {
			art.Finalized = true
			found = art
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
				"name":             art.Name,
				"id":               art.ID,
				"size":             art.Size,
				"created_at":       art.CreatedAt.UTC().Format(time.RFC3339),
				"database_id":      art.ID,
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
	json.NewDecoder(r.Body).Decode(&req)

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
		"name":               found.Name,
		"signed_url":         downloadURL,
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
	w.Write(art.Data)
}

// --- Cache stubs ---

func (s *Server) handleCacheReserve(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug().Msg("cache reserve (no-op)")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCacheLookup(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug().Msg("cache lookup (miss)")
	w.WriteHeader(http.StatusNoContent)
}

func (s *Server) handleCacheUpload(w http.ResponseWriter, r *http.Request) {
	// Drain the body and discard
	io.Copy(io.Discard, r.Body)
	s.logger.Debug().Msg("cache upload chunk (discarded)")
	w.WriteHeader(http.StatusOK)
}

func (s *Server) handleCacheFinalize(w http.ResponseWriter, r *http.Request) {
	s.logger.Debug().Msg("cache finalize (discarded)")
	w.WriteHeader(http.StatusOK)
}
