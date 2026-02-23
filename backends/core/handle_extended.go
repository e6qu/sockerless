package core

import (
	"encoding/json"
	"fmt"
	"net/http"
	"strings"
	"time"

	"github.com/sockerless/api"
)

func (s *BaseServer) handleContainerTop(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		})
		return
	}

	// Get process data via driver chain
	entries, _ := s.Drivers.ProcessLifecycle.Top(id)
	if len(entries) > 0 {
		var processes [][]string
		for _, e := range entries {
			processes = append(processes, []string{
				"root",
				fmt.Sprintf("%d", e.PID),
				"0",
				"0",
				"00:00",
				"?",
				"00:00:00",
				e.Command,
			})
		}

		WriteJSON(w, http.StatusOK, api.ContainerTopResponse{
			Titles:    []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
			Processes: processes,
		})
		return
	}

	// Synthetic process list fallback
	cmd := c.Path
	if len(c.Args) > 0 {
		cmd += " " + strings.Join(c.Args, " ")
	}

	WriteJSON(w, http.StatusOK, api.ContainerTopResponse{
		Titles: []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
		Processes: [][]string{
			{"root", "1", "0", "0", "00:00", "?", "00:00:00", cmd},
		},
	})
}

func (s *BaseServer) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	var deleted []string
	for _, c := range s.Store.Containers.List() {
		if c.State.Status == "exited" || c.State.Status == "dead" {
			s.Store.Containers.Delete(c.ID)
			s.Store.ContainerNames.Delete(c.Name)
			s.Store.LogBuffers.Delete(c.ID)
			s.Store.WaitChs.Delete(c.ID)
			s.Drivers.ProcessLifecycle.Cleanup(c.ID)
			deleted = append(deleted, c.ID)
		}
	}
	if deleted == nil {
		deleted = []string{}
	}
	WriteJSON(w, http.StatusOK, api.ContainerPruneResponse{
		ContainersDeleted: deleted,
		SpaceReclaimed:    0,
	})
}

func (s *BaseServer) handleContainerStats(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		})
		return
	}

	// stream=true (default) sends JSON lines every 1s; stream=false sends one snapshot
	stream := r.URL.Query().Get("stream") != "false"

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	preread := "0001-01-01T00:00:00Z"

	for {
		now := time.Now().UTC()
		entry := s.buildStatsEntry(id, now, preread)
		_ = enc.Encode(entry)
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		if !stream {
			return
		}

		preread = now.Format(time.RFC3339Nano)

		select {
		case <-r.Context().Done():
			return
		case <-time.After(1 * time.Second):
		}
	}
}

// buildStatsEntry constructs a Docker-compatible stats JSON object.
func (s *BaseServer) buildStatsEntry(containerID string, now time.Time, preread string) map[string]any {
	var memUsage int64
	var cpuNanos int64
	var pids int

	if stats, err := s.Drivers.ProcessLifecycle.Stats(containerID); err == nil && stats != nil {
		memUsage = stats.MemoryUsage
		cpuNanos = stats.CPUNanos
		pids = stats.PIDs
	}

	return map[string]any{
		"read":    now.Format(time.RFC3339Nano),
		"preread": preread,
		"cpu_stats": map[string]any{
			"cpu_usage": map[string]any{
				"total_usage": cpuNanos,
			},
			"online_cpus":      1,
			"system_cpu_usage": now.UnixNano(),
		},
		"memory_stats": map[string]any{
			"usage": memUsage,
			"limit": int64(1073741824), // 1 GiB
		},
		"pids_stats": map[string]any{
			"current": pids,
		},
		"networks": map[string]any{},
	}
}

func (s *BaseServer) handleContainerRename(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	newName := r.URL.Query().Get("name")
	if newName == "" {
		WriteError(w, &api.InvalidParameterError{Message: "name is required"})
		return
	}
	if !strings.HasPrefix(newName, "/") {
		newName = "/" + newName
	}

	c, _ := s.Store.Containers.Get(id)
	oldName := c.Name

	// Check for conflicts
	if _, exists := s.Store.ContainerNames.Get(newName); exists {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Conflict. The container name \"%s\" is already in use", strings.TrimPrefix(newName, "/")),
		})
		return
	}

	s.Store.ContainerNames.Delete(oldName)
	s.Store.ContainerNames.Put(newName, id)
	s.Store.Containers.Update(id, func(c *api.Container) {
		c.Name = newName
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerPause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Running {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not running", ref),
		})
		return
	}

	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Paused = true
		c.State.Status = "paused"
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)
	if !c.State.Paused {
		WriteError(w, &api.ConflictError{
			Message: fmt.Sprintf("Container %s is not paused", ref),
		})
		return
	}

	s.Store.Containers.Update(id, func(c *api.Container) {
		c.State.Paused = false
		c.State.Status = "running"
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleSystemEvents(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))
	typeFilter := filters["type"]

	subID := GenerateID()[:16]
	ch := s.EventBus.Subscribe(subID)
	defer s.EventBus.Unsubscribe(subID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	enc := json.NewEncoder(w)
	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			if len(typeFilter) > 0 {
				matched := false
				for _, t := range typeFilter {
					if event.Type == t {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			_ = enc.Encode(event)
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		case <-r.Context().Done():
			return
		}
	}
}

func (s *BaseServer) handleContainerUpdate(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	var req api.ContainerUpdateRequest
	if err := ReadJSON(r, &req); err != nil {
		// Empty body is fine — just return success with no changes
		WriteJSON(w, http.StatusOK, api.ContainerUpdateResponse{Warnings: []string{}})
		return
	}

	s.Store.Containers.Update(id, func(c *api.Container) {
		if req.RestartPolicy.Name != "" {
			c.HostConfig.RestartPolicy = req.RestartPolicy
		}
	})

	WriteJSON(w, http.StatusOK, api.ContainerUpdateResponse{Warnings: []string{}})
}

func (s *BaseServer) handleContainerChanges(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	_, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	WriteJSON(w, http.StatusOK, []api.ContainerChangeItem{})
}

func (s *BaseServer) handleSystemDf(w http.ResponseWriter, r *http.Request) {
	var images []*api.ImageSummary
	for _, img := range s.Store.Images.List() {
		created, _ := time.Parse(time.RFC3339Nano, img.Created)
		images = append(images, &api.ImageSummary{
			ID:          img.ID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     created.Unix(),
			Size:        img.Size,
		})
	}

	var containers []*api.ContainerSummary
	for _, c := range s.Store.Containers.List() {
		created, _ := time.Parse(time.RFC3339Nano, c.Created)
		cs := &api.ContainerSummary{
			ID:      c.ID,
			Names:   []string{c.Name},
			Image:   c.Config.Image,
			Created: created.Unix(),
			State:   c.State.Status,
		}
		// Calculate real container size from container rootDir
		if rootPath, err := s.Drivers.Filesystem.RootPath(c.ID); err == nil && rootPath != "" {
			cs.SizeRw = DirSize(rootPath)
		}
		containers = append(containers, cs)
	}

	var volumes []*api.Volume
	for _, v := range s.Store.Volumes.List() {
		vCopy := v
		// Calculate real volume size from temp dir
		if dir, ok := s.Store.VolumeDirs.Load(v.Name); ok {
			size := DirSize(dir.(string))
			vCopy.Status = map[string]any{"Size": size}
		}
		volumes = append(volumes, &vCopy)
	}

	WriteJSON(w, http.StatusOK, api.DiskUsageResponse{
		Images:     images,
		Containers: containers,
		Volumes:    volumes,
	})
}
