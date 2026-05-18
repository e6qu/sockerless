package aca

import (
	"context"
	"sync/atomic"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
	core "github.com/sockerless/backend-core"
)

// Server is the ACA backend server.
type Server struct {
	*core.BaseServer
	config          Config
	azure           *AzureClients
	images          *core.ImageManager
	storageBackings *core.StorageBackingRegistry
	ipCounter       atomic.Int32

	ACA          *core.StateStore[ACAState]
	NetworkState *core.StateStore[NetworkState]
	azureVolumeState
	// Reverse-agent registry for docker exec / attach through a
	// bootstrap running inside the ACA Job/App container.
	reverseAgents *core.ReverseAgentRegistry
}

// NewServer creates a new ACA backend server.
func NewServer(config Config, azureClients *AzureClients, logger zerolog.Logger) *Server {
	if config.CallbackURL == "" {
		logger.Fatal().Msg("ACA backend requires SOCKERLESS_CALLBACK_URL — the in-App/Job bootstrap dials back here to register the reverse-agent WebSocket. Without it every exec fails (no fallback to management-API exec). Set the env var to a URL the App / Job can reach.")
	}
	if config.BootstrapBinaryHash == "" && config.BootstrapBinaryPath != "" {
		if hash, err := hashBootstrapBinary(config.BootstrapBinaryPath); err == nil {
			config.BootstrapBinaryHash = hash
			logger.Info().Str("path", config.BootstrapBinaryPath).Str("hash", hash).Msg("hashed aca bootstrap binary for overlay-tag invalidation")
		} else {
			logger.Warn().Err(err).Str("path", config.BootstrapBinaryPath).Msg("failed to hash aca bootstrap binary; overlay images will not invalidate on bootstrap update")
		}
	}
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
	s.storageBackings = core.NewStorageBackingRegistry()
	s.storageBackings.Register(azurecommon.NewAzureFilesEphemeralDriver(config.StorageAccount))
	tmpfsMiB, terr := core.TmpfsSizeFromEnv("aca")
	if terr != nil {
		logger.Fatal().Err(terr).Msg("invalid SOCKERLESS_ACA_TMPFS_SIZE_MIB")
	}
	s.storageBackings.Register(core.NewMemoryDriver(tmpfsMiB))
	// Default backing for SharedVolumes without explicit Backing:
	// memory (tmpfs). ACA Apps + Jobs accept EmptyDir{Medium: MEMORY}
	// natively. Operators wanting durability must set Backing:
	// azure-files-ephemeral explicitly.
	s.storageBackings.SetDefault(core.BackingMemory)
	if svc, err := azurecommon.NewACRBuildService(
		azureClients.Cred, config.SubscriptionID, config.ResourceGroup,
		config.ACRName, config.BuildStorageAccount, config.BuildContainer, logger,
	); err == nil && svc != nil {
		s.images.BuildService = svc
	}
	s.SetSelf(s)
	s.CloudState = &acaCloudState{server: s}
	// Network-discovery driver. Selected via Config.NetworkDiscovery
	// (env: SOCKERLESS_ACA_NETWORK_DISCOVERY). Validated to one of
	// cloud-dns / host-aliases / nat-gateway-only by Config.Validate.
	switch config.NetworkDiscovery {
	case api.NetworkDiscoveryCloudDNS:
		s.NetworkDiscovery = azurecommon.NewPrivateDNSDiscovery(azurecommon.PrivateDNSDiscoveryConfig{
			PrivateDNSRecords: azureClients.PrivateDNSRecords,
			ContainerApps:     azureClients.ContainerApps,
			ResourceGroup:     config.ResourceGroup,
			Logger:            logger,
			LookupNetwork: func(ctx context.Context, networkID string) (azurecommon.PrivateDNSNetworkState, bool) {
				state, ok := s.resolveNetworkState(ctx, networkID)
				if !ok {
					return azurecommon.PrivateDNSNetworkState{}, false
				}
				return azurecommon.PrivateDNSNetworkState{DNSZoneName: state.DNSZoneName}, true
			},
			GetNetwork: func(networkID string) (azurecommon.PrivateDNSNetworkState, bool) {
				state, ok := s.NetworkState.Get(networkID)
				if !ok {
					return azurecommon.PrivateDNSNetworkState{}, false
				}
				return azurecommon.PrivateDNSNetworkState{DNSZoneName: state.DNSZoneName}, true
			},
		})
		s.DNS = &azurecommon.PrivateDNSZoneDNS{
			LookupZoneName: func(ctx context.Context, networkID string) (string, error) {
				state, ok := s.resolveNetworkState(ctx, networkID)
				if !ok {
					return "", nil
				}
				return state.DNSZoneName, nil
			},
		}
	case api.NetworkDiscoveryHostAliases:
		s.NetworkDiscovery = core.NewHostAliasesDiscovery()
	case api.NetworkDiscoveryNATGatewayOnly:
		s.NetworkDiscovery = core.NoOpNetworkDiscovery{}
	}
	// Access driver. Selected via Config.Access (env: SOCKERLESS_ACA_ACCESS).
	// none-internal (default) leaves ingress auth to the network layer
	// (managed environment isolation). azure-ad signs each invoke with
	// an OAuth2 bearer token via DefaultAzureCredential — paired with
	// an Easy Auth (AAD provider) on the ACA app at deploy time.
	switch config.Access {
	case api.AccessMechanismNoneInternal:
		s.Access = core.NoneInternalAccess{}
	case api.AccessMechanismAzureAD:
		s.Access = azurecommon.NewAzureADAccess(azureClients.Cred, config.AccessPrincipal)
	}

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
