package core

import (
	"net/http"
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

	// Read from container process or synthetic log buffer via driver chain
	logBytes := s.Drivers.Stream.LogBytes(id)

	// Add timestamp prefix if requested
	if timestamps && len(logBytes) > 0 {
		ts := time.Now().UTC().Format(time.RFC3339Nano)
		lines := strings.Split(strings.TrimRight(string(logBytes), "\n"), "\n")
		var out []byte
		for _, line := range lines {
			out = append(out, []byte(ts+" "+line+"\n")...)
		}
		logBytes = out
	}

	// Write multiplexed stream (stdout header + data)
	w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	w.WriteHeader(http.StatusOK)
	if len(logBytes) > 0 {
		// Write stdout header: [1, 0, 0, 0, size (4 bytes big-endian)]
		size := len(logBytes)
		header := []byte{1, 0, 0, 0, byte(size >> 24), byte(size >> 16), byte(size >> 8), byte(size)}
		w.Write(header)
		w.Write(logBytes)
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
		http.Error(w, "hijacking not supported", http.StatusInternalServerError)
		return
	}

	conn, buf, err := hj.Hijack()
	if err != nil {
		http.Error(w, err.Error(), http.StatusInternalServerError)
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
