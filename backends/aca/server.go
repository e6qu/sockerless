package aca

import (
	"context"
	"sync/atomic"

	"github.com/rs/zerolog"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// Server is the ACA backend server.
type Server struct {
	*core.BaseServer
	config    Config
	azure     *AzureClients
	images    *core.ImageManager
	ipCounter atomic.Int32

	ACA          *core.StateStore[ACAState]
	NetworkState *core.StateStore[NetworkState]
	azureVolumeState
	// Reverse-agent registry for docker exec / attach through a
	// bootstrap running inside the ACA Job/App container.
	reverseAgents *core.ReverseAgentRegistry
}

// NewServer creates a new ACA backend server.
func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
	s := &Server{
		config:           config,
		azure:            azureClients,
		ACA:              core.NewStateStore[ACAState](),
		NetworkState:     core.NewStateStore[NetworkState](),
		azureVolumeState: azureVolumeState{shares: azurecommon.NewFileShareManager(azureClients.FileShares, config.ResourceGroup, config.StorageAccount)},
	}
	s.ipCounter.Store(2)

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
	}, logger)
	s.images = &core.ImageManager{
		Base:   s.BaseServer,
		Auth:   azurecommon.NewACRAuthProvider(logger),
		Logger: logger,
	}
	if svc, err := azurecommon.NewACRBuildService(
		azureClients.Cred, config.SubscriptionID, config.ResourceGroup,
		config.ACRName, config.BuildStorageAccount, config.BuildContainer, logger,
	); err == nil && svc != nil {
		s.images.BuildService = svc
	}
	s.SetSelf(s)
	s.CloudState = &acaCloudState{server: s}

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
			"environment":     config.Environment,
		},
	}

	registerUI(s.BaseServer)

	// Reverse-agent registry + WS endpoint (see cloudrun for design
	// notes). Container-side bootstrap dials SOCKERLESS_CALLBACK_URL →
	// /v1/aca/reverse?session_id=<container>.
	s.reverseAgents = core.NewReverseAgentRegistry()
	s.Mux.HandleFunc("/v1/aca/reverse", core.HandleReverseAgentWS(s.reverseAgents, logger))
	s.Drivers.Exec = &core.ReverseAgentExecDriver{Registry: s.reverseAgents, Logger: logger}
	s.Drivers.Stream = &core.ReverseAgentStreamDriver{Registry: s.reverseAgents, Logger: logger}
	s.Typed.Exec = core.WrapLegacyExec(s.Drivers.Exec, "aca", "ReverseAgentExec")
	s.Typed.ProcList = core.NewReverseAgentProcListDriver(s.reverseAgents, "aca")
	s.Typed.FSDiff = core.NewReverseAgentFSDiffDriver(s.reverseAgents, "aca")
	s.Typed.FSRead = core.NewReverseAgentFSReadDriver(s.reverseAgents, "aca")
	s.Typed.FSWrite = core.NewReverseAgentFSWriteDriver(s.reverseAgents, "aca")
	s.Typed.FSExport = core.NewReverseAgentFSExportDriver(s.reverseAgents, "aca")
	s.Typed.Commit = core.NewReverseAgentCommitDriver(s.BaseServer, s.reverseAgents, "aca")

	// Cloud-native typed Logs + Attach driving Azure Monitor / Log
	// Analytics via the per-container fetcher factory.
	logFactory := func(containerID string) core.CloudLogFetchFunc {
		return s.buildCloudLogsFetcher(containerID)
	}
	s.Typed.Logs = core.NewCloudLogsLogsDriver(s.BaseServer, logFactory,
		core.StreamCloudLogsOptions{},
		"aca", "AzureMonitor")
	s.Typed.Attach = core.NewCloudLogsAttachDriver(s.BaseServer, logFactory,
		"aca", "CloudLogsReadOnlyAttach")

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
