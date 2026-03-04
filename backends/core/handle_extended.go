package core

import (
	"encoding/json"
	"fmt"
	"io"
	"net/http"
	"os"
	"strconv"
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
	filters := ParseFilters(r.URL.Query().Get("filters"))
	labelFilters := filters["label"]
	untilFilters := filters["until"]

	pruned := s.Store.Containers.PruneIf(func(_ string, c api.Container) bool {
		if c.State.Status != "exited" && c.State.Status != "dead" {
			return false
		}
		if len(labelFilters) > 0 && !MatchLabels(c.Config.Labels, labelFilters) {
			return false
		}
		if len(untilFilters) > 0 && !MatchUntil(c.Created, untilFilters) {
			return false
		}
		return true
	})
	deleted := make([]string, 0, len(pruned))
	for _, c := range pruned {
		s.StopHealthCheck(c.ID)
		s.Store.ContainerNames.Delete(c.Name)
		s.Store.LogBuffers.Delete(c.ID)
		if ch, ok := s.Store.WaitChs.LoadAndDelete(c.ID); ok {
			close(ch.(chan struct{}))
		}
		s.Drivers.ProcessLifecycle.Cleanup(c.ID)
		for _, ep := range c.NetworkSettings.Networks {
			if ep != nil && ep.NetworkID != "" {
				_ = s.Drivers.Network.Disconnect(r.Context(), ep.NetworkID, c.ID)
			}
		}
		if pod, inPod := s.Store.Pods.GetPodForContainer(c.ID); inPod {
			s.Store.Pods.RemoveContainer(pod.ID, c.ID)
		}
		s.Store.StagingDirs.Delete(c.ID)
		if dirs, ok := s.Store.TmpfsDirs.LoadAndDelete(c.ID); ok {
			for _, d := range dirs.([]string) {
				os.RemoveAll(d)
			}
		}
		for _, eid := range c.ExecIDs {
			s.Store.Execs.Delete(eid)
		}
		s.emitEvent("container", "destroy", c.ID, map[string]string{
			"name": strings.TrimPrefix(c.Name, "/"),
		})
		deleted = append(deleted, c.ID)
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
	stream := r.URL.Query().Get("stream") != "false"

	if !c.State.Running {
		now := time.Now().UTC()
		WriteJSON(w, http.StatusOK, s.buildStatsEntry(id, now, "0001-01-01T00:00:00Z"))
		return
	}

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
			// Stop streaming if container is no longer running (BUG-209)
			if cur, ok := s.Store.Containers.Get(id); !ok || !cur.State.Running {
				return
			}
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

	// Serialize renames to prevent race between conflict check and swap
	s.Store.RenameMu.Lock()
	defer s.Store.RenameMu.Unlock()

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

	// Update name in each network's Containers map (BUG-210)
	c, _ = s.Store.Containers.Get(id)
	for _, ep := range c.NetworkSettings.Networks {
		if ep != nil && ep.NetworkID != "" {
			s.Store.Networks.Update(ep.NetworkID, func(n *api.Network) {
				if er, ok := n.Containers[id]; ok {
					er.Name = strings.TrimPrefix(newName, "/")
					n.Containers[id] = er
				}
			})
		}
	}

	s.emitEvent("container", "rename", id, map[string]string{
		"name":    strings.TrimPrefix(newName, "/"),
		"oldName": strings.TrimPrefix(oldName, "/"),
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

	// Atomic check-and-set inside Update to prevent TOCTOU race
	var name string
	var conflict error
	s.Store.Containers.Update(id, func(c *api.Container) {
		if c.State.Paused {
			conflict = &api.ConflictError{
				Message: fmt.Sprintf("Container %s is already paused", ref),
			}
			return
		}
		if !c.State.Running {
			conflict = &api.ConflictError{
				Message: fmt.Sprintf("Container %s is not running", ref),
			}
			return
		}
		c.State.Paused = true
		c.State.Status = "paused"
		name = c.Name
	})
	if conflict != nil {
		WriteError(w, conflict)
		return
	}

	s.StopHealthCheck(id)

	s.emitEvent("container", "pause", id, map[string]string{
		"name": strings.TrimPrefix(name, "/"),
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

	// Atomic check-and-set inside Update to prevent TOCTOU race
	var name string
	var hasHealthcheck bool
	var conflict error
	s.Store.Containers.Update(id, func(c *api.Container) {
		if !c.State.Paused {
			conflict = &api.ConflictError{
				Message: fmt.Sprintf("Container %s is not paused", ref),
			}
			return
		}
		c.State.Paused = false
		c.State.Status = "running"
		name = c.Name
		hasHealthcheck = c.Config.Healthcheck != nil && len(c.Config.Healthcheck.Test) > 0 &&
			(len(c.Config.Healthcheck.Test) != 1 || !strings.EqualFold(c.Config.Healthcheck.Test[0], "NONE"))
	})
	if conflict != nil {
		WriteError(w, conflict)
		return
	}

	if hasHealthcheck {
		s.StartHealthCheck(id)
	}

	s.emitEvent("container", "unpause", id, map[string]string{
		"name": strings.TrimPrefix(name, "/"),
	})

	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleSystemEvents(w http.ResponseWriter, r *http.Request) {
	evFilters := ParseFilters(r.URL.Query().Get("filters"))
	typeFilter := evFilters["type"]
	actionFilter := evFilters["action"]
	containerFilter := evFilters["container"]
	labelFilter := evFilters["label"]

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
			if !matchEventFilter(typeFilter, event.Type) {
				continue
			}
			if !matchEventFilter(actionFilter, event.Action) {
				continue
			}
			if len(containerFilter) > 0 {
				matched := false
				for _, cf := range containerFilter {
					if event.Actor.ID == cf || event.Actor.Attributes["name"] == cf {
						matched = true
						break
					}
				}
				if !matched {
					continue
				}
			}
			if len(labelFilter) > 0 && !MatchLabels(event.Actor.Attributes, labelFilter) {
				continue
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

// matchEventFilter returns true if no filter is set or value matches one of the filter values.
func matchEventFilter(filter []string, value string) bool {
	if len(filter) == 0 {
		return true
	}
	for _, f := range filter {
		if value == f {
			return true
		}
	}
	return false
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
		// Empty body is fine — return success. Malformed JSON is 400.
		if err == io.EOF || r.ContentLength == 0 {
			WriteJSON(w, http.StatusOK, api.ContainerUpdateResponse{Warnings: []string{}})
			return
		}
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	s.Store.Containers.Update(id, func(c *api.Container) {
		if req.RestartPolicy.Name != "" {
			c.HostConfig.RestartPolicy = req.RestartPolicy
		}
		if req.Memory != 0 {
			c.HostConfig.Memory = req.Memory
		}
		if req.MemorySwap != 0 {
			c.HostConfig.MemorySwap = req.MemorySwap
		}
		if req.MemoryReservation != 0 {
			c.HostConfig.MemoryReservation = req.MemoryReservation
		}
		if req.CpuShares != 0 {
			c.HostConfig.CpuShares = req.CpuShares
		}
		if req.CpuQuota != 0 {
			c.HostConfig.CpuQuota = req.CpuQuota
		}
		if req.CpuPeriod != 0 {
			c.HostConfig.CpuPeriod = req.CpuPeriod
		}
		if req.CpusetCpus != "" {
			c.HostConfig.CpusetCpus = req.CpusetCpus
		}
		if req.CpusetMems != "" {
			c.HostConfig.CpusetMems = req.CpusetMems
		}
		if req.BlkioWeight != 0 {
			c.HostConfig.BlkioWeight = req.BlkioWeight
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

	// Deduplicate images by ID (BUG-211)
	seen := make(map[string]bool, len(images))
	dedupImages := make([]*api.ImageSummary, 0, len(images))
	for _, img := range images {
		if seen[img.ID] {
			continue
		}
		seen[img.ID] = true
		dedupImages = append(dedupImages, img)
	}

	WriteJSON(w, http.StatusOK, api.DiskUsageResponse{
		Images:     dedupImages,
		Containers: containers,
		Volumes:    volumes,
		BuildCache: []*api.BuildCache{},
	})
}

// MatchLabels checks whether a container's labels satisfy label filter expressions.
// Each filter is "key" (key must exist) or "key=value" (key must equal value).
func MatchLabels(labels map[string]string, filters []string) bool {
	for _, f := range filters {
		if k, v, ok := strings.Cut(f, "="); ok {
			if labels[k] != v {
				return false
			}
		} else {
			if _, exists := labels[f]; !exists {
				return false
			}
		}
	}
	return true
}

// MatchUntil checks whether a container was created before the given until timestamps.
func MatchUntil(created string, filters []string) bool {
	ct, err := time.Parse(time.RFC3339Nano, created)
	if err != nil {
		return false
	}
	for _, f := range filters {
		// Try as Unix timestamp first
		if secs, err := strconv.ParseInt(f, 10, 64); err == nil {
			if !ct.Before(time.Unix(secs, 0)) {
				return false
			}
			continue
		}
		// Try as RFC3339
		if t, err := time.Parse(time.RFC3339Nano, f); err == nil {
			if !ct.Before(t) {
				return false
			}
			continue
		}
	}
	return true
}
