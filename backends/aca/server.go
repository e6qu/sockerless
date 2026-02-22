package aca

import (
	"context"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the ACA backend server.
type Server struct {
	*core.BaseServer
	config Config
	azure  *AzureClients

	ACA          *core.StateStore[ACAState]
	NetworkState *core.StateStore[NetworkState]
	VolumeState  *core.StateStore[VolumeState]
}

// NewServer creates a new ACA backend server.
func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
	s := &Server{
		config:       config,
		azure:        azureClients,
		ACA:          core.NewStateStore[ACAState](),
		NetworkState: core.NewStateStore[NetworkState](),
		VolumeState:  core.NewStateStore[VolumeState](),
	}

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "aca-backend-" + config.ResourceGroup,
		Name:            "sockerless-aca",
		ServerVersion:   "0.1.0",
		Driver:          "aca-jobs",
		OperatingSystem: "Azure Container Apps",
		OSType:          "linux",
		Architecture:    "amd64",
		NCPU:            2,
		MemTotal:        4294967296,
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

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
