package core

import (
	"crypto/sha256"
	"fmt"
	"net/http"
	"sort"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
)

// --- Common container query/streaming handlers ---

func (s *BaseServer) handleContainerInspect(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	c, ok := s.Store.ResolveContainer(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	WriteJSON(w, http.StatusOK, c)
}

func (s *BaseServer) handleContainerList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	// BUG-394: Read size query parameter
	includeSize := r.URL.Query().Get("size") == "1" || r.URL.Query().Get("size") == "true"
	filters := ParseFilters(r.URL.Query().Get("filters"))
	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}

	var result []*api.ContainerSummary
	for _, c := range s.Store.Containers.List() {
		if !all && !c.State.Running {
			continue
		}
		if !MatchContainerFilters(c, filters) {
			continue
		}

		command := c.Path
		if len(c.Args) > 0 {
			command += " " + strings.Join(c.Args, " ")
		}

		created, _ := time.Parse(time.RFC3339Nano, c.Created)
		status := FormatStatus(c.State)

		imageID := ""
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			imageID = img.ID
		} else {
			h := sha256.Sum256([]byte(c.Config.Image))
			imageID = fmt.Sprintf("sha256:%x", h)
		}

		labels := c.Config.Labels
		if labels == nil {
			labels = make(map[string]string)
		}
		mounts := c.Mounts
		if mounts == nil {
			mounts = []api.MountPoint{}
		}
		summary := &api.ContainerSummary{
			ID:      c.ID,
			Names:   []string{c.Name},
			Image:   c.Config.Image,
			ImageID: imageID,
			Command: command,
			Created: created.Unix(),
			State:   c.State.Status,
			Status:  status,
			Ports:   buildPortList(c.HostConfig.PortBindings, c.Config.ExposedPorts),
			Labels:  labels,
			HostConfig: &api.HostConfigSummary{
				NetworkMode: c.HostConfig.NetworkMode,
			},
			Mounts:  mounts,
			NetworkSettings: &api.SummaryNetworkSettings{
				Networks: c.NetworkSettings.Networks,
			},
		}
		// BUG-394: Populate size fields when requested
		if includeSize {
			if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
				summary.SizeRootFs = img.Size
			}
			// BUG-452: Populate SizeRw from container root dir
			if rootPath, err := s.Drivers.Filesystem.RootPath(c.ID); err == nil && rootPath != "" {
				summary.SizeRw = DirSize(rootPath)
			}
		}
		result = append(result, summary)
	}

	// Sort by Created descending (newest first), matching Docker API behavior
	sort.Slice(result, func(i, j int) bool {
		return result[i].Created > result[j].Created
	})

	// Apply before/since post-filters (need Store access to resolve references)
	if beforeRef := filters["before"]; len(beforeRef) > 0 {
		if bc, ok := s.Store.ResolveContainer(beforeRef[0]); ok {
			beforeTime, err := time.Parse(time.RFC3339Nano, bc.Created)
			if err != nil {
				beforeTime, _ = time.Parse(time.RFC3339, bc.Created)
			}
			var filtered []*api.ContainerSummary
			for _, cs := range result {
				if cs.Created < beforeTime.Unix() {
					filtered = append(filtered, cs)
				}
			}
			result = filtered
		}
	}
	if sinceRef := filters["since"]; len(sinceRef) > 0 {
		if sc, ok := s.Store.ResolveContainer(sinceRef[0]); ok {
			sinceTime, err := time.Parse(time.RFC3339Nano, sc.Created)
			if err != nil {
				sinceTime, _ = time.Parse(time.RFC3339, sc.Created)
			}
			var filtered []*api.ContainerSummary
			for _, cs := range result {
				if cs.Created > sinceTime.Unix() {
					filtered = append(filtered, cs)
				}
			}
			result = filtered
		}
	}

	// Apply limit
	if limit > 0 && limit < len(result) {
		result = result[:limit]
	}

	if result == nil {
		result = []*api.ContainerSummary{}
	}
	WriteJSON(w, http.StatusOK, result)
}

func (s *BaseServer) handleContainerLogs(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, _ := s.Store.Containers.Get(id)

	// Check if container has logs capability
	if c.State.Status == "created" {
		WriteError(w, &api.InvalidParameterError{
			Message: "can not get logs from container which is dead or marked for removal",
		})
		return
	}

	timestamps := r.URL.Query().Get("timestamps") == "1" || r.URL.Query().Get("timestamps") == "true"
	follow := r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true"
	// BUG-390: Read details query parameter
	details := r.URL.Query().Get("details") == "1" || r.URL.Query().Get("details") == "true"

	// BUG-422: Parse stdout/stderr params — default both to true when neither specified
	stdoutParam := r.URL.Query().Get("stdout")
	stderrParam := r.URL.Query().Get("stderr")
	wantStdout := stdoutParam != "0" && stdoutParam != "false" &&
		((stderrParam != "1" && stderrParam != "true") || stdoutParam != "")

	// Read from container process or synthetic log buffer via driver chain
	logBytes := s.Drivers.Stream.LogBytes(id)

	// All simulator output is stdout — if stdout suppressed, return empty
	if !wantStdout {
		logBytes = nil
	}

	// Split into lines and stamp each with the container's start time for filtering (BUG-276)
	var lines []string
	if len(logBytes) > 0 {
		ts := c.State.StartedAt
		if ts == "" || ts == "0001-01-01T00:00:00Z" {
			ts = time.Now().UTC().Format(time.RFC3339Nano)
		}
		raw := strings.Split(strings.TrimRight(string(logBytes), "\n"), "\n")
		for _, line := range raw {
			if line == "" {
				continue // BUG-277: skip phantom empty lines
			}
			lines = append(lines, ts+" "+line)
		}
	}

	// Apply since/until filters
	if sinceStr := r.URL.Query().Get("since"); sinceStr != "" {
		if since, err := ParseDockerTimestamp(sinceStr); err == nil {
			lines = FilterLogSince(lines, since)
		}
	}
	if untilStr := r.URL.Query().Get("until"); untilStr != "" {
		if until, err := ParseDockerTimestamp(untilStr); err == nil {
			lines = FilterLogUntil(lines, until)
		}
	}

	// Apply tail filter
	if tailStr := r.URL.Query().Get("tail"); tailStr != "" && tailStr != "all" {
		if n, err := strconv.Atoi(tailStr); err == nil {
			lines = FilterLogTail(lines, n)
		}
	}

	// Strip timestamps if not requested
	if !timestamps {
		for i, line := range lines {
			if idx := strings.IndexByte(line, ' '); idx >= 0 {
				lines[i] = line[idx+1:]
			}
		}
	}

	// BUG-390: Prepend container labels when details=true
	if details && len(c.Config.Labels) > 0 {
		var labelParts []string
		for k, v := range c.Config.Labels {
			labelParts = append(labelParts, k+"="+v)
		}
		prefix := strings.Join(labelParts, ",") + " "
		for i, line := range lines {
			lines[i] = prefix + line
		}
	}

	// Reassemble into bytes
	var filtered []byte
	for _, line := range lines {
		filtered = append(filtered, []byte(line+"\n")...)
	}

	// Write stream — raw for TTY containers, multiplexed otherwise
	tty := c.Config.Tty
	if tty {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
	} else {
		w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	}
	w.WriteHeader(http.StatusOK)
	if len(filtered) > 0 {
		if tty {
			w.Write(filtered)
		} else {
			writeMuxChunk(w, 1, filtered)
		}
	}

	// Follow: stream live output
	if follow {
		if f, ok := w.(http.Flusher); ok {
			f.Flush()
		}

		// BUG-278: parse until for follow mode
		var followUntil time.Time
		if untilStr := r.URL.Query().Get("until"); untilStr != "" {
			if t, err := ParseDockerTimestamp(untilStr); err == nil {
				followUntil = t
			}
		}

		subID := GenerateID()[:16]
		ch := s.Drivers.Stream.LogSubscribe(id, subID)
		if ch != nil {
			defer s.Drivers.Stream.LogUnsubscribe(id, subID)
			for {
				// BUG-278: stop following if until time has passed
				if !followUntil.IsZero() && time.Now().After(followUntil) {
					return
				}
				select {
				case chunk, ok := <-ch:
					if !ok {
						return
					}
					if len(chunk) > 0 {
						if tty {
							w.Write(chunk)
						} else {
							writeMuxChunk(w, 1, chunk)
						}
						if f, ok := w.(http.Flusher); ok {
							f.Flush()
						}
					}
				case <-r.Context().Done():
					return
				}
			}
		}
	}
}

// handleContainerAttach establishes a bidirectional stream to the container.
// Dispatches through the StreamDriver chain (agent → WASM → synthetic).
func (s *BaseServer) handleContainerAttach(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	id, ok := s.Store.ResolveContainerID(ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	c, ok := s.Store.Containers.Get(id)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	// Hijack the connection for bidirectional streaming
	hj, ok := w.(http.Hijacker)
	if !ok {
		WriteError(w, &api.ServerError{Message: "hijacking not supported"})
		return
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		WriteError(w, &api.ServerError{Message: err.Error()})
		return
	}
	defer conn.Close()

	tty := c.Config.Tty
	contentType := "application/vnd.docker.multiplexed-stream"
	if tty {
		contentType = "application/vnd.docker.raw-stream"
	}

	// Write upgrade response
	buf.WriteString("HTTP/1.1 101 UPGRADED\r\n")
	buf.WriteString("Content-Type: " + contentType + "\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString("Upgrade: tcp\r\n")
	buf.WriteString("\r\n")
	buf.Flush()

	_ = s.Drivers.Stream.Attach(r.Context(), id, tty, conn)
}

// buildPortList converts PortBindings and ExposedPorts into a list of Port entries
// for the container list response.
func buildPortList(portBindings map[string][]api.PortBinding, exposedPorts map[string]struct{}) []api.Port {
	var ports []api.Port
	seen := make(map[string]bool)

	for portSpec, bindings := range portBindings {
		seen[portSpec] = true
		privatePort, portType := parsePortSpec(portSpec)
		for _, b := range bindings {
			p := api.Port{
				PrivatePort: privatePort,
				Type:        portType,
			}
			if b.HostPort != "" {
				hp, _ := strconv.ParseUint(b.HostPort, 10, 16)
				p.PublicPort = uint16(hp)
			}
			if b.HostIP != "" {
				p.IP = b.HostIP
			}
			ports = append(ports, p)
		}
		if len(bindings) == 0 {
			ports = append(ports, api.Port{PrivatePort: privatePort, Type: portType})
		}
	}

	for portSpec := range exposedPorts {
		if seen[portSpec] {
			continue
		}
		privatePort, portType := parsePortSpec(portSpec)
		ports = append(ports, api.Port{PrivatePort: privatePort, Type: portType})
	}

	if ports == nil {
		ports = []api.Port{}
	}
	return ports
}

// parsePortSpec parses a Docker port spec like "8080/tcp" into port number and type.
func parsePortSpec(spec string) (uint16, string) {
	parts := strings.SplitN(spec, "/", 2)
	port, _ := strconv.ParseUint(parts[0], 10, 16)
	portType := "tcp"
	if len(parts) == 2 && parts[1] != "" {
		portType = parts[1]
	}
	return uint16(port), portType
}
