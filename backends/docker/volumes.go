package docker

import (
	"net/http"

	"github.com/docker/docker/api/types/filters"
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

	result := api.Volume{
		Name:       vol.Name,
		Driver:     vol.Driver,
		Mountpoint: vol.Mountpoint,
		CreatedAt:  vol.CreatedAt,
		Labels:     vol.Labels,
		Scope:      vol.Scope,
		Options:    vol.Options,
		Status:     vol.Status,
	}
	if vol.UsageData != nil {
		result.UsageData = &api.VolumeUsageData{
			Size:     vol.UsageData.Size,
			RefCount: vol.UsageData.RefCount,
		}
	}
	writeJSON(w, http.StatusCreated, result)
}

func (s *Server) handleVolumeList(w http.ResponseWriter, r *http.Request) {
	opts := volume.ListOptions{}
	if fj := r.URL.Query().Get("filters"); fj != "" {
		opts.Filters, _ = filters.FromJSON(fj)
	}
	vols, err := s.docker.VolumeList(r.Context(), opts)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	result := make([]*api.Volume, 0)
	for _, v := range vols.Volumes {
		vol := &api.Volume{
			Name:       v.Name,
			Driver:     v.Driver,
			Mountpoint: v.Mountpoint,
			CreatedAt:  v.CreatedAt,
			Labels:     v.Labels,
			Scope:      v.Scope,
			Options:    v.Options,
			Status:     v.Status,
		}
		if v.UsageData != nil {
			vol.UsageData = &api.VolumeUsageData{
				Size:     v.UsageData.Size,
				RefCount: v.UsageData.RefCount,
			}
		}
		result = append(result, vol)
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

	inspectResult := api.Volume{
		Name:       vol.Name,
		Driver:     vol.Driver,
		Mountpoint: vol.Mountpoint,
		CreatedAt:  vol.CreatedAt,
		Labels:     vol.Labels,
		Scope:      vol.Scope,
		Options:    vol.Options,
		Status:     vol.Status,
	}
	if vol.UsageData != nil {
		inspectResult.UsageData = &api.VolumeUsageData{
			Size:     vol.UsageData.Size,
			RefCount: vol.UsageData.RefCount,
		}
	}
	writeJSON(w, http.StatusOK, inspectResult)
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
