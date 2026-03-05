package frontend

import (
	"encoding/json"
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sockerless/api"
)

func (s *Server) handleImageCreate(w http.ResponseWriter, r *http.Request) {
	// docker import: fromSrc is set (e.g. "-" for stdin tar)
	if fromSrc := r.URL.Query().Get("fromSrc"); fromSrc != "" {
		resp, err := s.backend.postRaw(r.Context(), "/images/load", "application/x-tar", r.Body)
		if err != nil {
			writeError(w, err)
			return
		}
		defer resp.Body.Close()

		// BUG-498: Forward repo/tag by tagging the loaded image
		repo := r.URL.Query().Get("repo")
		if repo != "" {
			body, _ := io.ReadAll(resp.Body)
			var result map[string]string
			if json.Unmarshal(body, &result) == nil {
				if stream, ok := result["stream"]; ok {
					loaded := strings.TrimPrefix(stream, "Loaded image: ")
					loaded = strings.TrimSpace(loaded)
					if loaded != "" {
						tag := r.URL.Query().Get("tag")
						query := url.Values{}
						query.Set("name", loaded)
						query.Set("repo", repo)
						if tag != "" {
							query.Set("tag", tag)
						}
						_, _ = s.backend.postWithQuery(r.Context(), "/images/tag", query, nil)
					}
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(body)
			return
		}

		proxyPassthrough(w, resp)
		return
	}

	platform := r.URL.Query().Get("platform")
	fromImage := r.URL.Query().Get("fromImage")
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}

	ref := fromImage
	if tag != "" && tag != "latest" {
		ref = fromImage + ":" + tag
	} else if ref != "" && tag == "latest" {
		ref = fromImage + ":latest"
	}

	auth := r.Header.Get("X-Registry-Auth")

	resp, err := s.backend.post(r.Context(), "/images/pull", &api.ImagePullRequest{
		Reference: ref,
		Auth:      auth,
		Platform:  platform,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	// Stream the progress JSON to the client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	flushingCopy(w, resp.Body)
}

// handleImageCatchAll handles /images/{name}/json, /images/{name}/tag, etc.
// where {name} can contain slashes (e.g., library/alpine).
// The path is parsed manually since Go's ServeMux doesn't support wildcards in the middle.
func (s *Server) handleImageCatchAll(w http.ResponseWriter, r *http.Request) {
	// Strip version prefix: /v1.44/images/... or /images/...
	path := r.URL.Path
	if i := strings.Index(path, "/images/"); i >= 0 {
		path = path[i+len("/images/"):]
	}

	// Determine the suffix action
	switch {
	case strings.HasSuffix(path, "/json"):
		name := strings.TrimSuffix(path, "/json")
		s.handleImageInspectByName(w, r, name)
	case strings.HasSuffix(path, "/tag"):
		name := strings.TrimSuffix(path, "/tag")
		s.handleImageTagByName(w, r, name)
	case strings.HasSuffix(path, "/history"):
		name := strings.TrimSuffix(path, "/history")
		s.handleImageHistoryByName(w, r, name)
	case strings.HasSuffix(path, "/push"):
		name := strings.TrimSuffix(path, "/push")
		s.handleImagePush(w, r, name)
	case strings.HasSuffix(path, "/get"):
		name := strings.TrimSuffix(path, "/get")
		s.handleImageSaveByName(w, r, name)
	default:
		// DELETE /images/{name}
		if r.Method == "DELETE" {
			s.handleImageRemoveByName(w, r, path)
		} else {
			s.handleNotImplemented(w, r)
		}
	}
}

func (s *Server) handleImageInspectByName(w http.ResponseWriter, r *http.Request, name string) {
	query := url.Values{}
	query.Set("name", name)
	resp, err := s.backend.getWithQuery(r.Context(), "/images/inspect", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImageLoad(w http.ResponseWriter, r *http.Request) {
	// BUG-492: Forward quiet query param to backend
	path := "/images/load"
	if q := r.URL.Query().Get("quiet"); q != "" {
		path += "?quiet=" + q
	}
	resp, err := s.backend.postRaw(r.Context(), path, "application/x-tar", r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImageTagByName(w http.ResponseWriter, r *http.Request, name string) {
	query := url.Values{}
	query.Set("name", name)
	if repo := r.URL.Query().Get("repo"); repo != "" {
		query.Set("repo", repo)
	}
	if tag := r.URL.Query().Get("tag"); tag != "" {
		query.Set("tag", tag)
	}

	resp, err := s.backend.postWithQuery(r.Context(), "/images/tag", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImageList(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if all := r.URL.Query().Get("all"); all != "" {
		query.Set("all", all)
	}
	if filters := r.URL.Query().Get("filters"); filters != "" {
		query.Set("filters", filters)
	}
	if sharedSize := r.URL.Query().Get("shared-size"); sharedSize != "" {
		query.Set("shared-size", sharedSize)
	}
	if digests := r.URL.Query().Get("digests"); digests != "" {
		query.Set("digests", digests)
	}
	resp, err := s.backend.getWithQuery(r.Context(), "/images", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImageRemoveByName(w http.ResponseWriter, r *http.Request, name string) {
	query := url.Values{}
	if force := r.URL.Query().Get("force"); force != "" {
		query.Set("force", force)
	}
	if noprune := r.URL.Query().Get("noprune"); noprune != "" {
		query.Set("noprune", noprune)
	}
	resp, err := s.backend.deleteWithQuery(r.Context(), "/images/"+url.PathEscape(name), query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImageHistoryByName(w http.ResponseWriter, r *http.Request, name string) {
	resp, err := s.backend.get(r.Context(), "/images/"+url.PathEscape(name)+"/history")
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImagePrune(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if filters := r.URL.Query().Get("filters"); filters != "" {
		query.Set("filters", filters)
	}
	resp, err := s.backend.postWithQuery(r.Context(), "/images/prune", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImageBuild(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	for _, key := range []string{
		"t", "dockerfile", "buildargs", "rm", "forcerm", "nocache",
		"labels", "target", "platform", "pull", "cachefrom",
		"q", "memory", "memswap", "cpushares", "cpuquota",
		"cpuperiod", "cpusetcpus", "cpusetmems", "shmsize",
		"extrahosts", "networkmode", "squash",
	} {
		if v := r.URL.Query().Get(key); v != "" {
			query.Set(key, v)
		}
	}

	resp, err := s.backend.postRawWithQuery(r.Context(), "/images/build", query, "application/x-tar", r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	flushingCopy(w, resp.Body)
}

func (s *Server) handleImagePush(w http.ResponseWriter, r *http.Request, name string) {
	query := url.Values{}
	if tag := r.URL.Query().Get("tag"); tag != "" {
		query.Set("tag", tag)
	}
	if auth := r.Header.Get("X-Registry-Auth"); auth != "" {
		query.Set("auth", auth)
	}
	resp, err := s.backend.postWithQuery(r.Context(), "/images/"+url.PathEscape(name)+"/push", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	flushingCopy(w, resp.Body)
}

func (s *Server) handleImageSave(w http.ResponseWriter, r *http.Request) {
	resp, err := s.backend.getWithQuery(r.Context(), "/images/get", r.URL.Query())
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	flushingCopy(w, resp.Body)
}

func (s *Server) handleImageSaveByName(w http.ResponseWriter, r *http.Request, name string) {
	resp, err := s.backend.get(r.Context(), "/images/"+url.PathEscape(name)+"/get")
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	for k, vs := range resp.Header {
		for _, v := range vs {
			w.Header().Add(k, v)
		}
	}
	w.WriteHeader(resp.StatusCode)
	flushingCopy(w, resp.Body)
}

func (s *Server) handleImageSearch(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if term := r.URL.Query().Get("term"); term != "" {
		query.Set("term", term)
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		query.Set("limit", limit)
	}
	if filters := r.URL.Query().Get("filters"); filters != "" {
		query.Set("filters", filters)
	}
	resp, err := s.backend.getWithQuery(r.Context(), "/images/search", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerCommit(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	for _, key := range []string{"container", "repo", "tag", "comment", "author", "pause", "changes"} {
		if v := r.URL.Query().Get(key); v != "" {
			query.Set(key, v)
		}
	}
	resp, err := s.backend.postRawWithQuery(r.Context(), "/commit", query, "application/json", r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

// BUG-573: handleBuildPrune returns a stub response for build cache pruning.
func (s *Server) handleBuildPrune(w http.ResponseWriter, r *http.Request) {
	writeJSON(w, http.StatusOK, map[string]any{
		"CachesDeleted": nil,
		"SpaceReclaimed": 0,
	})
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req api.AuthRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.backend.post(r.Context(), "/auth", &req)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}
