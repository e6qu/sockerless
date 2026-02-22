package docker

import (
	"net/http"

	dockerclient "github.com/docker/docker/client"
	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Docker passthrough backend HTTP server.
type Server struct {
	docker *dockerclient.Client
	logger zerolog.Logger
	mux    *http.ServeMux
}

// NewServer creates a new Docker backend server.
func NewServer(logger zerolog.Logger, dockerHost string) (*Server, error) {
	opts := []dockerclient.Opt{
		dockerclient.WithAPIVersionNegotiation(),
	}
	if dockerHost != "" {
		opts = append(opts, dockerclient.WithHost(dockerHost))
	} else {
		opts = append(opts, dockerclient.FromEnv)
	}

	cli, err := dockerclient.NewClientWithOpts(opts...)
	if err != nil {
		return nil, err
	}

	s := &Server{
		docker: cli,
		logger: logger,
		mux:    http.NewServeMux(),
	}
	s.registerRoutes()
	return s, nil
}

func (s *Server) registerRoutes() {
	// System
	s.mux.HandleFunc("GET /internal/v1/info", s.handleInfo)

	// Containers
	s.mux.HandleFunc("POST /internal/v1/containers", s.handleContainerCreate)
	s.mux.HandleFunc("GET /internal/v1/containers", s.handleContainerList)
	s.mux.HandleFunc("GET /internal/v1/containers/{id}", s.handleContainerInspect)
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/start", s.handleContainerStart)
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/stop", s.handleContainerStop)
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/kill", s.handleContainerKill)
	s.mux.HandleFunc("DELETE /internal/v1/containers/{id}", s.handleContainerRemove)
	s.mux.HandleFunc("GET /internal/v1/containers/{id}/logs", s.handleContainerLogs)
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/wait", s.handleContainerWait)
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/attach", s.handleContainerAttach)

	// Exec
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/exec", s.handleExecCreate)
	s.mux.HandleFunc("GET /internal/v1/exec/{id}", s.handleExecInspect)
	s.mux.HandleFunc("POST /internal/v1/exec/{id}/start", s.handleExecStart)

	// Images
	s.mux.HandleFunc("POST /internal/v1/images/pull", s.handleImagePull)
	s.mux.HandleFunc("GET /internal/v1/images/inspect", s.handleImageInspect)
	s.mux.HandleFunc("POST /internal/v1/images/load", s.handleImageLoad)
	s.mux.HandleFunc("POST /internal/v1/images/tag", s.handleImageTag)

	// Auth
	s.mux.HandleFunc("POST /internal/v1/auth", s.handleAuth)

	// Networks
	s.mux.HandleFunc("POST /internal/v1/networks", s.handleNetworkCreate)
	s.mux.HandleFunc("GET /internal/v1/networks", s.handleNetworkList)
	s.mux.HandleFunc("GET /internal/v1/networks/{id}", s.handleNetworkInspect)
	s.mux.HandleFunc("POST /internal/v1/networks/{id}/disconnect", s.handleNetworkDisconnect)
	s.mux.HandleFunc("DELETE /internal/v1/networks/{id}", s.handleNetworkRemove)
	s.mux.HandleFunc("POST /internal/v1/networks/prune", s.handleNetworkPrune)

	// Volumes
	s.mux.HandleFunc("POST /internal/v1/volumes", s.handleVolumeCreate)
	s.mux.HandleFunc("GET /internal/v1/volumes", s.handleVolumeList)
	s.mux.HandleFunc("GET /internal/v1/volumes/{name}", s.handleVolumeInspect)
	s.mux.HandleFunc("DELETE /internal/v1/volumes/{name}", s.handleVolumeRemove)

	// Extended
	s.mux.HandleFunc("POST /internal/v1/volumes/prune", s.handleVolumePrune)
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/restart", s.handleContainerRestart)
	s.mux.HandleFunc("GET /internal/v1/containers/{id}/top", s.handleContainerTop)
	s.mux.HandleFunc("POST /internal/v1/containers/prune", s.handleContainerPrune)
	s.mux.HandleFunc("GET /internal/v1/containers/{id}/stats", s.handleContainerStats)
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/rename", s.handleContainerRename)
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/pause", s.handleContainerPause)
	s.mux.HandleFunc("POST /internal/v1/containers/{id}/unpause", s.handleContainerUnpause)
	s.mux.HandleFunc("POST /internal/v1/networks/{id}/connect", s.handleNetworkConnect)
	s.mux.HandleFunc("GET /internal/v1/images", s.handleImageList)
	s.mux.HandleFunc("DELETE /internal/v1/images/{name}", s.handleImageRemove)
	s.mux.HandleFunc("GET /internal/v1/images/{name}/history", s.handleImageHistory)
	s.mux.HandleFunc("POST /internal/v1/images/prune", s.handleImagePrune)
	s.mux.HandleFunc("GET /internal/v1/events", s.handleSystemEvents)
	s.mux.HandleFunc("GET /internal/v1/system/df", s.handleSystemDf)
}

// ListenAndServe starts the HTTP server.
func (s *Server) ListenAndServe(addr string) error {
	srv := &http.Server{
		Addr:    addr,
		Handler: s.mux,
	}
	return srv.ListenAndServe()
}

// Aliases for core helpers used throughout this backend.
var (
	writeJSON  = core.WriteJSON
	writeError = core.WriteError
	readJSON   = core.ReadJSON
)
