package docker

import (
	"encoding/base64"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"time"

	"github.com/docker/docker/api/types"
	"github.com/docker/docker/api/types/container"
	"github.com/docker/docker/api/types/events"
	"github.com/docker/docker/api/types/filters"
	"github.com/docker/docker/api/types/image"
	"github.com/docker/docker/api/types/network"
	"github.com/docker/docker/api/types/registry"
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
	var f filters.Args
	if fj := r.URL.Query().Get("filters"); fj != "" {
		f, _ = filters.FromJSON(fj)
	}
	report, err := s.docker.ContainersPrune(r.Context(), f)
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
			NetworkID:           req.EndpointConfig.NetworkID,
			EndpointID:          req.EndpointConfig.EndpointID,
			Gateway:             req.EndpointConfig.Gateway,
			IPAddress:           req.EndpointConfig.IPAddress,
			IPPrefixLen:         req.EndpointConfig.IPPrefixLen,
			IPv6Gateway:         req.EndpointConfig.IPv6Gateway,
			GlobalIPv6Address:   req.EndpointConfig.GlobalIPv6Address,
			GlobalIPv6PrefixLen: req.EndpointConfig.GlobalIPv6PrefixLen,
			MacAddress:          req.EndpointConfig.MacAddress,
			Aliases:             req.EndpointConfig.Aliases,
			DriverOpts:          req.EndpointConfig.DriverOpts,
		}
		if req.EndpointConfig.IPAMConfig != nil {
			epConfig.IPAMConfig = &network.EndpointIPAMConfig{
				IPv4Address:  req.EndpointConfig.IPAMConfig.IPv4Address,
				IPv6Address:  req.EndpointConfig.IPAMConfig.IPv6Address,
				LinkLocalIPs: req.EndpointConfig.IPAMConfig.LinkLocalIPs,
			}
		}
	}

	err := s.docker.NetworkConnect(r.Context(), id, req.Container, epConfig)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

// handleVolumePrune removes unused volumes.
func (s *Server) handleVolumePrune(w http.ResponseWriter, r *http.Request) {
	var f filters.Args
	if fj := r.URL.Query().Get("filters"); fj != "" {
		f, _ = filters.FromJSON(fj)
	}
	report, err := s.docker.VolumesPrune(r.Context(), f)
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

	opts := image.ListOptions{All: all}
	if fj := r.URL.Query().Get("filters"); fj != "" {
		opts.Filters, _ = filters.FromJSON(fj)
	}
	images, err := s.docker.ImageList(r.Context(), opts)
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
			VirtualSize: img.VirtualSize,
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
	var f filters.Args
	if fj := r.URL.Query().Get("filters"); fj != "" {
		f, _ = filters.FromJSON(fj)
	}
	report, err := s.docker.ImagesPrune(r.Context(), f)
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
	opts := events.ListOptions{
		Since: r.URL.Query().Get("since"),
		Until: r.URL.Query().Get("until"),
	}
	if fj := r.URL.Query().Get("filters"); fj != "" {
		opts.Filters, _ = filters.FromJSON(fj)
	}
	eventsCh, errCh := s.docker.Events(r.Context(), opts)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	flusher, canFlush := w.(http.Flusher)

	enc := json.NewEncoder(w)
	for {
		select {
		case event, ok := <-eventsCh:
			if !ok {
				return
			}
			mapped := api.Event{
				Type:     string(event.Type),
				Action:   string(event.Action),
				Scope:    event.Scope,
				Time:     event.Time,
				TimeNano: event.TimeNano,
				Actor: api.EventActor{
					ID:         event.Actor.ID,
					Attributes: event.Actor.Attributes,
				},
			}
			enc.Encode(mapped)
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
		cs := &api.ContainerSummary{
			ID:         c.ID,
			Names:      c.Names,
			Image:      c.Image,
			ImageID:    c.ImageID,
			Command:    c.Command,
			Created:    c.Created,
			State:      c.State,
			Status:     c.Status,
			Labels:     c.Labels,
			SizeRw:     c.SizeRw,
			SizeRootFs: c.SizeRootFs,
			Mounts:     mapMountsFromDf(c.Mounts),
		}
		for _, p := range c.Ports {
			cs.Ports = append(cs.Ports, api.Port{
				IP:          p.IP,
				PrivatePort: p.PrivatePort,
				PublicPort:  p.PublicPort,
				Type:        p.Type,
			})
		}
		if c.NetworkSettings != nil && len(c.NetworkSettings.Networks) > 0 {
			nets := make(map[string]*api.EndpointSettings, len(c.NetworkSettings.Networks))
			for name, ep := range c.NetworkSettings.Networks {
				nets[name] = &api.EndpointSettings{
					NetworkID:   ep.NetworkID,
					EndpointID:  ep.EndpointID,
					Gateway:     ep.Gateway,
					IPAddress:   ep.IPAddress,
					IPPrefixLen: ep.IPPrefixLen,
					MacAddress:  ep.MacAddress,
				}
			}
			cs.NetworkSettings = &api.SummaryNetworkSettings{Networks: nets}
		}
		containers = append(containers, cs)
	}
	if containers == nil {
		containers = []*api.ContainerSummary{}
	}

	var images []*api.ImageSummary
	for _, img := range du.Images {
		images = append(images, &api.ImageSummary{
			ID:          img.ID,
			ParentID:    img.ParentID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     img.Created,
			Size:        img.Size,
			SharedSize:  img.SharedSize,
			VirtualSize: img.VirtualSize,
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
				Status:     v.Status,
			})
		}
	}
	if volumes == nil {
		volumes = []*api.Volume{}
	}

	var buildCache []*api.BuildCache
	for _, bc := range du.BuildCache {
		buildCache = append(buildCache, &api.BuildCache{
			ID:          bc.ID,
			Parent:      bc.Parent,
			Type:        bc.Type,
			Description: bc.Description,
			InUse:       bc.InUse,
			Shared:      bc.Shared,
			Size:        bc.Size,
			CreatedAt:   bc.CreatedAt.Format(time.RFC3339Nano),
			LastUsedAt:  bc.LastUsedAt.Format(time.RFC3339Nano),
			UsageCount:  bc.UsageCount,
		})
	}
	if buildCache == nil {
		buildCache = []*api.BuildCache{}
	}

	writeJSON(w, http.StatusOK, api.DiskUsageResponse{
		LayersSize: du.LayersSize,
		Images:     images,
		Containers: containers,
		Volumes:    volumes,
		BuildCache: buildCache,
	})
}

func mapMountsFromDf(mounts []types.MountPoint) []api.MountPoint {
	result := make([]api.MountPoint, 0, len(mounts))
	for _, m := range mounts {
		result = append(result, api.MountPoint{
			Type:        string(m.Type),
			Name:        m.Name,
			Source:      m.Source,
			Destination: m.Destination,
			Driver:      m.Driver,
			Mode:        m.Mode,
			RW:          m.RW,
			Propagation: string(m.Propagation),
		})
	}
	return result
}

// handleContainerUpdate updates container resources.
func (s *Server) handleContainerUpdate(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	var updateConfig container.UpdateConfig
	if err := json.NewDecoder(r.Body).Decode(&updateConfig); err != nil {
		writeError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}
	resp, err := s.docker.ContainerUpdate(r.Context(), id, updateConfig)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	writeJSON(w, http.StatusOK, resp)
}

// handleContainerChanges returns filesystem changes in a container.
func (s *Server) handleContainerChanges(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	changes, err := s.docker.ContainerDiff(r.Context(), id)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	var result []api.ContainerChangeItem
	for _, c := range changes {
		result = append(result, api.ContainerChangeItem{
			Kind: int(c.Kind),
			Path: c.Path,
		})
	}
	if result == nil {
		result = []api.ContainerChangeItem{}
	}
	writeJSON(w, http.StatusOK, result)
}

// handleContainerExport exports a container's filesystem as a tar archive.
func (s *Server) handleContainerExport(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	rc, err := s.docker.ContainerExport(r.Context(), id)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}

// handleContainerCommit creates a new image from a container's changes.
func (s *Server) handleContainerCommit(w http.ResponseWriter, r *http.Request) {
	containerID := r.URL.Query().Get("container")
	repo := r.URL.Query().Get("repo")
	tag := r.URL.Query().Get("tag")
	comment := r.URL.Query().Get("comment")
	author := r.URL.Query().Get("author")
	pause := r.URL.Query().Get("pause") != "false" && r.URL.Query().Get("pause") != "0"
	// BUG-560: Use r.URL.Query()["changes"] to get all values (Docker sends multiple params)
	changes := r.URL.Query()["changes"]

	resp, err := s.docker.ContainerCommit(r.Context(), containerID, container.CommitOptions{
		Reference: func() string {
			if tag != "" {
				return repo + ":" + tag
			}
			return repo
		}(),
		Comment: comment,
		Author:  author,
		Pause:   pause,
		Changes: changes,
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	writeJSON(w, http.StatusCreated, api.ContainerCommitResponse{ID: resp.ID})
}

// handleContainerResize resizes the TTY of a container.
func (s *Server) handleContainerResize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h, _ := strconv.ParseUint(r.URL.Query().Get("h"), 10, 32)
	cw, _ := strconv.ParseUint(r.URL.Query().Get("w"), 10, 32)
	err := s.docker.ContainerResize(r.Context(), id, container.ResizeOptions{
		Height: uint(h),
		Width:  uint(cw),
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent) // BUG-561
}

// handleExecResize resizes the TTY of an exec instance.
func (s *Server) handleExecResize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	h, _ := strconv.ParseUint(r.URL.Query().Get("h"), 10, 32)
	cw, _ := strconv.ParseUint(r.URL.Query().Get("w"), 10, 32)
	err := s.docker.ContainerExecResize(r.Context(), id, container.ResizeOptions{
		Height: uint(h),
		Width:  uint(cw),
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusNoContent) // BUG-561
}

// BUG-508: handleImagePush pushes an image to a registry via Docker SDK.
func (s *Server) handleImagePush(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}
	ref := name + ":" + tag

	// BUG-559: Read auth from X-Registry-Auth header, not query param
	authStr := r.Header.Get("X-Registry-Auth")

	resp, err := s.docker.ImagePush(r.Context(), ref, image.PushOptions{
		RegistryAuth: authStr,
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer resp.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp)
}

// BUG-509: handleImageSave exports multiple images as a tar archive.
func (s *Server) handleImageSave(w http.ResponseWriter, r *http.Request) {
	names := r.URL.Query()["names"]
	resp, err := s.docker.ImageSave(r.Context(), names)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer resp.Close()

	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp)
}

// BUG-509: handleImageSaveByName exports a single image as a tar archive.
func (s *Server) handleImageSaveByName(w http.ResponseWriter, r *http.Request) {
	name := r.PathValue("name")
	resp, err := s.docker.ImageSave(r.Context(), []string{name})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer resp.Close()

	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp)
}

// BUG-510: handleImageSearch searches Docker Hub for images.
func (s *Server) handleImageSearch(w http.ResponseWriter, r *http.Request) {
	term := r.URL.Query().Get("term")
	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}

	results, err := s.docker.ImageSearch(r.Context(), term, registry.SearchOptions{
		Limit: limit,
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	mapped := make([]map[string]any, 0, len(results))
	for _, r := range results {
		mapped = append(mapped, map[string]any{
			"name":         r.Name,
			"description":  r.Description,
			"star_count":   r.StarCount,
			"is_official":  r.IsOfficial,
			"is_automated": r.IsAutomated,
		})
	}
	writeJSON(w, http.StatusOK, mapped)
}

// BUG-511: handleImageBuild builds an image from a Dockerfile.
func (s *Server) handleImageBuild(w http.ResponseWriter, r *http.Request) {
	opts := types.ImageBuildOptions{
		Dockerfile: r.URL.Query().Get("dockerfile"),
		NoCache:    r.URL.Query().Get("nocache") == "1" || r.URL.Query().Get("nocache") == "true",
		Remove:     r.URL.Query().Get("rm") != "0" && r.URL.Query().Get("rm") != "false",
		PullParent: r.URL.Query().Get("pull") == "1" || r.URL.Query().Get("pull") == "true",
	}
	if opts.Dockerfile == "" {
		opts.Dockerfile = "Dockerfile"
	}
	if t := r.URL.Query().Get("t"); t != "" {
		opts.Tags = []string{t}
	}
	if buildArgs := r.URL.Query().Get("buildargs"); buildArgs != "" {
		var args map[string]*string
		if json.Unmarshal([]byte(buildArgs), &args) == nil {
			opts.BuildArgs = args
		}
	}
	if labelsJSON := r.URL.Query().Get("labels"); labelsJSON != "" {
		var labels map[string]string
		if json.Unmarshal([]byte(labelsJSON), &labels) == nil {
			opts.Labels = labels
		}
	}

	// Forward auth config if present
	if authConfigs := r.Header.Get("X-Registry-Config"); authConfigs != "" {
		if data, err := base64.URLEncoding.DecodeString(authConfigs); err == nil {
			var configs map[string]registry.AuthConfig
			if json.Unmarshal(data, &configs) == nil {
				opts.AuthConfigs = configs
			}
		}
	}

	resp, err := s.docker.ImageBuild(r.Context(), r.Body, opts)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer resp.Body.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, resp.Body)
}

// BUG-512: handlePutArchive uploads a tar archive to a container path.
func (s *Server) handlePutArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	path := r.URL.Query().Get("path")
	noOverwrite := r.URL.Query().Get("noOverwriteDirNonDir") == "1" || r.URL.Query().Get("noOverwriteDirNonDir") == "true"

	err := s.docker.CopyToContainer(r.Context(), id, path, r.Body, container.CopyToContainerOptions{
		AllowOverwriteDirWithFile: !noOverwrite,
	})
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	w.WriteHeader(http.StatusOK)
}

// BUG-512: handleHeadArchive returns file stat info for a container path.
func (s *Server) handleHeadArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pathVal := r.URL.Query().Get("path")

	stat, err := s.docker.ContainerStatPath(r.Context(), id, pathVal)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}

	statJSON, _ := json.Marshal(map[string]any{
		"name":  stat.Name,
		"size":  stat.Size,
		"mode":  stat.Mode,
		"mtime": stat.Mtime.Format(time.RFC3339Nano),
	})
	w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(statJSON))
	w.WriteHeader(http.StatusOK)
}

// BUG-512: handleGetArchive downloads a tar archive from a container path.
func (s *Server) handleGetArchive(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	pathVal := r.URL.Query().Get("path")

	rc, stat, err := s.docker.CopyFromContainer(r.Context(), id, pathVal)
	if err != nil {
		writeError(w, mapDockerError(err))
		return
	}
	defer rc.Close()

	statJSON, _ := json.Marshal(map[string]any{
		"name":  stat.Name,
		"size":  stat.Size,
		"mode":  stat.Mode,
		"mtime": stat.Mtime.Format(time.RFC3339Nano),
	})
	w.Header().Set("X-Docker-Container-Path-Stat", base64.StdEncoding.EncodeToString(statJSON))
	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	io.Copy(w, rc)
}

// Prevent unused import errors
var (
	_ = volume.ListOptions{}
)
