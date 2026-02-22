package bleephub

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"strings"
	"sync"
	"time"
)

// ActionCache stores downloaded action tarballs in memory.
type ActionCache struct {
	mu      sync.RWMutex
	entries map[string]*ActionCacheEntry
}

// ActionCacheEntry holds a cached action tarball.
type ActionCacheEntry struct {
	Data        []byte
	ResolvedSha string
	FetchedAt   time.Time
}

// NewActionCache creates an empty action cache.
func NewActionCache() *ActionCache {
	return &ActionCache{
		entries: make(map[string]*ActionCacheEntry),
	}
}

// Get returns a cached entry by key ("owner/repo@ref").
func (ac *ActionCache) Get(key string) *ActionCacheEntry {
	ac.mu.RLock()
	defer ac.mu.RUnlock()
	return ac.entries[key]
}

// Put stores an entry in the cache.
func (ac *ActionCache) Put(key string, entry *ActionCacheEntry) {
	ac.mu.Lock()
	defer ac.mu.Unlock()
	ac.entries[key] = entry
}

func (s *Server) registerActionRoutes() {
	// Tarball proxy — serves cached action tarballs
	s.mux.HandleFunc("GET /_apis/v1/actions/tarball/{owner}/{repo}/{ref...}", s.handleActionTarball)
}

// handleActionDownloadInfo handles the runner's request for action download URLs.
// It parses the ActionReferenceList body and returns ActionDownloadInfoCollection
// with tarball URLs pointing back to bleephub's proxy.
func (s *Server) handleActionDownloadInfo(w http.ResponseWriter, r *http.Request) {
	scheme := "http"
	if r.TLS != nil {
		scheme = "https"
	}
	serverURL := scheme + "://" + r.Host

	var body struct {
		Actions []struct {
			NameWithOwner string `json:"nameWithOwner"`
			Ref           string `json:"ref"`
		} `json:"actions"`
	}
	if err := json.NewDecoder(r.Body).Decode(&body); err != nil {
		s.logger.Debug().Err(err).Msg("action download info: no body or empty")
		// Return empty on parse failure (runner may send empty body for run: steps)
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"actions": map[string]interface{}{},
		})
		return
	}

	if len(body.Actions) == 0 {
		writeJSON(w, http.StatusOK, map[string]interface{}{
			"actions": map[string]interface{}{},
		})
		return
	}

	actions := make(map[string]interface{}, len(body.Actions))
	for _, a := range body.Actions {
		key := a.NameWithOwner + "@" + a.Ref

		// Check cache for resolved SHA
		resolvedSha := "0000000000000000000000000000000000000000"
		if entry := s.actionCache.Get(key); entry != nil {
			resolvedSha = entry.ResolvedSha
		}

		tarballURL := fmt.Sprintf("%s/_apis/v1/actions/tarball/%s/%s",
			serverURL, a.NameWithOwner, a.Ref)
		zipballURL := tarballURL // runner uses tarball, but we provide both

		actions[key] = map[string]interface{}{
			"nameWithOwner":         a.NameWithOwner,
			"resolvedNameWithOwner": a.NameWithOwner,
			"resolvedSha":           resolvedSha,
			"ref":                   a.Ref,
			"tarballUrl":            tarballURL,
			"zipballUrl":            zipballURL,
			"authentication": map[string]interface{}{
				"expiresAt": "2099-01-01T00:00:00Z",
				"token":     "x-access-token",
			},
		}
	}

	s.logger.Debug().Int("count", len(actions)).Msg("action download info")
	writeJSON(w, http.StatusOK, map[string]interface{}{
		"actions": actions,
	})
}

// handleActionTarball serves a cached action tarball, or fetches from GitHub
// and caches it on first request.
func (s *Server) handleActionTarball(w http.ResponseWriter, r *http.Request) {
	owner := r.PathValue("owner")
	repo := r.PathValue("repo")
	ref := r.PathValue("ref")

	if owner == "" || repo == "" || ref == "" {
		http.Error(w, "invalid action path", http.StatusBadRequest)
		return
	}

	nameWithOwner := owner + "/" + repo
	key := nameWithOwner + "@" + ref

	// Check cache
	if entry := s.actionCache.Get(key); entry != nil {
		s.logger.Debug().Str("key", key).Msg("serving cached action tarball")
		w.Header().Set("Content-Type", "application/gzip")
		w.Header().Set("Content-Length", fmt.Sprintf("%d", len(entry.Data)))
		w.WriteHeader(http.StatusOK)
		w.Write(entry.Data)
		return
	}

	// Fetch from GitHub
	s.logger.Info().Str("key", key).Msg("fetching action tarball from GitHub")
	entry, err := fetchActionTarball(nameWithOwner, ref)
	if err != nil {
		s.logger.Error().Err(err).Str("key", key).Msg("failed to fetch action tarball")
		http.Error(w, "failed to fetch action: "+err.Error(), http.StatusBadGateway)
		return
	}

	// Cache it
	s.actionCache.Put(key, entry)

	w.Header().Set("Content-Type", "application/gzip")
	w.Header().Set("Content-Length", fmt.Sprintf("%d", len(entry.Data)))
	w.WriteHeader(http.StatusOK)
	w.Write(entry.Data)
}

// fetchActionTarball downloads an action tarball from GitHub's public API.
func fetchActionTarball(nameWithOwner, ref string) (*ActionCacheEntry, error) {
	url := fmt.Sprintf("https://api.github.com/repos/%s/tarball/%s", nameWithOwner, ref)

	client := &http.Client{
		Timeout: 60 * time.Second,
		CheckRedirect: func(req *http.Request, via []*http.Request) error {
			if len(via) >= 10 {
				return fmt.Errorf("too many redirects")
			}
			return nil
		},
	}

	req, err := http.NewRequest("GET", url, nil)
	if err != nil {
		return nil, err
	}
	req.Header.Set("Accept", "application/vnd.github+json")
	req.Header.Set("User-Agent", "bleephub/1.0")

	resp, err := client.Do(req)
	if err != nil {
		return nil, fmt.Errorf("GET %s: %w", url, err)
	}
	defer resp.Body.Close()

	if resp.StatusCode != http.StatusOK {
		return nil, fmt.Errorf("GET %s: status %d", url, resp.StatusCode)
	}

	data, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, fmt.Errorf("read body: %w", err)
	}

	// Try to extract SHA from redirect URL or response headers
	resolvedSha := "0000000000000000000000000000000000000000"
	if etag := resp.Header.Get("ETag"); etag != "" {
		etag = strings.Trim(etag, "\"")
		if len(etag) == 40 {
			resolvedSha = etag
		}
	}

	return &ActionCacheEntry{
		Data:        data,
		ResolvedSha: resolvedSha,
		FetchedAt:   time.Now(),
	}, nil
}
