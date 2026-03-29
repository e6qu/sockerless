package core

import (
	"encoding/json"
	"net/http"
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

	// Containers — route to Docker compat handlers with libpod header
	s.Mux.HandleFunc("GET /libpod/containers/json", lp(s.handleContainerList))
	s.Mux.HandleFunc("POST /libpod/containers/create", lp(s.handleContainerCreate))
	s.Mux.HandleFunc("GET /libpod/containers/{id}/json", lp(s.handleContainerInspect))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/start", lp(s.handleContainerStart))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/stop", lp(s.handleContainerStop))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/restart", lp(s.handleContainerRestart))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/kill", lp(s.handleContainerKill))
	s.Mux.HandleFunc("DELETE /libpod/containers/{id}", lp(s.handleContainerRemove))
	s.Mux.HandleFunc("GET /libpod/containers/{id}/logs", lp(s.handleContainerLogs))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/wait", lp(s.handleContainerWait))
	s.Mux.HandleFunc("POST /libpod/containers/{id}/rename", lp(s.handleContainerRename))
	s.Mux.HandleFunc("GET /libpod/containers/{id}/top", lp(s.handleContainerTop))
	s.Mux.HandleFunc("GET /libpod/containers/{id}/stats", lp(s.handleContainerStats))

	// Exec
	s.Mux.HandleFunc("POST /libpod/containers/{id}/exec", lp(s.handleExecCreate))
	s.Mux.HandleFunc("POST /libpod/exec/{id}/start", lp(s.handleExecStart))

	// Images — route to Docker compat handlers
	s.Mux.HandleFunc("POST /libpod/images/pull", lp(s.handleDockerImageCreate))
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

	// Events and system
	s.Mux.HandleFunc("GET /libpod/events", lp(s.handleSystemEvents))
	s.Mux.HandleFunc("GET /libpod/system/df", lp(s.handleSystemDf))
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
