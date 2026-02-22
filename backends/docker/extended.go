package docker

import (
	"encoding/json"
	"io"
	"net/http"
	"strconv"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/volume"
	"github.com/sockerless/api"
)

// handleContainerRestart restarts a container via the Docker SDK.
func (s *Server) handleContainerRestart(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var timeout *int
	if t := r.URL.Query().Get("t"); t != "" {
		v, _ := strconv.Atoi(t)
		timeout = &v
	}
	err := s.docker.ContainerRestart(r.Context(), id, container.StopOptions{Timeout: timeout})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleContainerTop returns the running processes inside a container.
func (s *Server) handleContainerTop(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	psArgs := r.URL.Query().Get("ps_args")
	if psArgs == "" {
		psArgs = "-ef"
	}

	top, err := s.docker.ContainerTop(r.Context(), id, []string{psArgs})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	writeJSON(w, http.StatusOK, api.ContainerTopResponse{
		Titles:    top.Titles,
		Processes: top.Processes,
	})
}

// handleContainerPrune removes all stopped containers.
func (s *Server) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	report, err := s.docker.ContainersPrune(r.Context(), filters.Args{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	deleted := report.ContainersDeleted
	if deleted == nil {
		deleted = []string{}
	}
	writeJSON(w, http.StatusOK, api.ContainerPruneResponse{
		ContainersDeleted: deleted,
		SpaceReclaimed:    report.SpaceReclaimed,
	})
}

// handleContainerStats returns resource usage statistics for a container.
func (s *Server) handleContainerStats(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	stream := r.URL.Query().Get("stream") != "false" && r.URL.Query().Get("stream") != "0"

	stats, err := s.docker.ContainerStats(r.Context(), id, stream)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer stats.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, stats.Body)
}

// handleContainerRename renames a container.
func (s *Server) handleContainerRename(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	newName := r.URL.Query().Get("name")
	if newName == "" {
		writeError(w, &api.InvalidParameterError{Message: "new name is required"})
		return
	}

	err := s.docker.ContainerRename(r.Context(), id, newName)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleContainerPause pauses a container.
func (s *Server) handleContainerPause(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.docker.ContainerPause(r.Context(), id)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleContainerUnpause unpauses a container.
func (s *Server) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	err := s.docker.ContainerUnpause(r.Context(), id)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleNetworkConnect connects a container to a network.
func (s *Server) handleNetworkConnect(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var req api.NetworkConnectRequest
	if err := readJSON(r, &req); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	var epConfig *network.EndpointSettings
	if req.EndpointConfig != nil {
		epConfig = &network.EndpointSettings{
			NetworkID:  req.EndpointConfig.NetworkID,
			EndpointID: req.EndpointConfig.EndpointID,
			Gateway:    req.EndpointConfig.Gateway,
			IPAddress:  req.EndpointConfig.IPAddress,
			MacAddress: req.EndpointConfig.MacAddress,
		}
	}

	err := s.docker.NetworkConnect(r.Context(), id, req.Container, epConfig)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// handleVolumePrune removes unused volumes.
func (s *Server) handleVolumePrune(w http.ResponseWriter, r *http.Request) {
	report, err := s.docker.VolumesPrune(r.Context(), filters.Args{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	deleted := report.VolumesDeleted
	if deleted == nil {
		deleted = []string{}
	}
	writeJSON(w, http.StatusOK, api.VolumePruneResponse{
		VolumesDeleted: deleted,
		SpaceReclaimed: report.SpaceReclaimed,
	})
}

// handleImageList lists all images.
func (s *Server) handleImageList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"

	images, err := s.docker.ImageList(r.Context(), image.ListOptions{All: all})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	result := make([]*api.ImageSummary, 0, len(images))
	for _, img := range images {
		result = append(result, &api.ImageSummary{
			ID:          img.ID,
			ParentID:    img.ParentID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     img.Created,
			Size:        img.Size,
			SharedSize:  img.SharedSize,
			VirtualSize: img.Size,
			Labels:      img.Labels,
			Containers:  img.Containers,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// handleImageRemove removes an image.
func (s *Server) handleImageRemove(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	prune := r.URL.Query().Get("noprune") != "1" && r.URL.Query().Get("noprune") != "true"

	items, err := s.docker.ImageRemove(r.Context(), name, image.RemoveOptions{
		Force:         force,
		PruneChildren: prune,
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	result := make([]*api.ImageDeleteResponse, 0, len(items))
	for _, item := range items {
		result = append(result, &api.ImageDeleteResponse{
			Untagged: item.Untagged,
			Deleted:  item.Deleted,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// handleImageHistory returns the history of an image.
func (s *Server) handleImageHistory(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")

	history, err := s.docker.ImageHistory(r.Context(), name)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	result := make([]*api.ImageHistoryEntry, 0, len(history))
	for _, h := range history {
		result = append(result, &api.ImageHistoryEntry{
			ID:        h.ID,
			Created:   h.Created,
			CreatedBy: h.CreatedBy,
			Tags:      h.Tags,
			Size:      h.Size,
			Comment:   h.Comment,
		})
	}
	writeJSON(w, http.StatusOK, result)
}

// handleImagePrune removes unused images.
func (s *Server) handleImagePrune(w http.ResponseWriter, r *http.Request) {
	report, err := s.docker.ImagesPrune(r.Context(), filters.Args{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	var deleted []*api.ImageDeleteResponse
	for _, img := range report.ImagesDeleted {
		deleted = append(deleted, &api.ImageDeleteResponse{
			Untagged: img.Untagged,
			Deleted:  img.Deleted,
		})
	}
	if deleted == nil {
		deleted = []*api.ImageDeleteResponse{}
	}
	writeJSON(w, http.StatusOK, api.ImagePruneResponse{
		ImagesDeleted:  deleted,
		SpaceReclaimed: report.SpaceReclaimed,
	})
}

// handleSystemEvents streams Docker events to the client.
func (s *Server) handleSystemEvents(w http.ResponseWriter, r *http.Request) {
	eventsCh, errCh := s.docker.Events(r.Context(), events.ListOptions{})

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	for {
		select {
		case event, ok := <-eventsCh:
			if !ok {
				return
			}
			json.NewEncoder(w).Encode(event)
			if canFlush {
				flusher.Flush()
			}
		case err, ok := <-errCh:
			if ok && err != nil {
				return
			}
			return
		case <-r.Context().Done():
			return
		}
	}
}

// handleSystemDf returns disk usage information.
func (s *Server) handleSystemDf(w http.ResponseWriter, r *http.Request) {
	du, err := s.docker.DiskUsage(r.Context(), types.DiskUsageOptions{})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	var containers []*api.ContainerSummary
	for _, c := range du.Containers {
		containers = append(containers, &api.ContainerSummary{
			ID:      c.ID,
			Names:   c.Names,
			Image:   c.Image,
			ImageID: c.ImageID,
			Command: c.Command,
			Created: c.Created,
			State:   c.State,
			Status:  c.Status,
			Labels:  c.Labels,
		})
	}
	if containers == nil {
		containers = []*api.ContainerSummary{}
	}

	var images []*api.ImageSummary
	for _, img := range du.Images {
		images = append(images, &api.ImageSummary{
			ID:          img.ID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     img.Created,
			Size:        img.Size,
			SharedSize:  img.SharedSize,
			Labels:      img.Labels,
			Containers:  img.Containers,
		})
	}
	if images == nil {
		images = []*api.ImageSummary{}
	}

	var volumes []*api.Volume
	if du.Volumes != nil {
		for _, v := range du.Volumes {
			volumes = append(volumes, &api.Volume{
				Name:       v.Name,
				Driver:     v.Driver,
				Mountpoint: v.Mountpoint,
				CreatedAt:  v.CreatedAt,
				Labels:     v.Labels,
				Scope:      v.Scope,
				Options:    v.Options,
			})
		}
	}
	if volumes == nil {
		volumes = []*api.Volume{}
	}

	writeJSON(w, http.StatusOK, api.DiskUsageResponse{
		LayersSize: du.LayersSize,
		Images:     images,
		Containers: containers,
		Volumes:    volumes,
	})
}

// Prevent unused import errors
var (
	_ = volume.ListOptions{}
)
