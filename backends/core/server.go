package core

import (
	"net/http"
	"time"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
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
}

// NewBaseServer creates a new base server with the given store, descriptor,
// route overrides, and logger. It registers all routes and initializes the
// default bridge network.
func NewBaseServer(store *Store, desc BackendDescriptor, overrides RouteOverrides, logger zerolog.Logger) *BaseServer {
	s := &BaseServer{
		Store:         store,
		Logger:        logger,
		Desc:          desc,
		Mux:           http.NewServeMux(),
		AgentRegistry: NewAgentRegistry(),
	}
	s.InitDrivers()
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

	// Agent reverse connection
	s.Mux.HandleFunc("GET /internal/v1/agent/connect", s.handleAgentConnect)

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

	// Extended network operations
	s.Mux.HandleFunc("POST /internal/v1/networks/{id}/connect", s.handleNetworkConnect)

	// Extended image operations
	s.Mux.HandleFunc("GET /internal/v1/images", s.handleImageList)
	s.Mux.HandleFunc("DELETE /internal/v1/images/{name}", s.handleImageRemove)
	s.Mux.HandleFunc("GET /internal/v1/images/{name}/history", s.handleImageHistory)
	s.Mux.HandleFunc("POST /internal/v1/images/prune", s.handleImagePrune)

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
}

// ListenAndServe starts the HTTP server.
func (s *BaseServer) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.Mux,
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
