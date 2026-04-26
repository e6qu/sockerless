package azf

import (
	"context"
	"sync/atomic"

	"github.com/rs/zerolog"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// Server is the Azure Functions backend server.
type Server struct {
	*core.BaseServer
	config    Config
	azure     *AzureClients
	images    *core.ImageManager
	ipCounter atomic.Int32

	AZF *core.StateStore[AZFState]
	azfVolumeState
	// Reverse-agent registry for docker top / cp / stat via a
	// bootstrap running inside the function app container.
	reverseAgents *core.ReverseAgentRegistry
}

// NewServer creates a new Azure Functions backend server.
func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
	s := &Server{
		config:         config,
		azure:          azureClients,
		AZF:            core.NewStateStore[AZFState](),
		azfVolumeState: azfVolumeState{shares: azurecommon.NewFileShareManager(azureClients.FileShares, config.ResourceGroup, config.StorageAccount)},
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
	s.images = &core.ImageManager{
		Base:   s.BaseServer,
		Auth:   azurecommon.NewACRAuthProvider(logger),
		Logger: logger,
	}
	if svc, err := azurecommon.NewACRBuildService(
		azureClients.Cred, config.SubscriptionID, config.ResourceGroup,
		config.Registry, config.BuildStorageAccount, config.BuildContainer, logger,
	); err == nil && svc != nil {
		s.images.BuildService = svc
	}
	s.CloudState = &azfCloudState{server: s}
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

	// Reverse-agent registry + WS endpoint.
	s.reverseAgents = core.NewReverseAgentRegistry()
	s.Mux.HandleFunc("/v1/azf/reverse", core.HandleReverseAgentWS(s.reverseAgents, logger))
	s.Drivers.Exec = &core.ReverseAgentExecDriver{Registry: s.reverseAgents, Logger: logger}
	s.Drivers.Stream = &core.ReverseAgentStreamDriver{Registry: s.reverseAgents, Logger: logger}
	s.Typed.Exec = core.WrapLegacyExec(s.Drivers.Exec, "azf", "ReverseAgentExec")

	// Cloud-native typed drivers for Logs + Attach. Both go through
	// Azure Monitor / Log Analytics via a per-container fetcher factory.
	logFactory := func(containerID string) core.CloudLogFetchFunc {
		return s.buildCloudLogsFetcher(containerID)
	}
	s.Typed.Logs = core.NewCloudLogsLogsDriver(s.BaseServer, logFactory,
		core.StreamCloudLogsOptions{CheckLogBuffers: true},
		"azf", "AzureMonitor")
	s.Typed.Attach = core.NewCloudLogsAttachDriver(s.BaseServer, logFactory,
		"azf", "CloudLogsReadOnlyAttach")

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
