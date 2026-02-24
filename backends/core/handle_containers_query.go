package core

import (
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

		summary := &api.ContainerSummary{
			ID:      c.ID,
			Names:   []string{c.Name},
			Image:   c.Config.Image,
			ImageID: "sha256:" + GenerateID(),
			Command: command,
			Created: created.Unix(),
			State:   c.State.Status,
			Status:  status,
			Ports:   buildPortList(c.HostConfig.PortBindings, c.Config.ExposedPorts),
			Labels:  c.Config.Labels,
			Mounts:  c.Mounts,
			NetworkSettings: &api.SummaryNetworkSettings{
				Networks: c.NetworkSettings.Networks,
			},
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
			beforeTime, _ := time.Parse(time.RFC3339Nano, bc.Created)
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
			sinceTime, _ := time.Parse(time.RFC3339Nano, sc.Created)
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

	// Read from container process or synthetic log buffer via driver chain
	logBytes := s.Drivers.Stream.LogBytes(id)

	// Split into lines and stamp each with a timestamp for filtering
	var lines []string
	if len(logBytes) > 0 {
		ts := time.Now().UTC().Format(time.RFC3339Nano)
		raw := strings.Split(strings.TrimRight(string(logBytes), "\n"), "\n")
		for _, line := range raw {
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
		subID := GenerateID()[:16]
		ch := s.Drivers.Stream.LogSubscribe(id, subID)
		if ch != nil {
			defer s.Drivers.Stream.LogUnsubscribe(id, subID)
			for {
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
