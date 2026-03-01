package cloudrun

import (
	"context"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Cloud Run backend server.
type Server struct {
	*core.BaseServer
	config Config
	gcp    *GCPClients

	CloudRun     *core.StateStore[CloudRunState]
	NetworkState *core.StateStore[NetworkState]
	VolumeState  *core.StateStore[VolumeState]
}

// NewServer creates a new Cloud Run backend server.
func NewServer(config Config, gcpClients *GCPClients, logger zerolog.Logger) *Server {
	s := &Server{
		config:       config,
		gcp:          gcpClients,
		CloudRun:     core.NewStateStore[CloudRunState](),
		NetworkState: core.NewStateStore[NetworkState](),
		VolumeState:  core.NewStateStore[VolumeState](),
	}

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "cloudrun-backend-" + config.Project,
		Name:            "sockerless-cloudrun",
		ServerVersion:   "0.1.0",
		Driver:          "cloudrun-jobs",
		OperatingSystem: "Google Cloud Run",
		OSType:          "linux",
		Architecture:    "amd64",
		NCPU:            1,
		MemTotal:        536870912,
	}, core.RouteOverrides{
		ContainerCreate:  s.handleContainerCreate,
		ContainerStart:   s.handleContainerStart,
		ContainerStop:    s.handleContainerStop,
		ContainerKill:    s.handleContainerKill,
		ContainerRemove:  s.handleContainerRemove,
		ContainerLogs:    s.handleContainerLogs,
		ContainerRestart: s.handleContainerRestart,
		ContainerPrune:   s.handleContainerPrune,
		ContainerPause:   s.handleContainerPause,
		ContainerUnpause: s.handleContainerUnpause,
		ImagePull:        s.handleImagePull,
		ImageLoad:        s.handleImageLoad,
		VolumeRemove:     s.handleVolumeRemove,
		VolumePrune:      s.handleVolumePrune,
	}, logger)

	registerUI(s.BaseServer)

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
