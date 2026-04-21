package core

import (
	"net/http"

	"github.com/sockerless/api"
)

func (s *BaseServer) handleVolumeCreate(w http.ResponseWriter, r *http.Request) {
	var req api.VolumeCreateRequest
	if err := ReadJSON(r, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}
	vol, err := s.self.VolumeCreate(&req)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusCreated, vol)
}

func (s *BaseServer) handleVolumeList(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))
	resp, err := s.self.VolumeList(filters)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handleVolumeInspect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	vol, err := s.self.VolumeInspect(name)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, vol)
}

func (s *BaseServer) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"

	if err := s.self.VolumeRemove(name, force); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleVolumePrune(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	resp, err := s.self.VolumePrune(filters)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}
