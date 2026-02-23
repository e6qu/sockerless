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
	resp, err := s.backend.post(r.Context(), "/libpod/pods/"+name+"/stop", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodKill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.post(r.Context(), "/libpod/pods/"+name+"/kill", nil)
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
