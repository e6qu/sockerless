package gcf

import (
	"context"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Cloud Run Functions backend server.
type Server struct {
	*core.BaseServer
	config Config
	gcp    *GCPClients

	GCF *core.StateStore[GCFState]
}

// NewServer creates a new Cloud Run Functions backend server.
func NewServer(config Config, gcpClients *GCPClients, logger zerolog.Logger) *Server {
	s := &Server{
		config: config,
		gcp:    gcpClients,
		GCF:    core.NewStateStore[GCFState](),
	}

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "gcf-backend",
		Name:            "sockerless-gcf",
		ServerVersion:   "0.1.0",
		Driver:          "cloud-run-functions",
		OperatingSystem: "Google Cloud Functions",
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
		ContainerPrune: s.handleContainerPrune,
		ImagePull:      s.handleImagePull,
		ImageLoad:      s.handleImageLoad,
	}, logger)

	registerUI(s.BaseServer)

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
