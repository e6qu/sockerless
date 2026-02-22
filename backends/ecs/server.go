package ecs

import (
	"context"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the ECS backend server.
type Server struct {
	*core.BaseServer
	config       Config
	aws          *AWSClients
	ECS          *core.StateStore[ECSState]
	NetworkState *core.StateStore[NetworkState]
	VolumeState  *core.StateStore[VolumeState]
}

// NewServer creates a new ECS backend server.
func NewServer(config Config, awsClients *AWSClients, logger zerolog.Logger) *Server {
	s := &Server{
		config:       config,
		aws:          awsClients,
		ECS:          core.NewStateStore[ECSState](),
		NetworkState: core.NewStateStore[NetworkState](),
		VolumeState:  core.NewStateStore[VolumeState](),
	}

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "ecs-backend-" + config.Cluster,
		Name:            "sockerless-ecs",
		ServerVersion:   "0.1.0",
		Driver:          "ecs-fargate",
		OperatingSystem: "AWS Fargate",
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
