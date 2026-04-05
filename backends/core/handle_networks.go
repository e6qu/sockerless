package core

import (
	"net/http"

	"github.com/sockerless/api"
)

func (s *BaseServer) handleNetworkCreate(w http.ResponseWriter, r *http.Request) {
	var req api.NetworkCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.self.NetworkCreate(&req)
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, http.StatusCreated, resp)
}

func (s *BaseServer) handleNetworkList(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	result, err := s.self.NetworkList(filters)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

func (s *BaseServer) handleNetworkInspect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	n, err := s.self.NetworkInspect(ref)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, n)
}

func (s *BaseServer) handleNetworkDisconnect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")

	var req api.NetworkDisconnectRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	if err := s.self.NetworkDisconnect(ref, &req); err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleNetworkRemove(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")

	if err := s.self.NetworkRemove(ref); err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleNetworkPrune(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	resp, err := s.self.NetworkPrune(filters)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handleNetworkConnect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")

	var req api.NetworkConnectRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	if err := s.self.NetworkConnect(ref, &req); err != nil {
		WriteError(w, err)
		return
	}

	w.WriteHeader(http.StatusNoContent)
}
