package frontend

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/sockerless/api"
)

func (s *Server) handleImageCreate(w http.ResponseWriter, r *http.Request) {
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

	resp, err := s.backend.post("/images/pull", &api.ImagePullRequest{
		Reference: ref,
		Auth:      auth,
	})
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	// Stream the progress JSON to the client
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
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
		s.handleNotImplemented(w, r)
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
	resp, err := s.backend.getWithQuery("/images/inspect", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImageLoad(w http.ResponseWriter, r *http.Request) {
	resp, err := s.backend.postRaw("/images/load", "application/x-tar", r.Body)
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

	resp, err := s.backend.postWithQuery("/images/tag", query, nil)
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
	resp, err := s.backend.getWithQuery("/images", query)
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
	resp, err := s.backend.deleteWithQuery("/images/"+url.PathEscape(name), query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImageHistoryByName(w http.ResponseWriter, r *http.Request, name string) {
	resp, err := s.backend.get("/images/" + url.PathEscape(name) + "/history")
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
	resp, err := s.backend.postWithQuery("/images/prune", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleImageBuild(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	for _, key := range []string{"t", "dockerfile", "buildargs", "rm", "forcerm", "nocache"} {
		if v := r.URL.Query().Get(key); v != "" {
			query.Set(key, v)
		}
	}

	resp, err := s.backend.postRawWithQuery("/images/build", query, "application/x-tar", r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(resp.StatusCode)
	io.Copy(w, resp.Body)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

func (s *Server) handleAuth(w http.ResponseWriter, r *http.Request) {
	var req api.AuthRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.backend.post("/auth", &req)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}
