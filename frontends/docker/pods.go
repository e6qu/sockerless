package frontend

import (
	"net/http"
	"net/url"
)

func (s *Server) handlePodCreate(w http.ResponseWriter, r *http.Request) {
	resp, err := s.backend.postRaw("/libpod/pods/create", "application/json", r.Body)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodList(w http.ResponseWriter, r *http.Request) {
	resp, err := s.backend.get("/libpod/pods/json")
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodInspect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.get("/libpod/pods/" + name + "/json")
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodExists(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.get("/libpod/pods/" + name + "/exists")
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodStart(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.post("/libpod/pods/"+name+"/start", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodStop(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.post("/libpod/pods/"+name+"/stop", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handlePodKill(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.post("/libpod/pods/"+name+"/kill", nil)
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
	resp, err := s.backend.deleteWithQuery("/libpod/pods/"+name, query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}
