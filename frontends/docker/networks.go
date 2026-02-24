package frontend

import (
	"encoding/json"
	"net/http"
	"net/url"

	"github.com/sockerless/api"
)

func (s *Server) handleNetworkCreate(w http.ResponseWriter, r *http.Request) {
	var req api.NetworkCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.backend.post(r.Context(), "/networks", &req)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()

	if resp.StatusCode >= 400 {
		proxyErrorResponse(w, resp)
		return
	}

	var result api.NetworkCreateResponse
	if err := json.NewDecoder(resp.Body).Decode(&result); err != nil {
		writeError(w, err)
		return
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleNetworkList(w http.ResponseWriter, r *http.Request) {
	query := url.Values{}
	if filters := r.URL.Query().Get("filters"); filters != "" {
		query.Set("filters", filters)
	}
	resp, err := s.backend.getWithQuery(r.Context(), "/networks", query)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleNetworkInspect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.backend.get(r.Context(), "/networks/"+id)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleNetworkDisconnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req api.NetworkDisconnectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.backend.post(r.Context(), "/networks/"+id+"/disconnect", &req)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleNetworkRemove(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	resp, err := s.backend.delete(r.Context(), "/networks/"+id)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}

func (s *Server) handleNetworkPrune(w http.ResponseWriter, r *http.Request) {
	resp, err := s.backend.post(r.Context(), "/networks/prune", nil)
	if err != nil {
		writeError(w, err)
		return
	}
	defer resp.Body.Close()
	proxyPassthrough(w, resp)
}
