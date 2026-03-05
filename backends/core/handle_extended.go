package core

import (
	"crypto/sha256"
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

func (s *BaseServer) handleContainerResize(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	// BUG-495: Accept h/w query params and store on container
	h, _ := strconv.Atoi(r.URL.Query().Get("h"))
	rw, _ := strconv.Atoi(r.URL.Query().Get("w"))
	if h > 0 || rw > 0 {
		s.Store.Containers.Update(id, func(c *api.Container) {
			c.HostConfig.ConsoleSize = [2]uint{uint(h), uint(rw)}
		})
	}
	w.WriteHeader(http.StatusOK)
}

func (s *BaseServer) handleExecResize(w http.ResponseWriter, r *http.Request) {
	id := r.PathValue("id")
	exec, ok := s.Store.Execs.Get(id)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "exec instance", ID: id})
		return
	}
	// BUG-496: Accept h/w query params and store on exec's container
	h, _ := strconv.Atoi(r.URL.Query().Get("h"))
	rw, _ := strconv.Atoi(r.URL.Query().Get("w"))
	if h > 0 || rw > 0 {
		s.Store.Containers.Update(exec.ContainerID, func(c *api.Container) {
			c.HostConfig.ConsoleSize = [2]uint{uint(h), uint(rw)}
		})
	}
	w.WriteHeader(http.StatusOK)
}

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

	// BUG-504: Read ps_args query param for API parity
	_ = r.URL.Query().Get("ps_args")

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

	// BUG-497: Use container's PID instead of hardcoded "1"
	pid := fmt.Sprintf("%d", c.State.Pid)
	if c.State.Pid == 0 {
		pid = "1"
	}

	WriteJSON(w, http.StatusOK, api.ContainerTopResponse{
		Titles: []string{"UID", "PID", "PPID", "C", "STIME", "TTY", "TIME", "CMD"},
		Processes: [][]string{
			{"root", pid, "0", "0", "00:00", "?", "00:00:00", cmd},
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
	var spaceReclaimed uint64
	deleted := make([]string, 0, len(pruned))
	for _, c := range pruned {
		if rootPath, err := s.Drivers.Filesystem.RootPath(c.ID); err == nil && rootPath != "" {
			spaceReclaimed += uint64(DirSize(rootPath))
		}
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
		SpaceReclaimed:    spaceReclaimed,
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

	memLimit := int64(1073741824) // 1 GiB default
	if c.HostConfig.Memory > 0 {
		memLimit = c.HostConfig.Memory
	}

	if !c.State.Running {
		now := time.Now().UTC()
		WriteJSON(w, http.StatusOK, s.buildStatsEntry(id, now, "0001-01-01T00:00:00Z", memLimit))
		return
	}

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)

	enc := json.NewEncoder(w)
	preread := "0001-01-01T00:00:00Z"

	for {
		now := time.Now().UTC()
		entry := s.buildStatsEntry(id, now, preread, memLimit)
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
func (s *BaseServer) buildStatsEntry(containerID string, now time.Time, preread string, memLimit int64) map[string]any {
	var memUsage int64
	var cpuNanos int64
	var pids int

	if stats, err := s.Drivers.ProcessLifecycle.Stats(containerID); err == nil && stats != nil {
		memUsage = stats.MemoryUsage
		cpuNanos = stats.CPUNanos
		pids = stats.PIDs
	}

	systemNanos := now.UnixNano()

	// BUG-518: Load previous CPU reading for precpu_stats
	var prevCPU, prevSys int64
	if prev, ok := s.Store.PrevCPUStats.Load(containerID); ok {
		p := prev.(*prevCPUStats)
		prevCPU = p.CPUNanos
		prevSys = p.SystemCPUNanos
	}
	s.Store.PrevCPUStats.Store(containerID, &prevCPUStats{
		CPUNanos:       cpuNanos,
		SystemCPUNanos: systemNanos,
	})

	// BUG-517: Look up container name
	var name string
	if c, ok := s.Store.Containers.Get(containerID); ok {
		name = c.Name
	}

	return map[string]any{
		"id":   containerID, // BUG-517
		"name": name,        // BUG-517
		"read":    now.Format(time.RFC3339Nano),
		"preread": preread,
		"cpu_stats": map[string]any{
			"cpu_usage": map[string]any{
				"total_usage": cpuNanos,
			},
			"online_cpus":      1,
			"system_cpu_usage": systemNanos,
		},
		"precpu_stats": map[string]any{
			"cpu_usage": map[string]any{
				"total_usage": prevCPU, // BUG-518
			},
			"online_cpus":      1,
			"system_cpu_usage": prevSys, // BUG-518
		},
		"memory_stats": map[string]any{
			"usage": memUsage,
			"limit": memLimit,
		},
		"pids_stats": map[string]any{
			"current": pids,
		},
		"networks": s.buildNetworkStats(containerID),
	}
}

// prevCPUStats holds the previous CPU stats reading for a container.
type prevCPUStats struct {
	CPUNanos       int64
	SystemCPUNanos int64
}

// buildNetworkStats returns per-network zero-value stats for a container.
func (s *BaseServer) buildNetworkStats(containerID string) map[string]any {
	netStats := make(map[string]any)
	if c, ok := s.Store.Containers.Get(containerID); ok {
		for netName := range c.NetworkSettings.Networks {
			netStats[netName] = map[string]any{
				"rx_bytes":   0,
				"rx_packets": 0,
				"rx_errors":  0,
				"rx_dropped": 0,
				"tx_bytes":   0,
				"tx_packets": 0,
				"tx_errors":  0,
				"tx_dropped": 0,
			}
		}
	}
	return netStats
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

	// BUG-520: Parse since/until query params (Unix timestamp or RFC3339)
	sinceTS := parseEventTimestamp(r.URL.Query().Get("since"))
	untilTS := parseEventTimestamp(r.URL.Query().Get("until"))

	subID := GenerateID()[:16]
	ch := s.EventBus.Subscribe(subID)
	defer s.EventBus.Unsubscribe(subID)

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}

	enc := json.NewEncoder(w)

	matchEvent := func(event api.Event) bool {
		if !matchEventFilter(typeFilter, event.Type) {
			return false
		}
		if !matchEventFilter(actionFilter, event.Action) {
			return false
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
				return false
			}
		}
		if len(labelFilter) > 0 && !MatchLabels(event.Actor.Attributes, labelFilter) {
			return false
		}
		return true
	}

	// BUG-520: Replay historical events if since is set
	if sinceTS > 0 {
		for _, event := range s.EventBus.History(sinceTS, untilTS) {
			if matchEvent(event) {
				_ = enc.Encode(event)
			}
		}
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}
		// If until is in the past, stop immediately
		if untilTS > 0 && untilTS <= time.Now().Unix() {
			return
		}
	}

	// Set up until timer if specified and in the future
	var untilCh <-chan time.Time
	if untilTS > 0 {
		d := time.Until(time.Unix(untilTS, 0))
		if d > 0 {
			untilCh = time.After(d)
		} else {
			return
		}
	}

	for {
		select {
		case event, ok := <-ch:
			if !ok {
				return
			}
			if matchEvent(event) {
				_ = enc.Encode(event)
				if f, ok := w.(http.Flusher); ok {
					f.Flush()
				}
			}
		case <-r.Context().Done():
			return
		case <-func() <-chan time.Time {
			if untilCh != nil {
				return untilCh
			}
			return nil
		}():
			return
		}
	}
}

// parseEventTimestamp parses a Docker event timestamp (Unix seconds or RFC3339).
func parseEventTimestamp(s string) int64 {
	if s == "" {
		return 0
	}
	// Try integer (Unix seconds) first
	if n, err := strconv.ParseInt(s, 10, 64); err == nil {
		return n
	}
	// Try float (Unix seconds with nanos)
	if f, err := strconv.ParseFloat(s, 64); err == nil {
		return int64(f)
	}
	// Try RFC3339
	if t, err := time.Parse(time.RFC3339, s); err == nil {
		return t.Unix()
	}
	return 0
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
		if req.PidsLimit != nil {
			c.HostConfig.PidsLimit = req.PidsLimit
		}
		if req.OomKillDisable != nil {
			c.HostConfig.OomKillDisable = req.OomKillDisable
		}
	})

	c, _ := s.Store.Containers.Get(id)
	s.emitEvent("container", "update", id, map[string]string{
		"name": strings.TrimPrefix(c.Name, "/"),
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
	// BUG-436: Build image→container count map
	imgContainerCount := make(map[string]int64)
	for _, c := range s.Store.Containers.List() {
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			imgContainerCount[img.ID]++
		}
	}

	var images []*api.ImageSummary
	for _, img := range s.Store.Images.List() {
		created, _ := time.Parse(time.RFC3339Nano, img.Created)
		images = append(images, &api.ImageSummary{
			ID:          img.ID,
			RepoTags:    img.RepoTags,
			RepoDigests: img.RepoDigests,
			Created:     created.Unix(),
			Size:        img.Size,
			VirtualSize: img.VirtualSize,   // BUG-450
			Labels:      img.Config.Labels, // BUG-451
			Containers:  imgContainerCount[img.ID],
		})
	}

	var containers []*api.ContainerSummary
	for _, c := range s.Store.Containers.List() {
		created, _ := time.Parse(time.RFC3339Nano, c.Created)
		command := c.Path
		if len(c.Args) > 0 {
			command += " " + strings.Join(c.Args, " ")
		}
		// BUG-456: Resolve image ID
		imageID := ""
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			imageID = img.ID
		} else {
			h := sha256.Sum256([]byte(c.Config.Image))
			imageID = fmt.Sprintf("sha256:%x", h)
		}
		// BUG-459: Labels
		labels := c.Config.Labels
		if labels == nil {
			labels = make(map[string]string)
		}
		// BUG-460: Mounts
		mounts := c.Mounts
		if mounts == nil {
			mounts = []api.MountPoint{}
		}
		cs := &api.ContainerSummary{
			ID:      c.ID,
			Names:   []string{c.Name},
			Image:   c.Config.Image,
			ImageID: imageID,                                                           // BUG-456
			Command: command,                                                           // BUG-457
			Created: created.Unix(),
			State:   c.State.Status,
			Status:  FormatStatus(c.State),                                             // BUG-458
			Labels:  labels,                                                            // BUG-459
			Ports:   buildPortList(c.HostConfig.PortBindings, c.Config.ExposedPorts),    // BUG-459
			Mounts:  mounts,                                                            // BUG-460
			NetworkSettings: &api.SummaryNetworkSettings{Networks: c.NetworkSettings.Networks}, // BUG-460
			HostConfig:      &api.HostConfigSummary{NetworkMode: c.HostConfig.NetworkMode},     // BUG-460
		}
		// Calculate real container size from container rootDir
		if rootPath, err := s.Drivers.Filesystem.RootPath(c.ID); err == nil && rootPath != "" {
			cs.SizeRw = DirSize(rootPath)
		}
		// BUG-461: SizeRootFs from image
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			cs.SizeRootFs = img.Size
		}
		containers = append(containers, cs)
	}

	// BUG-449: Build volume→container reference count map
	volRefCount := make(map[string]int64)
	for _, c := range s.Store.Containers.List() {
		for _, m := range c.Mounts {
			if m.Name != "" {
				volRefCount[m.Name]++
			}
		}
	}

	var volumes []*api.Volume
	for _, v := range s.Store.Volumes.List() {
		vCopy := v
		// Calculate real volume size from temp dir
		size := int64(-1)
		if dir, ok := s.Store.VolumeDirs.Load(v.Name); ok {
			size = DirSize(dir.(string))
			vCopy.Status = map[string]any{"Size": size}
		}
		vCopy.UsageData = &api.VolumeUsageData{
			RefCount: volRefCount[v.Name],
			Size:     size,
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
