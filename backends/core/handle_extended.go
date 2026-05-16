package core

import (
	"context"
	"encoding/json"
	"io"
	"net/http"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
)

func (s *BaseServer) handleContainerResize(w http.ResponseWriter, r *http.Request) {
	h, _ := strconv.Atoi(r.URL.Query().Get("h"))
	rw, _ := strconv.Atoi(r.URL.Query().Get("w"))
	if err := s.self.ContainerResize(r.PathValue("id"), h, rw); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *BaseServer) handleExecResize(w http.ResponseWriter, r *http.Request) {
	h, _ := strconv.Atoi(r.URL.Query().Get("h"))
	rw, _ := strconv.Atoi(r.URL.Query().Get("w"))
	if err := s.self.ExecResize(r.PathValue("id"), h, rw); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusOK)
}

func (s *BaseServer) handleContainerTop(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	psArgs := r.URL.Query().Get("ps_args")
	c, _ := s.ResolveContainerAuto(r.Context(), ref)
	if c.ID == "" {
		c.ID = ref
	}
	dctx := DriverContext{
		Ctx:       r.Context(),
		Container: c,
		Backend:   s.Desc.Driver,
		Logger:    s.Logger,
	}
	resp, err := s.Typed.ProcList.Top(dctx, psArgs)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handleContainerPrune(w http.ResponseWriter, r *http.Request) {
	filters := ParseFilters(r.URL.Query().Get("filters"))

	resp, err := s.self.ContainerPrune(filters)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handleContainerStats(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	c, ok := s.ResolveContainerAuto(r.Context(), ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	id := c.ID
	// Docker clients send the stream flag as "true" / "false" (newer
	// SDKs) or "1" / "0" (older). Normalise both forms — anything that
	// parses as "no stream" is treated as one-shot.
	streamRaw := r.URL.Query().Get("stream")
	stream := streamRaw != "false" && streamRaw != "0"

	// Streaming `docker stats` is an accepted gap on cloud backends
	// (see specs/CLOUD_RESOURCE_MAPPING.md § Acceptable gaps): cloud
	// metrics surface with 30–60 s+ lag, so a "stream" would be a
	// high-latency polling reskin that misleads callers. Local docker
	// backend overrides this handler with a real streaming impl. For
	// every cloud backend (CloudState set), fall back to a single
	// snapshot regardless of the stream flag.
	if stream && s.CloudState != nil {
		WriteError(w, &api.NotImplementedError{Message: "streaming `docker stats` is not supported on cloud backends — use `docker stats --no-stream` for one-shot metrics (cloud metrics lag 30-60s)"})
		return
	}

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
	now := time.Now().UTC()
	entry := s.buildStatsEntry(id, now, "0001-01-01T00:00:00Z", memLimit)
	_ = enc.Encode(entry)
	if f, ok := w.(http.Flusher); ok {
		f.Flush()
	}
}

// buildStatsEntry constructs a Docker-compatible stats JSON object.
// Uses StatsProvider for real metrics when available.
func (s *BaseServer) buildStatsEntry(containerID string, now time.Time, preread string, memLimit int64) map[string]any {
	var memUsage int64
	var cpuNanos int64
	var pids int

	// Fetch real metrics from cloud provider if available.
	if s.StatsProvider != nil {
		if m, err := s.StatsProvider.ContainerMetrics(containerID); err == nil && m != nil {
			cpuNanos = m.CPUNanos
			memUsage = m.MemBytes
			pids = m.PIDs
		}
	}

	systemNanos := now.UnixNano()

	// Load previous CPU reading for precpu_stats
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

	// Look up container name (try cloud state first for stateless backends)
	var name string
	if c, ok := s.ResolveContainerAuto(context.Background(), containerID); ok {
		name = c.Name
	}

	return map[string]any{
		"id":      containerID,
		"name":    name,
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
				"total_usage": prevCPU,
			},
			"online_cpus":      1,
			"system_cpu_usage": prevSys,
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
	newName := r.URL.Query().Get("name")
	if newName == "" {
		WriteError(w, &api.InvalidParameterError{Message: "name is required"})
		return
	}
	// Route through s.self.ContainerRename so per-backend overrides
	// (e.g. ECS pushing the new name to the task's `sockerless-name`
	// tag) run. The base implementation handles in-memory Store updates
	// and the network-name-map sync; backends wrap it.
	if err := s.self.ContainerRename(ref, newName); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerPause(w http.ResponseWriter, r *http.Request) {
	if err := s.self.ContainerPause(r.PathValue("id")); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleContainerUnpause(w http.ResponseWriter, r *http.Request) {
	if err := s.self.ContainerUnpause(r.PathValue("id")); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusNoContent)
}

func (s *BaseServer) handleSystemEvents(w http.ResponseWriter, r *http.Request) {
	evFilters := ParseFilters(r.URL.Query().Get("filters"))
	typeFilter := evFilters["type"]
	actionFilter := evFilters["action"]
	containerFilter := evFilters["container"]
	labelFilter := evFilters["label"]

	// Parse since/until query params (Unix timestamp or RFC3339)
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

	// Replay historical events if since is set
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
	var req api.ContainerUpdateRequest
	if err := ReadJSON(r, &req); err != nil {
		if err == io.EOF || r.ContentLength == 0 {
			WriteJSON(w, http.StatusOK, api.ContainerUpdateResponse{Warnings: []string{}})
			return
		}
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	resp, err := s.self.ContainerUpdate(r.PathValue("id"), &req)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
}

func (s *BaseServer) handleContainerChanges(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	c, _ := s.ResolveContainerAuto(r.Context(), ref)
	if c.ID == "" {
		c.ID = ref
	}
	dctx := DriverContext{
		Ctx:       r.Context(),
		Container: c,
		Backend:   s.Desc.Driver,
		Logger:    s.Logger,
	}
	result, err := s.Typed.FSDiff.Changes(dctx)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

// collectContainers returns the "current truth" view: CloudState is the source
// when present (stateless cloud backends), otherwise the local Store covers
// passthrough backends. PendingCreates are folded in either way so in-flight
// containers stay visible. The `all` flag and `filters` apply to PendingCreates
// gating; CloudState honors them via its own ListContainers signature.
func (s *BaseServer) collectContainers(ctx context.Context, all bool, filters map[string][]string) []api.Container {
	seen := map[string]bool{}
	var result []api.Container

	if s.CloudState != nil {
		if cc, err := s.CloudState.ListContainers(ctx, all, filters); err == nil {
			for _, c := range cc {
				if !seen[c.ID] {
					seen[c.ID] = true
					result = append(result, c)
				}
			}
		}
	} else {
		for _, c := range s.Store.Containers.List() {
			if !seen[c.ID] {
				seen[c.ID] = true
				result = append(result, c)
			}
		}
	}

	if s.PendingCreates != nil {
		for _, pc := range s.PendingCreates.List() {
			if seen[pc.ID] {
				continue
			}
			if !all && !pc.State.Running {
				continue
			}
			seen[pc.ID] = true
			result = append(result, pc)
		}
	}

	return result
}

// collectAllContainers returns the union of Store + CloudState + PendingCreates,
// Store first (Store wins on duplicate IDs). Used by SystemDf for disk-usage
// accounting where Store may carry tags / labels CloudState doesn't.
func (s *BaseServer) collectAllContainers(ctx context.Context) []api.Container {
	seen := map[string]bool{}
	var result []api.Container

	for _, c := range s.Store.Containers.List() {
		if !seen[c.ID] {
			seen[c.ID] = true
			result = append(result, c)
		}
	}
	if s.CloudState != nil {
		if cc, err := s.CloudState.ListContainers(ctx, true, nil); err == nil {
			for _, c := range cc {
				if !seen[c.ID] {
					seen[c.ID] = true
					result = append(result, c)
				}
			}
		}
	}
	if s.PendingCreates != nil {
		for _, c := range s.PendingCreates.List() {
			if !seen[c.ID] {
				seen[c.ID] = true
				result = append(result, c)
			}
		}
	}
	return result
}

func (s *BaseServer) handleSystemDf(w http.ResponseWriter, r *http.Request) {
	resp, err := s.self.SystemDf()
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, resp)
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
