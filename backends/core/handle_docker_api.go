package core

import (
	"io"
	"net/http"
	"regexp"
	"runtime"
	"strings"

	"github.com/sockerless/api"
)

var versionPrefix = regexp.MustCompile(`^/v\d+\.\d+/`)

// stripVersionPrefix is middleware that removes /v1.XX/ prefix from request paths.
func stripVersionPrefix(next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		if loc := versionPrefix.FindStringIndex(r.URL.Path); loc != nil {
			r.URL.Path = r.URL.Path[loc[1]-1:] // keep the leading /
			if r.URL.RawPath != "" {
				if loc := versionPrefix.FindStringIndex(r.URL.RawPath); loc != nil {
					r.URL.RawPath = r.URL.RawPath[loc[1]-1:]
				}
			}
		}
		next.ServeHTTP(w, r)
	})
}

// FlushingCopy copies from src to w, flushing after each read chunk.
func FlushingCopy(w http.ResponseWriter, src io.Reader) {
	flusher, canFlush := w.(http.Flusher)
	buf := make([]byte, 4096)
	for {
		n, err := src.Read(buf)
		if n > 0 {
			_, _ = w.Write(buf[:n])
			if canFlush {
				flusher.Flush()
			}
		}
		if err != nil {
			break
		}
	}
}

// registerDockerAPIRoutes registers Docker-compatible API routes on the same mux.
func (s *BaseServer) registerDockerAPIRoutes() {
	// System
	s.Mux.HandleFunc("GET /_ping", s.handleDockerPing)
	s.Mux.HandleFunc("HEAD /_ping", s.handleDockerPing)
	s.Mux.HandleFunc("GET /version", s.handleDockerVersion)
	s.Mux.HandleFunc("GET /info", s.handleDockerInfo)

	// Containers
	s.Mux.HandleFunc("POST /containers/create", s.handleContainerCreate)
	s.Mux.HandleFunc("GET /containers/json", s.handleContainerList)
	s.Mux.HandleFunc("GET /containers/{id}/json", s.handleContainerInspect)
	s.Mux.HandleFunc("POST /containers/{id}/start", s.handleContainerStart)
	s.Mux.HandleFunc("POST /containers/{id}/stop", s.handleContainerStop)
	s.Mux.HandleFunc("POST /containers/{id}/restart", s.handleContainerRestart)
	s.Mux.HandleFunc("POST /containers/{id}/kill", s.handleContainerKill)
	s.Mux.HandleFunc("DELETE /containers/{id}", s.handleContainerRemove)
	s.Mux.HandleFunc("GET /containers/{id}/logs", s.handleContainerLogs)
	s.Mux.HandleFunc("POST /containers/{id}/wait", s.handleContainerWait)
	s.Mux.HandleFunc("POST /containers/{id}/attach", s.handleContainerAttach)
	s.Mux.HandleFunc("POST /containers/{id}/resize", s.handleContainerResize)
	s.Mux.HandleFunc("GET /containers/{id}/top", s.handleContainerTop)
	s.Mux.HandleFunc("GET /containers/{id}/stats", s.handleContainerStats)
	s.Mux.HandleFunc("POST /containers/{id}/rename", s.handleContainerRename)
	s.Mux.HandleFunc("POST /containers/{id}/pause", s.handleContainerPause)
	s.Mux.HandleFunc("POST /containers/{id}/unpause", s.handleContainerUnpause)
	s.Mux.HandleFunc("POST /containers/prune", s.handleContainerPrune)
	s.Mux.HandleFunc("PUT /containers/{id}/archive", s.handlePutArchive)
	s.Mux.HandleFunc("HEAD /containers/{id}/archive", s.handleHeadArchive)
	s.Mux.HandleFunc("GET /containers/{id}/archive", s.handleGetArchive)
	s.Mux.HandleFunc("GET /containers/{id}/changes", s.handleContainerChanges)
	s.Mux.HandleFunc("GET /containers/{id}/export", s.handleContainerExport)
	s.Mux.HandleFunc("POST /containers/{id}/update", s.handleContainerUpdate)

	// Exec
	s.Mux.HandleFunc("POST /containers/{id}/exec", s.handleExecCreate)
	s.Mux.HandleFunc("GET /exec/{id}/json", s.handleExecInspect)
	s.Mux.HandleFunc("POST /exec/{id}/start", s.handleExecStart)
	s.Mux.HandleFunc("POST /exec/{id}/resize", s.handleExecResize)

	// Images — specific routes first, catch-all last
	s.Mux.HandleFunc("POST /images/create", s.handleDockerImageCreate)
	s.Mux.HandleFunc("GET /images/json", s.handleImageList)
	s.Mux.HandleFunc("POST /images/load", s.handleImageLoad)
	s.Mux.HandleFunc("GET /images/search", s.handleImageSearch)
	s.Mux.HandleFunc("GET /images/get", s.handleImageSave)
	s.Mux.HandleFunc("POST /images/prune", s.handleImagePrune)
	// Catch-all for /images/{name}/json, /images/{name}/tag, etc.
	s.Mux.HandleFunc("GET /images/", s.handleDockerImageCatchAll)
	s.Mux.HandleFunc("POST /images/", s.handleDockerImageCatchAll)
	s.Mux.HandleFunc("DELETE /images/", s.handleDockerImageCatchAll)

	// Auth
	s.Mux.HandleFunc("POST /auth", s.handleAuth)

	// Networks
	s.Mux.HandleFunc("POST /networks/create", s.handleNetworkCreate)
	s.Mux.HandleFunc("GET /networks", s.handleNetworkList)
	s.Mux.HandleFunc("GET /networks/{id}", s.handleNetworkInspect)
	s.Mux.HandleFunc("POST /networks/{id}/connect", s.handleNetworkConnect)
	s.Mux.HandleFunc("POST /networks/{id}/disconnect", s.handleNetworkDisconnect)
	s.Mux.HandleFunc("DELETE /networks/{id}", s.handleNetworkRemove)
	s.Mux.HandleFunc("POST /networks/prune", s.handleNetworkPrune)

	// Volumes
	s.Mux.HandleFunc("POST /volumes/create", s.handleVolumeCreate)
	s.Mux.HandleFunc("GET /volumes", s.handleVolumeList)
	s.Mux.HandleFunc("GET /volumes/{name}", s.handleVolumeInspect)
	s.Mux.HandleFunc("DELETE /volumes/{name}", s.handleVolumeRemove)
	s.Mux.HandleFunc("POST /volumes/prune", s.handleVolumePrune)

	// System events and disk usage
	s.Mux.HandleFunc("GET /events", s.handleSystemEvents)
	s.Mux.HandleFunc("GET /system/df", s.handleSystemDf)

	// Podman Libpod pod API (Sockerless extension)
	s.Mux.HandleFunc("POST /libpod/pods/create", s.handlePodCreate)
	s.Mux.HandleFunc("GET /libpod/pods/json", s.handlePodList)
	s.Mux.HandleFunc("GET /libpod/pods/{name}/json", s.handlePodInspect)
	s.Mux.HandleFunc("GET /libpod/pods/{name}/exists", s.handlePodExists)
	s.Mux.HandleFunc("POST /libpod/pods/{name}/start", s.handlePodStart)
	s.Mux.HandleFunc("POST /libpod/pods/{name}/stop", s.handlePodStop)
	s.Mux.HandleFunc("POST /libpod/pods/{name}/kill", s.handlePodKill)
	s.Mux.HandleFunc("DELETE /libpod/pods/{name}", s.handlePodRemove)

	// Build + commit
	s.Mux.HandleFunc("POST /build", s.handleImageBuild)
	s.Mux.HandleFunc("POST /commit", s.handleContainerCommit)

	// Unsupported endpoints (501)
	s.Mux.HandleFunc("POST /swarm/", s.handleDockerNotImplemented)
	s.Mux.HandleFunc("GET /swarm", s.handleDockerNotImplemented)
	s.Mux.HandleFunc("GET /nodes", s.handleDockerNotImplemented)
	s.Mux.HandleFunc("GET /services", s.handleDockerNotImplemented)
	s.Mux.HandleFunc("GET /tasks", s.handleDockerNotImplemented)
	s.Mux.HandleFunc("GET /secrets", s.handleDockerNotImplemented)
	s.Mux.HandleFunc("GET /configs", s.handleDockerNotImplemented)
	s.Mux.HandleFunc("GET /plugins", s.handleDockerNotImplemented)
	s.Mux.HandleFunc("POST /session", s.handleDockerNotImplemented)
	s.Mux.HandleFunc("GET /distribution/", s.handleDockerNotImplemented)
}

// ── Docker-specific handlers ─────────────────────────────────────────

func (s *BaseServer) handleDockerPing(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("API-Version", "1.44")
	w.Header().Set("Builder-Version", "2")
	w.Header().Set("Docker-Experimental", "false")
	w.Header().Set("Pragma", "no-cache")
	w.Header().Set("Cache-Control", "no-cache")
	w.Header().Set("Content-Type", "text/plain; charset=utf-8")
	w.Header().Set("Content-Length", "2")
	if r.Method == http.MethodHead {
		w.WriteHeader(http.StatusOK)
		return
	}
	w.Write([]byte("OK"))
}

func (s *BaseServer) handleDockerVersion(w http.ResponseWriter, r *http.Request) {
	info, _ := s.self.Info()
	version := "0.1.0"
	if info != nil && info.ServerVersion != "" {
		version = info.ServerVersion
	}

	kernelVersion := ""
	if info != nil {
		kernelVersion = info.KernelVersion
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"Version":       version,
		"ApiVersion":    "1.44",
		"MinAPIVersion": "1.24",
		"GitCommit":     "sockerless",
		"GoVersion":     runtime.Version(),
		"Os":            runtime.GOOS,
		"Arch":          runtime.GOARCH,
		"KernelVersion": kernelVersion,
		"BuildTime":     "2024-01-01T00:00:00.000000000+00:00",
		"Platform": map[string]string{
			"Name": "Sockerless",
		},
		"Components": []map[string]any{
			{
				"Name":    "Engine",
				"Version": version,
				"Details": map[string]string{
					"ApiVersion":    "1.44",
					"MinAPIVersion": "1.24",
					"GitCommit":     "sockerless",
					"GoVersion":     runtime.Version(),
					"Os":            runtime.GOOS,
					"Arch":          runtime.GOARCH,
					"BuildTime":     "2024-01-01T00:00:00.000000000+00:00",
				},
			},
			{
				"Name":    "containerd",
				"Version": "1.7.0",
				"Details": map[string]string{
					"GitCommit": "sockerless",
				},
			},
		},
	})
}

func (s *BaseServer) handleDockerInfo(w http.ResponseWriter, r *http.Request) {
	info, err := s.self.Info()
	if err != nil {
		WriteError(w, err)
		return
	}

	WriteJSON(w, http.StatusOK, map[string]any{
		"ID":                info.ID,
		"Name":              info.Name,
		"ServerVersion":     info.ServerVersion,
		"Containers":        info.Containers,
		"ContainersRunning": info.ContainersRunning,
		"ContainersPaused":  info.ContainersPaused,
		"ContainersStopped": info.ContainersStopped,
		"Images":            info.Images,
		"Driver":            info.Driver,
		"OperatingSystem":   info.OperatingSystem,
		"OSType":            info.OSType,
		"Architecture":      info.Architecture,
		"NCPU":              info.NCPU,
		"MemTotal":          info.MemTotal,
		"KernelVersion":     info.KernelVersion,
		"DockerRootDir":     "/var/lib/sockerless",
		"HttpProxy":         "",
		"HttpsProxy":        "",
		"NoProxy":           "",
		"Labels":            []string{},
		"ExperimentalBuild": false,
		"LiveRestoreEnabled": false,
		"SecurityOptions":   []string{},
		"Warnings":          []string{},
		"RegistryConfig": map[string]any{
			"InsecureRegistryCIDRs": []string{},
			"IndexConfigs":          map[string]any{},
			"Mirrors":               []string{},
		},
		"Plugins": map[string]any{
			"Volume":        []string{"local"},
			"Network":       []string{"bridge", "host", "null"},
			"Authorization": nil,
			"Log":           []string{"json-file"},
		},
		"Runtimes": map[string]any{
			"runc": map[string]string{"path": "runc"},
		},
		"LoggingDriver":  "json-file",
		"DefaultRuntime": "runc",
		"Swarm":          map[string]any{"LocalNodeState": "inactive"},
		"Containerd":     map[string]any{"Address": ""},
	})
}

func (s *BaseServer) handleDockerImageCreate(w http.ResponseWriter, r *http.Request) {
	// docker import: fromSrc is set (e.g. "-" for stdin tar)
	if fromSrc := r.URL.Query().Get("fromSrc"); fromSrc != "" {
		rc, err := s.self.ImageLoad(r.Body)
		if err != nil {
			WriteError(w, err)
			return
		}
		defer rc.Close()

		// BUG-498: Forward repo/tag by tagging the loaded image
		repo := r.URL.Query().Get("repo")
		if repo != "" {
			body, _ := io.ReadAll(rc)
			loaded := strings.TrimPrefix(string(body), "{\"stream\":\"Loaded image: ")
			if idx := strings.Index(loaded, "\""); idx > 0 {
				loaded = strings.TrimSpace(loaded[:idx])
				if loaded != "" {
					tag := r.URL.Query().Get("tag")
					_ = s.self.ImageTag(loaded, repo, tag)
				}
			}
			w.Header().Set("Content-Type", "application/json")
			w.WriteHeader(http.StatusOK)
			w.Write(body)
			return
		}

		w.Header().Set("Content-Type", "application/json")
		w.WriteHeader(http.StatusOK)
		FlushingCopy(w, rc)
		return
	}

	fromImage := r.URL.Query().Get("fromImage")
	tag := r.URL.Query().Get("tag")
	if tag == "" {
		tag = "latest"
	}

	ref := fromImage
	if tag != "" && tag != "latest" {
		ref = fromImage + ":" + tag
	} else if ref != "" && tag == "latest" {
		ref = fromImage + ":latest"
	}

	auth := r.Header.Get("X-Registry-Auth")

	rc, err := s.self.ImagePull(ref, auth)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()

	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	FlushingCopy(w, rc)
}

// handleDockerImageCatchAll handles /images/{name}/json, /images/{name}/tag, etc.
func (s *BaseServer) handleDockerImageCatchAll(w http.ResponseWriter, r *http.Request) {
	path := r.URL.Path
	if i := strings.Index(path, "/images/"); i >= 0 {
		path = path[i+len("/images/"):]
	}

	switch {
	case strings.HasSuffix(path, "/json"):
		name := strings.TrimSuffix(path, "/json")
		s.handleDockerImageInspect(w, r, name)
	case strings.HasSuffix(path, "/tag"):
		name := strings.TrimSuffix(path, "/tag")
		s.handleDockerImageTag(w, r, name)
	case strings.HasSuffix(path, "/history"):
		name := strings.TrimSuffix(path, "/history")
		s.handleDockerImageHistory(w, r, name)
	case strings.HasSuffix(path, "/push"):
		name := strings.TrimSuffix(path, "/push")
		s.handleDockerImagePush(w, r, name)
	case strings.HasSuffix(path, "/get"):
		name := strings.TrimSuffix(path, "/get")
		s.handleDockerImageSaveByName(w, r, name)
	default:
		if r.Method == "DELETE" {
			s.handleDockerImageRemove(w, r, path)
		} else {
			s.handleDockerNotImplemented(w, r)
		}
	}
}

func (s *BaseServer) handleDockerImageInspect(w http.ResponseWriter, _ *http.Request, name string) {
	img, err := s.self.ImageInspect(name)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, img)
}

func (s *BaseServer) handleDockerImageTag(w http.ResponseWriter, r *http.Request, name string) {
	repo := r.URL.Query().Get("repo")
	tag := r.URL.Query().Get("tag")

	if err := s.self.ImageTag(name, repo, tag); err != nil {
		WriteError(w, err)
		return
	}
	w.WriteHeader(http.StatusCreated)
}

func (s *BaseServer) handleDockerImageHistory(w http.ResponseWriter, _ *http.Request, name string) {
	result, err := s.self.ImageHistory(name)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

func (s *BaseServer) handleDockerImagePush(w http.ResponseWriter, r *http.Request, name string) {
	tag := r.URL.Query().Get("tag")
	auth := r.Header.Get("X-Registry-Auth")
	rc, err := s.self.ImagePush(name, tag, auth)
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/json")
	w.WriteHeader(http.StatusOK)
	FlushingCopy(w, rc)
}

func (s *BaseServer) handleDockerImageSaveByName(w http.ResponseWriter, _ *http.Request, name string) {
	rc, err := s.self.ImageSave([]string{name})
	if err != nil {
		WriteError(w, err)
		return
	}
	defer rc.Close()
	w.Header().Set("Content-Type", "application/x-tar")
	w.WriteHeader(http.StatusOK)
	FlushingCopy(w, rc)
}

func (s *BaseServer) handleDockerImageRemove(w http.ResponseWriter, r *http.Request, name string) {
	force := r.URL.Query().Get("force") == "1" || r.URL.Query().Get("force") == "true"
	noprune := r.URL.Query().Get("noprune") == "1" || r.URL.Query().Get("noprune") == "true"

	result, err := s.self.ImageRemove(name, force, !noprune)
	if err != nil {
		WriteError(w, err)
		return
	}
	WriteJSON(w, http.StatusOK, result)
}

func (s *BaseServer) handleDockerNotImplemented(w http.ResponseWriter, r *http.Request) {
	WriteError(w, &api.NotImplementedError{
		Message: r.Method + " " + r.URL.Path + " is not supported",
	})
}
