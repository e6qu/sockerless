package frontend

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/sockerless/api"
)

func (s *Server) handleVolumeCreate(w http.ResponseWriter, r *http.Request) {
	var req api.VolumeCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.backend.post(r.Context(), "/volumes", &req)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		proxyErrorResponse(w, resp)
		return
	}

	var vol api.Volume
	if err := json.NewDecoder(resp.Body).Decode(&vol); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, vol)
}

func (s *Server) handleVolumeList(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if filters := r.URL.Query().Get("filters"); filters != "" {
		query.Set("filters", filters)
	}
	resp, err := s.backend.getWithQuery(r.Context(), "/volumes", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleVolumeInspect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.backend.get(r.Context(), "/volumes/"+name)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	query := url.Values{}
	if force := r.URL.Query().Get("force"); force != "" {
		query.Set("force", force)
	}
	resp, err := s.backend.deleteWithQuery(r.Context(), "/volumes/"+name, query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}
