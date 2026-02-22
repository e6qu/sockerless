package frontend

import (
	"net/http"
	"net/url"

	"github.com/sockerless/api"
)

func (s *Server) handleContainerCreate(w http.ResponseWriter, r *http.Request) {
	var req api.ContainerCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	query := url.Values{}
	if name := r.URL.Query().Get("name"); name != "" {
		query.Set("name", name)
	}

	resp, err := s.backend.postWithQuery("/containers", query, &req)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		proxyErrorResponse(w, resp)
		return
	}

	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerList(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if all := r.URL.Query().Get("all"); all != "" {
		query.Set("all", all)
	}
	if filters := r.URL.Query().Get("filters"); filters != "" {
		query.Set("filters", filters)
	}
	if limit := r.URL.Query().Get("limit"); limit != "" {
		query.Set("limit", limit)
	}

	resp, err := s.backend.getWithQuery("/containers", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.backend.get("/containers/" + id)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerStart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.backend.post("/containers/"+id+"/start", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerStop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if t := r.URL.Query().Get("t"); t != "" {
		query.Set("t", t)
	}
	resp, err := s.backend.postWithQuery("/containers/"+id+"/stop", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerRestart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if t := r.URL.Query().Get("t"); t != "" {
		query.Set("t", t)
	}
	resp, err := s.backend.postWithQuery("/containers/"+id+"/restart", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerKill(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if signal := r.URL.Query().Get("signal"); signal != "" {
		query.Set("signal", signal)
	}
	resp, err := s.backend.postWithQuery("/containers/"+id+"/kill", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if force := r.URL.Query().Get("force"); force != "" {
		query.Set("force", force)
	}
	if v := r.URL.Query().Get("v"); v != "" {
		query.Set("v", v)
	}
	resp, err := s.backend.deleteWithQuery("/containers/"+id, query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerWait(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if condition := r.URL.Query().Get("condition"); condition != "" {
		query.Set("condition", condition)
	}
	resp, err := s.backend.postWithQuery("/containers/"+id+"/wait", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerRename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	query := url.Values{}
	if name := r.URL.Query().Get("name"); name != "" {
		query.Set("name", name)
	}
	resp, err := s.backend.postWithQuery("/containers/"+id+"/rename", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerPause(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.backend.post("/containers/"+id+"/pause", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.backend.post("/containers/"+id+"/unpause", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if filters := r.URL.Query().Get("filters"); filters != "" {
		query.Set("filters", filters)
	}
	resp, err := s.backend.postWithQuery("/containers/prune", query, nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}
