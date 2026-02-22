package docker

import (
	"net/http"

	"github.com/docker/docker/api/types/volume"
	"github.com/sockerless/api"
)

func (s *Server) handleVolumeCreate(w http.ResponseWriter, r *http.Request) {
	var req api.VolumeCreateRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	vol, err := s.docker.VolumeCreate(r.Context(), volume.CreateOptions{
		Name:       req.Name,
		Driver:     req.Driver,
		DriverOpts: req.DriverOpts,
		Labels:     req.Labels,
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	writeJSON(w, http.StatusCreated, api.Volume{
		Name:       vol.Name,
		Driver:     vol.Driver,
		Mountpoint: vol.Mountpoint,
		CreatedAt:  vol.CreatedAt,
		Labels:     vol.Labels,
		Scope:      vol.Scope,
		Options:    vol.Options,
	})
}

func (s *Server) handleVolumeList(w http.ResponseWriter, r *http.Request) {
	vols, err := s.docker.VolumeList(r.Context(), volume.ListOptions{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	result := make([]*api.Volume, 0)
	for _, v := range vols.Volumes {
		result = append(result, &api.Volume{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			CreatedAt:  v.CreatedAt,
			Labels:     v.Labels,
			Scope:      v.Scope,
			Options:    v.Options,
		})
	}

	writeJSON(w, http.StatusOK, api.VolumeListResponse{
		Volumes:  result,
		Warnings: []string{},
	})
}

func (s *Server) handleVolumeInspect(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	vol, err := s.docker.VolumeInspect(r.Context(), name)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	writeJSON(w, http.StatusOK, api.Volume{
		Name:       vol.Name,
		Driver:     vol.Driver,
		Mountpoint: vol.Mountpoint,
		CreatedAt:  vol.CreatedAt,
		Labels:     vol.Labels,
		Scope:      vol.Scope,
		Options:    vol.Options,
	})
}

func (s *Server) handleVolumeRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	if err := s.docker.VolumeRemove(r.Context(), name, force); err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}
