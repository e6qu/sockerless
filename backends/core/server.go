package core

import (
	"context"
	"fmt"
	"io/fs"
	"net/http"
	"os"
	"path/filepath"
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
	NCPU            int
	MemTotal        int64
	InstanceID      string
}

// RouteOverrides allows backends to provide custom handlers for operations
// that differ between backends. If a field is nil, the default (memory-like)
// implementation is used.
type RouteOverrides struct {
	ContainerCreate  http.HandlerFunc
	ContainerStart   http.HandlerFunc
	ContainerStop    http.HandlerFunc
	ContainerKill    http.HandlerFunc
	ContainerRemove  http.HandlerFunc
	ContainerLogs    http.HandlerFunc
	ContainerAttach  http.HandlerFunc
	ContainerRestart http.HandlerFunc
	ContainerPrune   http.HandlerFunc
	ContainerPause   http.HandlerFunc
	ContainerUnpause http.HandlerFunc
	ExecStart        http.HandlerFunc
	ImagePull        http.HandlerFunc
	ImageLoad        http.HandlerFunc
	VolumeRemove     http.HandlerFunc
	VolumePrune      http.HandlerFunc
}

// BaseServer is the common HTTP server used by all non-Docker backends.
type BaseServer struct {
	Store          *Store
	Logger         zerolog.Logger
	Desc           BackendDescriptor
	Mux            *http.ServeMux
	AgentRegistry  *AgentRegistry
	ProcessFactory ProcessFactory
	Drivers        DriverSet
	Registry       *ResourceRegistry
	StartedAt      time.Time
	Metrics        *Metrics
	HealthChecker  HealthChecker
	EventBus       *EventBus
}

// NewBaseServer creates a new base server with the given store, descriptor,
// route overrides, and logger. It registers all routes and initializes the
// default bridge network.
func NewBaseServer(store *Store, desc BackendDescriptor, overrides RouteOverrides, logger zerolog.Logger) *BaseServer {
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
	s.InitDrivers()
	store.RestartHook = s.handleRestartPolicy
	s.registerRoutes(overrides)
	s.InitDefaultNetwork()
	return s
}

func (s *BaseServer) registerRoutes(o RouteOverrides) {
	or := func(override http.HandlerFunc, def http.HandlerFunc) http.HandlerFunc {
		if override != nil {
			return override
		}
		return def
	}

	// System
	s.Mux.HandleFunc("GET /internal/v1/info", s.handleInfo)

	// Management
	s.Mux.HandleFunc("GET /internal/v1/healthz", s.handleHealthz)
	s.Mux.HandleFunc("GET /internal/v1/status", s.handleMgmtStatus)
	s.Mux.HandleFunc("GET /internal/v1/containers/summary", s.handleContainerSummary)
	s.Mux.HandleFunc("GET /internal/v1/metrics", s.handleMetrics)
	s.Mux.HandleFunc("GET /internal/v1/check", s.handleCheck)
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

	// Containers
	s.Mux.HandleFunc("POST /internal/v1/containers", or(o.ContainerCreate, s.handleContainerCreate))
	s.Mux.HandleFunc("GET /internal/v1/containers", s.handleContainerList)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}", s.handleContainerInspect)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/start", or(o.ContainerStart, s.handleContainerStart))
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/stop", or(o.ContainerStop, s.handleContainerStop))
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/kill", or(o.ContainerKill, s.handleContainerKill))
	s.Mux.HandleFunc("DELETE /internal/v1/containers/{id}", or(o.ContainerRemove, s.handleContainerRemove))
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/logs", or(o.ContainerLogs, s.handleContainerLogs))
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/wait", s.handleContainerWait)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/attach", or(o.ContainerAttach, s.handleContainerAttach))

	// Exec
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/exec", s.handleExecCreate)
	s.Mux.HandleFunc("GET /internal/v1/exec/{id}", s.handleExecInspect)
	s.Mux.HandleFunc("POST /internal/v1/exec/{id}/start", or(o.ExecStart, s.handleExecStart))

	// Images
	s.Mux.HandleFunc("POST /internal/v1/images/pull", or(o.ImagePull, s.handleImagePull))
	s.Mux.HandleFunc("GET /internal/v1/images/inspect", s.handleImageInspect)
	s.Mux.HandleFunc("POST /internal/v1/images/load", or(o.ImageLoad, s.handleImageLoad))
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
	s.Mux.HandleFunc("DELETE /internal/v1/volumes/{name}", or(o.VolumeRemove, s.handleVolumeRemove))
	s.Mux.HandleFunc("POST /internal/v1/volumes/prune", or(o.VolumePrune, s.handleVolumePrune))

	// Container archive (copy files to/from container)
	s.Mux.HandleFunc("PUT /internal/v1/containers/{id}/archive", s.handlePutArchive)
	s.Mux.HandleFunc("HEAD /internal/v1/containers/{id}/archive", s.handleHeadArchive)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/archive", s.handleGetArchive)

	// Extended container operations
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/restart", or(o.ContainerRestart, s.handleContainerRestart))
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/top", s.handleContainerTop)
	s.Mux.HandleFunc("POST /internal/v1/containers/prune", or(o.ContainerPrune, s.handleContainerPrune))
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/stats", s.handleContainerStats)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/rename", s.handleContainerRename)
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/pause", or(o.ContainerPause, s.handleContainerPause))
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/unpause", or(o.ContainerUnpause, s.handleContainerUnpause))
	s.Mux.HandleFunc("POST /internal/v1/containers/{id}/update", s.handleContainerUpdate)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/changes", s.handleContainerChanges)
	s.Mux.HandleFunc("GET /internal/v1/containers/{id}/export", s.handleContainerExport)

	s.Mux.HandleFunc("POST /internal/v1/networks/{id}/connect", s.handleNetworkConnect)

	// Extended image operations
	s.Mux.HandleFunc("GET /internal/v1/images", s.handleImageList)
	s.Mux.HandleFunc("DELETE /internal/v1/images/{name}", s.handleImageRemove)
	s.Mux.HandleFunc("GET /internal/v1/images/{name}/history", s.handleImageHistory)
	s.Mux.HandleFunc("POST /internal/v1/images/prune", s.handleImagePrune)

	s.Mux.HandleFunc("POST /internal/v1/commit", s.handleContainerCommit)

	// System
	s.Mux.HandleFunc("GET /internal/v1/events", s.handleSystemEvents)
	s.Mux.HandleFunc("GET /internal/v1/system/df", s.handleSystemDf)
}

// InitDefaultNetwork creates the default bridge network.
func (s *BaseServer) InitDefaultNetwork() {
	s.Store.Networks.Put("bridge", api.Network{
		Name:   "bridge",
		ID:     "bridge",
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
		Created:    time.Now().UTC().Format(time.RFC3339Nano),
	})
}

// InitDrivers sets up the default driver chain for this server.
// Backends that inject a ProcessFactory should call this after setting it.
// The chain is: Agent → WASM (if ProcessFactory set) → Synthetic.
func (s *BaseServer) InitDrivers() {
	syntheticExec := &SyntheticExecDriver{}
	syntheticFS := &SyntheticFilesystemDriver{Store: s.Store}
	syntheticStream := &SyntheticStreamDriver{Store: s.Store}

	// Build exec chain
	var execDriver ExecDriver = syntheticExec
	if s.ProcessFactory != nil {
		execDriver = &WASMExecDriver{
			Store:          s.Store,
			ProcessFactory: s.ProcessFactory,
			Fallback:       syntheticExec,
		}
	}
	s.Drivers.Exec = &AgentExecDriver{
		Store:         s.Store,
		AgentRegistry: s.AgentRegistry,
		Logger:        s.Logger,
		Fallback:      execDriver,
	}

	// Build filesystem chain
	var fsDriver FilesystemDriver = syntheticFS
	if s.ProcessFactory != nil {
		fsDriver = &WASMFilesystemDriver{
			Store:    s.Store,
			Fallback: syntheticFS,
		}
	}
	s.Drivers.Filesystem = &AgentFilesystemDriver{
		Store:    s.Store,
		Logger:   s.Logger,
		Fallback: fsDriver,
	}

	// Build stream chain
	var streamDriver StreamDriver = syntheticStream
	if s.ProcessFactory != nil {
		streamDriver = &WASMStreamDriver{
			Store:    s.Store,
			Fallback: syntheticStream,
		}
	}
	s.Drivers.Stream = &AgentStreamDriver{
		Store:         s.Store,
		AgentRegistry: s.AgentRegistry,
		Logger:        s.Logger,
		Fallback:      streamDriver,
	}

	// Build process lifecycle
	if s.ProcessFactory != nil {
		s.Drivers.ProcessLifecycle = &WASMProcessLifecycleDriver{
			Store:          s.Store,
			ProcessFactory: s.ProcessFactory,
		}
	} else {
		s.Drivers.ProcessLifecycle = &SyntheticProcessLifecycleDriver{}
	}

	// Build network driver
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

// ListenAndServe starts the HTTP server.
func (s *BaseServer) ListenAndServe(addr string) error {
	// Crash-only startup: load persisted registry state
	if err := s.Registry.Load(); err != nil {
		s.Logger.Warn().Err(err).Msg("failed to load resource registry")
	} else if active := s.Registry.ListActive(); len(active) > 0 {
		s.Logger.Info().Int("active_resources", len(active)).Msg("loaded resource registry from disk")
	}

	srv := &http.Server{
		Addr:    addr,
		Handler: otelhttp.NewHandler(MetricsMiddleware(s.Metrics, s.Mux), "sockerless-backend"),
	}
	return srv.ListenAndServe()
}

// handleInfo returns backend system information.
func (s *BaseServer) handleInfo(w http.ResponseWriter, r *http.Request) {
	running := 0
	stopped := 0
	for _, c := range s.Store.Containers.List() {
		if c.State.Running {
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
		ContainersStopped: stopped,
		Images:            s.Store.Images.Len(),
		Driver:            s.Desc.Driver,
		OperatingSystem:   s.Desc.OperatingSystem,
		OSType:            s.Desc.OSType,
		Architecture:      s.Desc.Architecture,
		NCPU:              s.Desc.NCPU,
		MemTotal:          s.Desc.MemTotal,
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
