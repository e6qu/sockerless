package core

import (
	"bufio"
	"context"
	"crypto/tls"
	"fmt"
	"io/fs"
	"net"
	"net/http"
	"os"
	"path/filepath"
	"strings"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	"go.opentelemetry.io/contrib/instrumentation/net/http/otelhttp"
)

// BackendDescriptor holds static metadata about a backend.
type BackendDescriptor struct {
	ID              string
	Name            string
	ServerVersion   string
	Driver          string
	OperatingSystem string
	OSType          string
	Architecture    string
	KernelVersion   string // per-backend kernel version
	NCPU            int
	MemTotal        int64
	InstanceID      string
}

// ProviderInfo describes the cloud provider connection for a backend.
type ProviderInfo struct {
	Provider  string            `json:"provider"`  // aws, gcp, azure, memory, docker
	Mode      string            `json:"mode"`      // cloud, simulator, local
	Region    string            `json:"region"`    // us-east-1, us-central1, eastus
	Endpoint  string            `json:"endpoint"`  // custom endpoint if simulator
	Resources map[string]string `json:"resources"` // backend-specific: cluster, project, etc.
}

// BaseServer is the common HTTP server used by all non-Docker backends.
type BaseServer struct {
	Store         *Store
	Logger        zerolog.Logger
	Desc          BackendDescriptor
	Mux           *http.ServeMux
	AgentRegistry *AgentRegistry
	Drivers       DriverSet
	Registry      *ResourceRegistry
	StartedAt     time.Time
	Metrics       *Metrics
	HealthChecker HealthChecker
	EventBus      *EventBus
	ProviderInfo  *ProviderInfo
	StatsProvider StatsProvider // real container metrics (nil = zeros)
	self          api.Backend   // virtual dispatch target for overrideable methods
}

// SetSelf sets the virtual dispatch target for overrideable api.Backend methods.
// Embedding types call this after construction to enable method overriding.
func (s *BaseServer) SetSelf(b api.Backend) { s.self = b }

// kernelVersion returns the kernel version string for system info.
// Uses per-backend value from descriptor, falls back to host kernel.
func (s *BaseServer) kernelVersion() string {
	if s.Desc.KernelVersion != "" {
		return s.Desc.KernelVersion
	}
	return hostKernelVersion()
}

// NewBaseServer creates a new base server with the given store, descriptor,
// and logger. It registers all routes and initializes the default bridge network.
func NewBaseServer(store *Store, desc BackendDescriptor, logger zerolog.Logger) *BaseServer {
	if desc.InstanceID == "" {
		desc.InstanceID = DefaultInstanceID()
	}

	registryPath := os.Getenv("SOCKERLESS_REGISTRY_PATH")
	if registryPath == "" {
		dataDir := os.Getenv("SOCKERLESS_DATA_DIR")
		if dataDir == "" {
			dataDir = "."
		}
		registryPath = filepath.Join(dataDir, "sockerless-registry.json")
	}

	s := &BaseServer{
		Store:         store,
		Logger:        logger,
		Desc:          desc,
		Mux:           http.NewServeMux(),
		AgentRegistry: NewAgentRegistry(),
		Registry:      NewResourceRegistry(registryPath, logger),
		StartedAt:     time.Now(),
		Metrics:       NewMetrics(),
		EventBus:      NewEventBus(),
	}
	s.self = s
	s.InitDrivers()
	store.RestartHook = s.handleRestartPolicy
	s.registerRoutes()
	s.InitDefaultNetwork()
	return s
}

func (s *BaseServer) registerRoutes() {
	// System
	s.Mux.HandleFunc("GET /internal/v1/info", s.handleInfo)

	// Management
	s.Mux.HandleFunc("GET /internal/v1/healthz", s.handleHealthz)
	s.Mux.HandleFunc("GET /internal/v1/status", s.handleMgmtStatus)
	s.Mux.HandleFunc("GET /internal/v1/containers/summary", s.handleContainerSummary)
	s.Mux.HandleFunc("GET /internal/v1/metrics", s.handleMetrics)
	s.Mux.HandleFunc("GET /internal/v1/check", s.handleCheck)
	s.Mux.HandleFunc("GET /internal/v1/provider", s.handleMgmtProvider)
	s.Mux.HandleFunc("POST /internal/v1/reload", s.handleReload)

	// Resource registry
	s.Mux.HandleFunc("GET /internal/v1/resources", s.handleResourceList)
	s.Mux.HandleFunc("GET /internal/v1/resources/orphaned", s.handleResourceOrphaned)
	s.Mux.HandleFunc("POST /internal/v1/resources/cleanup", s.handleResourceCleanup)
	s.Mux.HandleFunc("GET /internal/v1/agent/connect", s.handleAgentConnect)

	// Podman Libpod pod API
	s.Mux.HandleFunc("POST /internal/v1/libpod/pods/create", s.handlePodCreate)
	s.Mux.HandleFunc("GET /internal/v1/libpod/pods/json", s.handlePodList)
	s.Mux.HandleFunc("GET /internal/v1/libpod/pods/{name}/json", s.handlePodInspect)
	s.Mux.HandleFunc("GET /internal/v1/libpod/pods/{name}/exists", s.handlePodExists)
	s.Mux.HandleFunc("POST /internal/v1/libpod/pods/{name}/start", s.handlePodStart)
	s.Mux.HandleFunc("POST /internal/v1/libpod/pods/{name}/stop", s.handlePodStop)
	s.Mux.HandleFunc("POST /internal/v1/libpod/pods/{name}/kill", s.handlePodKill)
	s.Mux.HandleFunc("DELETE /internal/v1/libpod/pods/{name}", s.handlePodRemove)

	// Containers — all handlers delegate to s.self for virtual dispatch
	s.Mux.HandleFunc("POST /internal/v1/containers", s.handleContainerCreate)
	s.Mux.HandleFunc("GET /internal/v1/containers", s.handleContainerList)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}", s.handleContainerInspect)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/start", s.handleContainerStart)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/stop", s.handleContainerStop)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/kill", s.handleContainerKill)
	s.Mux.HandleFunc("DELETE /internal/v1/containers/{id}", s.handleContainerRemove)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/logs", s.handleContainerLogs)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/wait", s.handleContainerWait)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/attach", s.handleContainerAttach)

	// Exec
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/exec", s.handleExecCreate)
	s.Mux.HandleFunc("GET /internal/v1/exec/{id}", s.handleExecInspect)
	s.Mux.HandleFunc("POST /internal/v1/exec/{id}/start", s.handleExecStart)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/resize", s.handleContainerResize)
	s.Mux.HandleFunc("POST /internal/v1/exec/{id}/resize", s.handleExecResize)

	// Images
	s.Mux.HandleFunc("POST /internal/v1/images/pull", s.handleImagePull)
	s.Mux.HandleFunc("GET /internal/v1/images/inspect", s.handleImageInspect)
	s.Mux.HandleFunc("POST /internal/v1/images/load", s.handleImageLoad)
	s.Mux.HandleFunc("POST /internal/v1/images/tag", s.handleImageTag)
	s.Mux.HandleFunc("POST /internal/v1/images/build", s.handleImageBuild)

	// Auth
	s.Mux.HandleFunc("POST /internal/v1/auth", s.handleAuth)

	// Networks
	s.Mux.HandleFunc("POST /internal/v1/networks", s.handleNetworkCreate)
	s.Mux.HandleFunc("GET /internal/v1/networks", s.handleNetworkList)
	s.Mux.HandleFunc("GET /internal/v1/networks/{id}", s.handleNetworkInspect)
	s.Mux.HandleFunc("POST /internal/v1/networks/{id}/disconnect", s.handleNetworkDisconnect)
	s.Mux.HandleFunc("DELETE /internal/v1/networks/{id}", s.handleNetworkRemove)
	s.Mux.HandleFunc("POST /internal/v1/networks/prune", s.handleNetworkPrune)

	// Volumes
	s.Mux.HandleFunc("POST /internal/v1/volumes", s.handleVolumeCreate)
	s.Mux.HandleFunc("GET /internal/v1/volumes", s.handleVolumeList)
	s.Mux.HandleFunc("GET /internal/v1/volumes/{name}", s.handleVolumeInspect)
	s.Mux.HandleFunc("DELETE /internal/v1/volumes/{name}", s.handleVolumeRemove)
	s.Mux.HandleFunc("POST /internal/v1/volumes/prune", s.handleVolumePrune)

	// Container archive (copy files to/from container)
	s.Mux.HandleFunc("PUT /internal/v1/containers/{id}/archive", s.handlePutArchive)
	s.Mux.HandleFunc("HEAD /internal/v1/containers/{id}/archive", s.handleHeadArchive)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/archive", s.handleGetArchive)

	// Extended container operations
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/restart", s.handleContainerRestart)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/top", s.handleContainerTop)
	s.Mux.HandleFunc("POST /internal/v1/containers/prune", s.handleContainerPrune)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/stats", s.handleContainerStats)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/rename", s.handleContainerRename)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/pause", s.handleContainerPause)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/unpause", s.handleContainerUnpause)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/update", s.handleContainerUpdate)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/changes", s.handleContainerChanges)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/export", s.handleContainerExport)

	s.Mux.HandleFunc("POST /internal/v1/networks/{id}/connect", s.handleNetworkConnect)

	// Extended image operations
	s.Mux.HandleFunc("GET /internal/v1/images", s.handleImageList)
	s.Mux.HandleFunc("DELETE /internal/v1/images/{name}", s.handleImageRemove)
	s.Mux.HandleFunc("GET /internal/v1/images/{name}/history", s.handleImageHistory)
	s.Mux.HandleFunc("POST /internal/v1/images/{name}/push", s.handleImagePush)
	s.Mux.HandleFunc("GET /internal/v1/images/get", s.handleImageSave)
	s.Mux.HandleFunc("GET /internal/v1/images/{name}/get", s.handleImageSave)
	s.Mux.HandleFunc("GET /internal/v1/images/search", s.handleImageSearch)
	s.Mux.HandleFunc("POST /internal/v1/images/prune", s.handleImagePrune)

	s.Mux.HandleFunc("POST /internal/v1/commit", s.handleContainerCommit)

	// System
	s.Mux.HandleFunc("GET /internal/v1/events", s.handleSystemEvents)
	s.Mux.HandleFunc("GET /internal/v1/system/df", s.handleSystemDf)

	// Docker-compatible API routes (same mux, no /internal/v1/ prefix)
	s.registerDockerAPIRoutes()

	// Podman Libpod API routes (container/image/volume/network ops via /libpod/ prefix)
	s.registerLibpodRoutes()
}

// InitDefaultNetwork creates the default bridge, host, and none networks.
// IDs must be full-length hex strings for Podman compatibility.
func (s *BaseServer) InitDefaultNetwork() {
	now := time.Now().UTC().Format(time.RFC3339Nano)
	bridgeID := "b0000000000000000000000000000000000000000000000000000000bridge00"
	s.Store.Networks.Put(bridgeID, api.Network{
		Name:   "bridge",
		ID:     bridgeID,
		Driver: "bridge",
		Scope:  "local",
		IPAM: api.IPAM{
			Driver: "default",
			Config: []api.IPAMConfig{
				{Subnet: "172.17.0.0/16", Gateway: "172.17.0.1"},
			},
		},
		Containers: make(map[string]api.EndpointResource),
		Options:    make(map[string]string),
		Labels:     make(map[string]string),
		Created:    now,
	})
	hostID := "f00000000000000000000000000000000000000000000000000000000host0000"
	s.Store.Networks.Put(hostID, api.Network{
		Name:       "host",
		ID:         hostID,
		Driver:     "host",
		Scope:      "local",
		IPAM:       api.IPAM{Driver: "default"},
		Containers: make(map[string]api.EndpointResource),
		Options:    make(map[string]string),
		Labels:     make(map[string]string),
		Created:    now,
	})
	noneID := "a00000000000000000000000000000000000000000000000000000000none0000"
	s.Store.Networks.Put(noneID, api.Network{
		Name:       "none",
		ID:         noneID,
		Driver:     "null",
		Scope:      "local",
		IPAM:       api.IPAM{Driver: "default"},
		Containers: make(map[string]api.EndpointResource),
		Options:    make(map[string]string),
		Labels:     make(map[string]string),
		Created:    now,
	})
}

// InitDrivers sets up the default driver set for this server.
// Agent drivers handle all cases; operations error when no agent is connected.
func (s *BaseServer) InitDrivers() {
	s.Drivers.Exec = &AgentExecDriver{Store: s.Store, AgentRegistry: s.AgentRegistry, Logger: s.Logger}
	s.Drivers.Filesystem = &AgentFilesystemDriver{Store: s.Store, Logger: s.Logger}
	s.Drivers.Stream = &AgentStreamDriver{Store: s.Store, AgentRegistry: s.AgentRegistry, Logger: s.Logger}

	syntheticNet := &SyntheticNetworkDriver{Store: s.Store, IPAlloc: s.Store.IPAlloc}
	if platformDriver := NewPlatformNetworkDriver(syntheticNet, s.Logger); platformDriver != nil {
		s.Drivers.Network = platformDriver
	} else {
		s.Drivers.Network = syntheticNet
	}
}

// RegisterUI registers a single-page application served from the given filesystem
// at /ui/. A redirect from GET / to /ui/ is also registered. If fsys is nil, this
// is a no-op so backends without a UI are unaffected.
func (s *BaseServer) RegisterUI(fsys fs.FS) {
	if fsys == nil {
		return
	}
	s.Mux.Handle("/ui/", SPAHandler(fsys, "/ui/"))
	s.Mux.HandleFunc("GET /{$}", func(w http.ResponseWriter, r *http.Request) {
		http.Redirect(w, r, "/ui/", http.StatusTemporaryRedirect)
	})
	s.Logger.Info().Msg("UI registered at /ui/")
}

// RecoverRegistry loads persisted registry state and scans the cloud for orphaned resources.
func (s *BaseServer) RecoverRegistry(ctx context.Context, scanner CloudScanner) error {
	s.Logger.Info().Msg("recovering resource registry")
	if err := RecoverOnStartup(ctx, s.Registry, scanner, s.Desc.InstanceID); err != nil {
		return fmt.Errorf("registry recovery failed: %w", err)
	}
	active := s.Registry.ListActive()
	recovered := ReconstructContainerState(s.Store, s.Registry)
	s.Logger.Info().
		Int("active_resources", len(active)).
		Int("recovered_containers", recovered).
		Msg("registry recovery complete")
	return nil
}

// LoggingMiddleware logs HTTP requests at Debug level with method, path, status, and duration.
func LoggingMiddleware(logger zerolog.Logger, next http.Handler) http.Handler {
	return http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		start := time.Now()
		rec := &statusRecorder{ResponseWriter: w, status: http.StatusOK}
		next.ServeHTTP(rec, r)
		logger.Debug().
			Str("method", r.Method).
			Str("path", r.URL.Path).
			Int("status", rec.status).
			Dur("duration", time.Since(start)).
			Msg("http request")
	})
}

type statusRecorder struct {
	http.ResponseWriter
	status int
}

func (sr *statusRecorder) WriteHeader(code int) {
	sr.status = code
	sr.ResponseWriter.WriteHeader(code)
}

// Hijack implements http.Hijacker so exec/attach connection upgrades work.
func (sr *statusRecorder) Hijack() (net.Conn, *bufio.ReadWriter, error) {
	if h, ok := sr.ResponseWriter.(http.Hijacker); ok {
		return h.Hijack()
	}
	return nil, nil, fmt.Errorf("hijack not supported")
}

// Flush implements http.Flusher so streaming responses work.
func (sr *statusRecorder) Flush() {
	if f, ok := sr.ResponseWriter.(http.Flusher); ok {
		f.Flush()
	}
}

// ListenAndServe starts the HTTP server.
// If addr starts with /, it listens on a Unix socket (TLS is ignored).
// If certFile and keyFile are both non-empty, the TCP listener uses TLS.
func (s *BaseServer) ListenAndServe(addr, certFile, keyFile string) error {
	// Crash-only startup: load persisted registry state
	if err := s.Registry.Load(); err != nil {
		s.Logger.Warn().Err(err).Msg("failed to load resource registry")
	} else if active := s.Registry.ListActive(); len(active) > 0 {
		s.Logger.Info().Int("active_resources", len(active)).Msg("loaded resource registry from disk")
	}

	wrapped := stripVersionPrefix(s.Mux)
	handler := otelhttp.NewHandler(LoggingMiddleware(s.Logger, MetricsMiddleware(s.Metrics, wrapped)), "sockerless-backend")

	if strings.HasPrefix(addr, "/") {
		os.Remove(addr)
		listener, err := net.Listen("unix", addr)
		if err != nil {
			return err
		}
		defer func() { _ = listener.Close() }()
		srv := &http.Server{Handler: handler}
		return srv.Serve(listener)
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: handler,
	}
	if certFile != "" && keyFile != "" {
		cert, err := tls.LoadX509KeyPair(certFile, keyFile)
		if err != nil {
			return err
		}
		srv.TLSConfig = &tls.Config{Certificates: []tls.Certificate{cert}}
		return srv.ListenAndServeTLS("", "")
	}
	return srv.ListenAndServe()
}

// handleInfo returns backend system information.
func (s *BaseServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	running := 0
	paused := 0
	stopped := 0
	for _, c := range s.Store.Containers.List() {
		if c.State.Paused {
			paused++
			running++ // Docker counts paused as subset of running
		} else if c.State.Running {
			running++
		} else {
			stopped++
		}
	}
	WriteJSON(w, http.StatusOK, &api.BackendInfo{
		ID:                s.Desc.ID,
		Name:              s.Desc.Name,
		ServerVersion:     s.Desc.ServerVersion,
		Containers:        s.Store.Containers.Len(),
		ContainersRunning: running,
		ContainersPaused:  paused,
		ContainersStopped: stopped,
		Images:            s.Store.Images.Len(),
		Driver:            s.Desc.Driver,
		OperatingSystem:   s.Desc.OperatingSystem,
		OSType:            s.Desc.OSType,
		Architecture:      s.Desc.Architecture,
		NCPU:              s.Desc.NCPU,
		MemTotal:          s.Desc.MemTotal,
		KernelVersion:     s.kernelVersion(),
	})
}

// handleResourceList returns tracked resources, optionally filtered by active status.
func (s *BaseServer) handleResourceList(w http.ResponseWriter, r *http.Request) {
	active := r.URL.Query().Get("active")
	var entries []ResourceEntry
	if active == "true" {
		entries = s.Registry.ListActive()
	} else {
		entries = s.Registry.ListAll()
	}
	if entries == nil {
		entries = []ResourceEntry{}
	}
	WriteJSON(w, http.StatusOK, entries)
}

// handleResourceOrphaned returns active resources older than max_age.
func (s *BaseServer) handleResourceOrphaned(w http.ResponseWriter, r *http.Request) {
	maxAgeStr := r.URL.Query().Get("max_age")
	maxAge := time.Hour // default 1 hour
	if maxAgeStr != "" {
		if d, err := time.ParseDuration(maxAgeStr); err == nil {
			maxAge = d
		}
	}
	entries := s.Registry.ListOrphaned(maxAge)
	if entries == nil {
		entries = []ResourceEntry{}
	}
	WriteJSON(w, http.StatusOK, entries)
}

// handleResourceCleanup triggers best-effort cleanup of orphaned resources.
func (s *BaseServer) handleResourceCleanup(w http.ResponseWriter, r *http.Request) {
	maxAgeStr := r.URL.Query().Get("max_age")
	maxAge := time.Hour
	if maxAgeStr != "" {
		if d, err := time.ParseDuration(maxAgeStr); err == nil {
			maxAge = d
		}
	}
	orphans := s.Registry.ListOrphaned(maxAge)
	for _, o := range orphans {
		s.Registry.MarkCleanedUp(o.ResourceID)
	}
	WriteJSON(w, http.StatusOK, map[string]any{
		"cleaned": len(orphans),
	})
}
