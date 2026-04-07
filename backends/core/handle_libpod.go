package core

import (
	"encoding/json"
	"io"
	"net/http"
	"time"

	"github.com/sockerless/api"
)

// registerLibpodRoutes registers Podman Libpod API routes.
// These route to the same handlers as the Docker compat API but wrap
// responses with the Libpod-API-Version header. Container and image
// list/inspect endpoints use a format adapter for field name differences.
func (s *BaseServer) registerLibpodRoutes() {
	lp := func(h http.HandlerFunc) http.HandlerFunc {
		return libpodHeader(h)
	}

	// System (ping and version already registered in registerDockerAPIRoutes)
	s.Mux.HandleFunc("GET /libpod/info", lp(s.handleLibpodInfo))

	// Containers
	s.Mux.HandleFunc("GET /libpod/containers/json", lp(s.handleLibpodContainerList))
	s.Mux.HandleFunc("POST /libpod/containers/create", lp(s.handleLibpodContainerCreate))
	s.Mux.HandleFunc("GET /libpod/containers/{id}/json", lp(s.handleContainerInspect))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/start", lp(s.handleContainerStart))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/stop", lp(s.handleContainerStop))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/restart", lp(s.handleContainerRestart))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/kill", lp(s.handleContainerKill))
	s.Mux.HandleFunc("DELETE /libpod/containers/{id}", lp(s.handleLibpodContainerRemove))
	s.Mux.HandleFunc("GET /libpod/containers/{id}/logs", lp(s.handleContainerLogs))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/wait", lp(s.handleContainerWait))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/rename", lp(s.handleContainerRename))
	s.Mux.HandleFunc("GET /libpod/containers/{id}/top", lp(s.handleContainerTop))
	s.Mux.HandleFunc("GET /libpod/containers/{id}/stats", lp(s.handleContainerStats))

	// Exec
	s.Mux.HandleFunc("POST /libpod/containers/{id}/exec", lp(s.handleExecCreate))
	s.Mux.HandleFunc("POST /libpod/exec/{id}/start", lp(s.handleExecStart))

	// Images
	s.Mux.HandleFunc("POST /libpod/images/pull", lp(s.handleLibpodImagePull))
	s.Mux.HandleFunc("GET /libpod/images/json", lp(s.handleImageList))
	s.Mux.HandleFunc("GET /libpod/images/{name}/json", lp(s.handleImageInspect))
	s.Mux.HandleFunc("DELETE /libpod/images/{name}", lp(s.handleDockerImageCatchAll))
	s.Mux.HandleFunc("GET /libpod/images/{name}/history", lp(s.handleDockerImageCatchAll))
	s.Mux.HandleFunc("POST /libpod/images/{name}/tag", lp(s.handleDockerImageCatchAll))

	// Networks
	s.Mux.HandleFunc("POST /libpod/networks/create", lp(s.handleNetworkCreate))
	s.Mux.HandleFunc("GET /libpod/networks/json", lp(s.handleNetworkList))
	s.Mux.HandleFunc("GET /libpod/networks/{id}/json", lp(s.handleNetworkInspect))
	s.Mux.HandleFunc("DELETE /libpod/networks/{id}", lp(s.handleNetworkRemove))

	// Volumes
	s.Mux.HandleFunc("POST /libpod/volumes/create", lp(s.handleVolumeCreate))
	s.Mux.HandleFunc("GET /libpod/volumes/json", lp(s.handleVolumeList))
	s.Mux.HandleFunc("GET /libpod/volumes/{name}/json", lp(s.handleVolumeInspect))
	s.Mux.HandleFunc("DELETE /libpod/volumes/{name}", lp(s.handleVolumeRemove))

	// Build
	s.Mux.HandleFunc("POST /libpod/build", lp(s.handleImageBuild))

	// Events and system
	s.Mux.HandleFunc("GET /libpod/events", lp(s.handleSystemEvents))
	s.Mux.HandleFunc("GET /libpod/system/df", lp(s.handleSystemDf))
}

// handleLibpodContainerRemove handles DELETE /libpod/containers/{id}.
// Podman expects JSON array []*reports.RmReport, not 204 No Content.
func (s *BaseServer) handleLibpodContainerRemove(w http.ResponseWriter, r *http.Request) {
	ref := r.PathValue("id")
	force := r.URL.Query().Get("force") == "true" || r.URL.Query().Get("force") == "1"

	id, ok := s.ResolveContainerIDAuto(r.Context(), ref)
	if !ok {
		WriteError(w, &api.NotFoundError{Resource: "container", ID: ref})
		return
	}

	if err := s.self.ContainerRemove(id, force); err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, []map[string]any{
		{"Id": id, "Err": nil},
	})
}

// handleLibpodContainerCreate handles POST /libpod/containers/create.
// Podman sends the container name and pod in the JSON body, not as query params.
func (s *BaseServer) handleLibpodContainerCreate(w http.ResponseWriter, r *http.Request) {
	// Pre-read body to extract Podman-specific fields (pod, name) not in ContainerCreateRequest
	bodyBytes, err := io.ReadAll(r.Body)
	if err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	// Extract pod field from raw JSON (Podman sends --pod here)
	var rawFields struct {
		Pod  string `json:"pod"`
		Name string `json:"name"`
	}
	_ = json.Unmarshal(bodyBytes, &rawFields)

	// Parse the standard request
	var req api.ContainerCreateRequest
	if err := json.Unmarshal(bodyBytes, &req); err != nil {
		WriteError(w, &api.InvalidParameterError{Message: err.Error()})
		return
	}

	// Podman sends name in body; Docker compat uses ?name= query param
	if qName := r.URL.Query().Get("name"); qName != "" {
		req.Name = qName
	} else if rawFields.Name != "" {
		req.Name = rawFields.Name
	}

	// Pod from body or query param
	podRef := rawFields.Pod
	if qPod := r.URL.Query().Get("pod"); qPod != "" {
		podRef = qPod
	}
	if podRef != "" {
		if _, ok := s.Store.Pods.GetPod(podRef); !ok {
			WriteError(w, &api.NotFoundError{Resource: "pod", ID: podRef})
			return
		}
	}

	resp, err := s.self.ContainerCreate(&req)
	if err != nil {
		WriteError(w, err)
		return
	}

	if podRef != "" {
		pod, _ := s.Store.Pods.GetPod(podRef)
		_ = s.Store.Pods.AddContainer(pod.ID, resp.ID)
	}

	WriteJSON(w, http.StatusCreated, resp)
}

// handleLibpodImagePull handles POST /libpod/images/pull with Podman-format response.
// Podman expects: stream lines like {"stream":"Pulling..."} then final {"images":["ref"],"id":"sha256:..."}
func (s *BaseServer) handleLibpodImagePull(w http.ResponseWriter, r *http.Request) {
	ref := r.URL.Query().Get("reference")
	if ref == "" {
		ref = r.URL.Query().Get("fromImage")
	}
	if ref == "" {
		WriteError(w, &api.InvalidParameterError{Message: "image reference is required"})
		return
	}

	auth := r.Header.Get("X-Registry-Auth")

	rc, err := s.self.ImagePull(ref, auth)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()

	// Consume the Docker-format pull stream (discard progress)
	_, _ = io.Copy(io.Discard, rc)

	// Resolve the pulled image to get its ID
	imageID := ""
	if img, ok := s.Store.ResolveImage(ref); ok {
		imageID = img.ID
	}

	// Return Podman-format response
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	enc := json.NewEncoder(w)
	_ = enc.Encode(map[string]any{"stream": "Pulling " + ref + "...\n"})
	_ = enc.Encode(map[string]any{"stream": "Pull complete\n"})
	_ = enc.Encode(map[string]any{
		"images": []string{imageID},
		"id":     imageID,
	})
}

// handleLibpodContainerList returns the container list in Podman's expected format.
// Key difference from Docker: Created is an RFC3339 string, not a Unix timestamp.
func (s *BaseServer) handleLibpodContainerList(w http.ResponseWriter, r *http.Request) {
	all := r.URL.Query().Get("all") == "1" || r.URL.Query().Get("all") == "true"
	filters := ParseFilters(r.URL.Query().Get("filters"))

	type libpodContainer struct {
		AutoRemove bool              `json:"AutoRemove"`
		Command    []string          `json:"Command"`
		Created    string            `json:"Created"`
		StartedAt  int64             `json:"StartedAt"`
		Exited     bool              `json:"Exited"`
		ExitedAt   int64             `json:"ExitedAt"`
		ExitCode   int               `json:"ExitCode"`
		ID         string            `json:"Id"`
		Image      string            `json:"Image"`
		ImageID    string            `json:"ImageID"`
		IsInfra    bool              `json:"IsInfra"`
		Labels     map[string]string `json:"Labels"`
		Mounts     []string          `json:"Mounts"`
		Names      []string          `json:"Names"`
		Pid        int               `json:"Pid"`
		Pod        string            `json:"Pod"`
		PodName    string            `json:"PodName"`
		State      string            `json:"State"`
		Status     string            `json:"Status"`
	}

	var result []libpodContainer
	for _, c := range s.Store.Containers.List() {
		if !all && !c.State.Running {
			continue
		}
		if !MatchContainerFilters(c, filters) {
			continue
		}

		labels := c.Config.Labels
		if labels == nil {
			labels = make(map[string]string)
		}

		var mounts []string
		for _, m := range c.Mounts {
			mounts = append(mounts, m.Destination)
		}

		cmd := append(c.Config.Entrypoint, c.Config.Cmd...)
		if len(cmd) == 0 && c.Path != "" {
			cmd = append([]string{c.Path}, c.Args...)
		}

		imageID := ""
		if img, ok := s.Store.ResolveImage(c.Config.Image); ok {
			imageID = img.ID
		}

		// Podman uses state names like "running", "exited", "created"
		state := c.State.Status
		exited := state == "exited"

		startedAt := int64(0)
		if t, err := time.Parse(time.RFC3339Nano, c.State.StartedAt); err == nil {
			startedAt = t.Unix()
		}
		exitedAt := int64(0)
		if t, err := time.Parse(time.RFC3339Nano, c.State.FinishedAt); err == nil && exited {
			exitedAt = t.Unix()
		}

		result = append(result, libpodContainer{
			Command:   cmd,
			Created:   c.Created,
			StartedAt: startedAt,
			ExitedAt:  exitedAt,
			Exited:    exited,
			ExitCode:  c.State.ExitCode,
			ID:        c.ID,
			Image:     c.Config.Image,
			ImageID:   imageID,
			Labels:    labels,
			Mounts:    mounts,
			Names:     []string{c.Name},
			Pid:       c.State.Pid,
			State:     state,
			Status:    FormatStatus(c.State),
		})
	}

	if result == nil {
		result = []libpodContainer{}
	}
	WriteJSON(w, http.StatusOK, result)
}

// libpodHeader wraps a handler to add the Libpod-API-Version header.
func libpodHeader(next http.HandlerFunc) http.HandlerFunc {
	return func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("Libpod-API-Version", "5.0.0")
		next(w, r)
	}
}

// handleLibpodInfo returns system info in podman's expected format.
func (s *BaseServer) handleLibpodInfo(w http.ResponseWriter, r *http.Request) {
	running, paused, stopped := 0, 0, 0
	for _, c := range s.Store.Containers.List() {
		switch {
		case c.State.Running:
			running++
		case c.State.Paused:
			paused++
		default:
			stopped++
		}
	}

	libpodInfo := map[string]any{
		"host": map[string]any{
			"arch":            s.Desc.Architecture,
			"os":              s.Desc.OSType,
			"hostname":        s.Desc.Name,
			"kernel":          s.kernelVersion(),
			"memTotal":        s.Desc.MemTotal,
			"cpus":            s.Desc.NCPU,
			"distribution":    map[string]string{"distribution": s.Desc.OperatingSystem, "version": ""},
			"remoteSocket":    map[string]any{"exists": true, "path": ""},
			"serviceIsRemote": true,
		},
		"store": map[string]any{
			"containerStore": map[string]any{
				"number":  s.Store.Containers.Len(),
				"paused":  paused,
				"running": running,
				"stopped": stopped,
			},
			"imageStore": map[string]any{
				"number": s.Store.Images.Len(),
			},
			"graphDriverName": s.Desc.Driver,
		},
		"registries": map[string]any{
			"search": []string{"docker.io"},
		},
		"version": map[string]any{
			"APIVersion": "5.0.0",
			"Version":    s.Desc.ServerVersion,
			"GoVersion":  "go1.23",
			"Built":      0,
			"OsArch":     s.Desc.OSType + "/" + s.Desc.Architecture,
		},
	}

	w.Header().Set("Libpod-API-Version", "5.0.0")
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	json.NewEncoder(w).Encode(libpodInfo)
}
