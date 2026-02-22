package azf

import (
	"context"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Azure Functions backend server.
type Server struct {
	*core.BaseServer
	config Config
	azure  *AzureClients

	AZF *core.StateStore[AZFState]
}

// NewServer creates a new Azure Functions backend server.
func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
	s := &Server{
		config: config,
		azure:  azureClients,
		AZF:    core.NewStateStore[AZFState](),
	}

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "azf-backend-" + config.ResourceGroup,
		Name:            "sockerless-azf",
		ServerVersion:   "0.1.0",
		Driver:          "azure-functions",
		OperatingSystem: "Azure Functions",
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

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
