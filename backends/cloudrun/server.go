package cloudrun

import (
	"context"
	"sync/atomic"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Cloud Run backend server.
type Server struct {
	*core.BaseServer
	config    Config
	gcp       *GCPClients
	arAuth    *ARAuthProvider
	ipCounter atomic.Int32

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
	s.arAuth = NewARAuthProvider(s.ctx)

	s.ipCounter.Store(2)

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
	}, logger)
	s.SetSelf(s)

	mode := "cloud"
	if config.EndpointURL != "" {
		mode = "custom-endpoint"
	}
	s.ProviderInfo = &core.ProviderInfo{
		Provider: "gcp",
		Mode:     mode,
		Region:   config.Region,
		Endpoint: config.EndpointURL,
		Resources: map[string]string{
			"project":       config.Project,
			"vpc_connector": config.VPCConnector,
		},
	}

	registerUI(s.BaseServer)

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
