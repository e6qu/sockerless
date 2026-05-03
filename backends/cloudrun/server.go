package cloudrun

import (
	"context"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// Server is the Cloud Run backend server.
type Server struct {
	*core.BaseServer
	config    Config
	gcp       *GCPClients
	images    *core.ImageManager
	ipCounter atomic.Int32

	CloudRun     *core.StateStore[CloudRunState]
	NetworkState *core.StateStore[NetworkState]
	gcsVolumeState
	// Reverse-agent registry for docker exec / attach through a
	// bootstrap running inside the CR Job/Service container.
	reverseAgents *core.ReverseAgentRegistry
	// stdinPipes buffers stdin bytes written via the hijacked attach
	// connection (gitlab-runner / `docker run -i` pattern). Each per-
	// container pipe is read by invokeServiceDefaultCmd at deferred
	// invoke time and POSTed as the bootstrap's `execEnvelope.Stdin`.
	// Mirror of backends/ecs/server.go::stdinPipes + lambda equivalent.
	stdinPipes sync.Map
	// attachStreams maps containerID -> *attachStream so
	// invokeServiceDefaultCmd can publish the bootstrap response (mux-
	// framed stdout/stderr) back to the attached gitlab-runner. One
	// per container at a time; gitlab-runner cycles attach→start→stop
	// per stage and each new attach gets a fresh entry.
	attachStreams sync.Map
}

// NewServer creates a new Cloud Run backend server.
func NewServer(config Config, gcpClients *GCPClients, logger zerolog.Logger) *Server {
	s := &Server{
		config:         config,
		gcp:            gcpClients,
		CloudRun:       core.NewStateStore[CloudRunState](),
		NetworkState:   core.NewStateStore[NetworkState](),
		gcsVolumeState: gcsVolumeState{buckets: gcpcommon.NewBucketManager(gcpClients.Storage, config.Project, config.Region)},
	}
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
	s.images = &core.ImageManager{
		Base:   s.BaseServer,
		Auth:   gcpcommon.NewARAuthProvider(s.ctx, logger),
		Logger: logger,
	}
	if svc, err := gcpcommon.NewGCPBuildService(context.Background(), config.Project, config.BuildBucket, "", config.EndpointURL, logger); err == nil && svc != nil {
		s.images.BuildService = svc
	}
	s.SetSelf(s)
	s.CloudState = &cloudRunCloudState{server: s}

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

	// Reverse-agent registry + WS endpoint. Container-side bootstraps
	// dial `SOCKERLESS_CALLBACK_URL` → `/v1/cloudrun/reverse` and
	// register under their container ID. Without a bootstrap image in
	// use, the registry stays empty and Exec/Attach return code 126
	// (no session).
	s.reverseAgents = core.NewReverseAgentRegistry()
	s.Mux.HandleFunc("/v1/cloudrun/reverse", core.HandleReverseAgentWS(s.reverseAgents, logger))
	s.Drivers.Exec = &core.ReverseAgentExecDriver{Registry: s.reverseAgents, Logger: logger}
	s.Drivers.Stream = &core.ReverseAgentStreamDriver{Registry: s.reverseAgents, Logger: logger}
	s.Typed.Exec = core.WrapLegacyExec(s.Drivers.Exec, "cloudrun", "ReverseAgentExec")
	s.Typed.ProcList = core.NewReverseAgentProcListDriver(s.reverseAgents, "cloudrun")
	s.Typed.FSDiff = core.NewReverseAgentFSDiffDriver(s.reverseAgents, "cloudrun")
	s.Typed.FSRead = core.NewReverseAgentFSReadDriver(s.reverseAgents, "cloudrun")
	s.Typed.FSWrite = core.NewReverseAgentFSWriteDriver(s.reverseAgents, "cloudrun")
	s.Typed.FSExport = core.NewReverseAgentFSExportDriver(s.reverseAgents, "cloudrun")
	s.Typed.Commit = core.NewReverseAgentCommitDriver(s.BaseServer, s.reverseAgents, "cloudrun")

	// Cloud-native typed Logs + Attach driving Cloud Logging via the
	// per-container fetcher factory.
	logFactory := func(containerID string) core.CloudLogFetchFunc {
		return s.buildCloudLogsFetcher(containerID)
	}
	s.Typed.Logs = core.NewCloudLogsLogsDriver(s.BaseServer, logFactory,
		core.StreamCloudLogsOptions{},
		"cloudrun", "CloudLogging")
	s.Typed.Attach = core.NewCloudLogsAttachDriver(s.BaseServer, logFactory,
		"cloudrun", "CloudLogsReadOnlyAttach")

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
