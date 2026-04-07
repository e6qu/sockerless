package core

import (
	"crypto/sha256"
	"fmt"
	"io"
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
	c, ok := s.ResolveContainerAuto(r.Context(), ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}
	// Populate size fields when size=true or size=1
	includeSize := r.URL.Query().Get("size") == "1" || r.URL.Query().Get("size") == "true"
	if includeSize {
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			size := img.Size
			c.SizeRootFs = &size
		}
		if rootPath, err := s.Drivers.Filesystem.RootPath(c.ID); err == nil && rootPath != "" {
			rw := DirSize(rootPath)
			c.SizeRw = &rw
		}
	}
	WriteJSON(w, http.StatusOK, c)
}

func (s *BaseServer) handleContainerList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	// Read size query parameter
	includeSize := r.URL.Query().Get("size") == "1" || r.URL.Query().Get("size") == "true"
	filters := ParseFilters(r.URL.Query().Get("filters"))
	limit := 0
	if l := r.URL.Query().Get("limit"); l != "" {
		limit, _ = strconv.Atoi(l)
	}

	// Use cloud state provider when available
	var containers []api.Container
	if s.CloudState != nil {
		cloudContainers, err := s.CloudState.ListContainers(r.Context(), all, filters)
		if err == nil {
			containers = cloudContainers
		}
	}
	// Also include pending creates (not yet in cloud)
	for _, pc := range s.PendingCreates.List() {
		if all || pc.State.Running {
			containers = append(containers, pc)
		}
	}
	// Fall back to Store when CloudState is nil or returned nothing
	if s.CloudState == nil {
		containers = s.Store.Containers.List()
	}

	var result []*api.ContainerSummary
	for _, c := range containers {
		if s.CloudState == nil && !all && !c.State.Running {
			continue
		}
		if s.CloudState == nil && !MatchContainerFilters(c, filters) {
			continue
		}
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
			Mounts: mounts,
			NetworkSettings: &api.SummaryNetworkSettings{
				Networks: c.NetworkSettings.Networks,
			},
		}
		// Populate size fields when requested
		if includeSize {
			if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
				summary.SizeRootFs = img.Size
			}
			// Populate SizeRw from container root dir
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
		if bc, ok := s.ResolveContainerAuto(r.Context(), beforeRef[0]); ok {
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
		if sc, ok := s.ResolveContainerAuto(r.Context(), sinceRef[0]); ok {
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

	// Parse stdout/stderr params
	stdoutParam := r.URL.Query().Get("stdout")
	stderrParam := r.URL.Query().Get("stderr")
	wantStdout := stdoutParam != "0" && stdoutParam != "false" &&
		((stderrParam != "1" && stderrParam != "true") || stdoutParam != "")

	opts := api.ContainerLogsOptions{
		ShowStdout: wantStdout,
		ShowStderr: stderrParam == "1" || stderrParam == "true",
		Follow:     r.URL.Query().Get("follow") == "1" || r.URL.Query().Get("follow") == "true",
		Timestamps: r.URL.Query().Get("timestamps") == "1" || r.URL.Query().Get("timestamps") == "true",
		Tail:       r.URL.Query().Get("tail"),
		Since:      r.URL.Query().Get("since"),
		Until:      r.URL.Query().Get("until"),
	}

	rc, err := s.self.ContainerLogs(ref, opts)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()

	// Determine framing from container TTY
	c, _ := s.ResolveContainerAuto(r.Context(), ref)
	tty := c.Config.Tty

	// Read details query parameter and prepend labels
	details := r.URL.Query().Get("details") == "1" || r.URL.Query().Get("details") == "true"
	var detailPrefix string
	if details && len(c.Config.Labels) > 0 {
		var labelParts []string
		for k, v := range c.Config.Labels {
			labelParts = append(labelParts, k+"="+v)
		}
		detailPrefix = strings.Join(labelParts, ",") + " "
	}

	if tty {
		w.Header().Set("Content-Type", "application/vnd.docker.raw-stream")
	} else {
		w.Header().Set("Content-Type", "application/vnd.docker.multiplexed-stream")
	}
	w.WriteHeader(http.StatusOK)

	buf := make([]byte, 32*1024)
	for {
		n, readErr := rc.Read(buf)
		if n > 0 {
			data := buf[:n]
			if detailPrefix != "" {
				data = prependDetailsToLines(data, detailPrefix)
			}
			if tty {
				w.Write(data)
			} else {
				writeMuxChunk(w, 1, data)
			}
			if f, ok := w.(http.Flusher); ok {
				f.Flush()
			}
		}
		if readErr != nil {
			break
		}
	}
}

// prependDetailsToLines prepends a prefix to each line in data.
func prependDetailsToLines(data []byte, prefix string) []byte {
	s := string(data)
	lines := strings.Split(s, "\n")
	var result []string
	for i, line := range lines {
		if i == len(lines)-1 && line == "" {
			result = append(result, "")
			continue
		}
		result = append(result, prefix+line)
	}
	return []byte(strings.Join(result, "\n"))
}

// handleContainerAttach establishes a bidirectional stream to the container.
func (s *BaseServer) handleContainerAttach(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")

	opts := api.ContainerAttachOptions{
		Stream: r.URL.Query().Get("stream") != "false",
		Stdin:  r.URL.Query().Get("stdin") == "1" || r.URL.Query().Get("stdin") == "true",
		Stdout: r.URL.Query().Get("stdout") != "0" && r.URL.Query().Get("stdout") != "false",
		Stderr: r.URL.Query().Get("stderr") == "1" || r.URL.Query().Get("stderr") == "true",
		Logs:   r.URL.Query().Get("logs") == "1" || r.URL.Query().Get("logs") == "true",
	}

	rwc, err := s.self.ContainerAttach(ref, opts)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rwc.Close()

	// Determine framing from container TTY
	c, _ := s.ResolveContainerAuto(r.Context(), ref)
	tty := c.Config.Tty

	// Hijack the connection for bidirectional streaming
	hj, ok := w.(http.Hijacker)
	if !ok {
		WriteError(w, &api.ServerError{Message: "hijacking not supported"})
		return
	}

	conn, buf, herr := hj.Hijack()
	if herr != nil {
		WriteError(w, &api.ServerError{Message: herr.Error()})
		return
	}
	defer conn.Close()

	contentType := "application/vnd.docker.multiplexed-stream"
	if tty {
		contentType = "application/vnd.docker.raw-stream"
	}

	buf.WriteString("HTTP/1.1 101 UPGRADED\r\n")
	buf.WriteString("Content-Type: " + contentType + "\r\n")
	buf.WriteString("Connection: Upgrade\r\n")
	buf.WriteString("Upgrade: tcp\r\n")
	buf.WriteString("\r\n")
	buf.Flush()

	// Copy data between the attached stream and the hijacked connection
	done := make(chan struct{})
	go func() {
		io.Copy(conn, rwc)
		close(done)
	}()
	go func() {
		io.Copy(rwc, conn)
	}()
	<-done
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
