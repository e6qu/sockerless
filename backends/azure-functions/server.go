package azf

import (
	"context"
	"sync/atomic"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
)

// Server is the Azure Functions backend server.
type Server struct {
	*core.BaseServer
	config    Config
	azure     *AzureClients
	ipCounter atomic.Int32

	AZF *core.StateStore[AZFState]
}

// NewServer creates a new Azure Functions backend server.
func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
	s := &Server{
		config: config,
		azure:  azureClients,
		AZF:    core.NewStateStore[AZFState](),
	}
	s.ipCounter.Store(2)

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
	}, logger)
	s.SetSelf(s)

	mode := "cloud"
	if config.EndpointURL != "" {
		mode = "custom-endpoint"
	}
	s.ProviderInfo = &core.ProviderInfo{
		Provider: "azure",
		Mode:     mode,
		Region:   config.Location,
		Endpoint: config.EndpointURL,
		Resources: map[string]string{
			"subscription_id": config.SubscriptionID,
			"resource_group":  config.ResourceGroup,
			"storage_account": config.StorageAccount,
		},
	}

	registerUI(s.BaseServer)

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
