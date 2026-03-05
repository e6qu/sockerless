package frontend

import (
	"net/http"
	"net/url"
)

func (s *Server) handlePodCreate(w http.ResponseWriter, r *http.Request) {
	resp, err := s.backend.postRaw(r.Context(), "/libpod/pods/create", "application/json", r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodList(w http.ResponseWriter, r *http.Request) {
	resp, err := s.backend.get(r.Context(), "/libpod/pods/json")
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodInspect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.get(r.Context(), "/libpod/pods/"+name+"/json")
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodExists(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.get(r.Context(), "/libpod/pods/"+name+"/exists")
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodStart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.post(r.Context(), "/libpod/pods/"+name+"/start", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodStop(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// BUG-389: Forward timeout query parameter to backend
	query := url.Values{}
	if t := r.URL.Query().Get("t"); t != "" {
		query.Set("t", t)
	}
	resp, err := s.backend.postWithQuery(r.Context(), "/libpod/pods/"+name+"/stop", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodKill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	// BUG-388: Forward signal query parameter to backend
	query := url.Values{}
	if signal := r.URL.Query().Get("signal"); signal != "" {
		query.Set("signal", signal)
	}
	resp, err := s.backend.postWithQuery(r.Context(), "/libpod/pods/"+name+"/kill", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	query := url.Values{}
	if force := r.URL.Query().Get("force"); force != "" {
		query.Set("force", force)
	}
	resp, err := s.backend.deleteWithQuery(r.Context(), "/libpod/pods/"+name, query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}
