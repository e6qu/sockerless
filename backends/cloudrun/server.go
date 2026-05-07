package cloudrun

import (
	"context"
	"io"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
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

	// storageBackings resolves SharedVolume.Backing → driver.
	storageBackings *core.StorageBackingRegistry
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
	// networkServices maps user-defined-network ID → []serviceContainerID
	// so subsequent script-runner stages joining the same network can
	// re-bundle service containers (postgres etc.) as Cloud Run
	// multi-container sidecars in their own revisions. Without this,
	// only the FIRST script-runner stage would see postgres on loopback;
	// later stages (gitlab-runner v17.5 creates a new container per
	// stage) would deploy without the sidecar and lose service access.
	networkServices sync.Map
}

// NewServer creates a new Cloud Run backend server.
func NewServer(config Config, gcpClients *GCPClients, logger zerolog.Logger) *Server {
	// Hash the bootstrap binary at startup so OverlayContentTag changes
	// whenever the binary changes. Mirror of gcf's path; without this,
	// the AR overlay cache hits forever and updates to the bootstrap
	// (e.g. SOCKERLESS_SYNC_MOUNTS support) never reach the JOB pod-Service.
	if config.BootstrapBinaryHash == "" && config.BootstrapBinaryPath != "" {
		if hash, err := gcpcommon.HashBootstrapBinary(config.BootstrapBinaryPath); err == nil {
			config.BootstrapBinaryHash = hash
			logger.Info().Str("path", config.BootstrapBinaryPath).Str("hash", hash).Msg("hashed cloudrun bootstrap binary for overlay-tag invalidation")
		} else {
			logger.Warn().Err(err).Str("path", config.BootstrapBinaryPath).Msg("failed to hash bootstrap binary — overlay images will not invalidate on bootstrap update")
		}
	}
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

	// Storage backing registry. EmptyDirDriver always available;
	// GCSFuseDriver kept for legacy SharedVolumes (tar-pack persist);
	// GCSSyncDriver registered when the GCS client constructs
	// successfully. No-fallbacks directive: SharedVolumes with an
	// unrecognized Backing fail at resolve time rather than silently
	// selecting a default.
	s.storageBackings = core.NewStorageBackingRegistry()
	s.storageBackings.Register(&gcpcommon.GCSFuseDriver{
		MountOptions: gcpcommon.RunnerWorkspaceMountOptions(),
	})
	if syncDriver, err := gcpcommon.NewGCSSyncDriver(context.Background()); err == nil {
		s.storageBackings.Register(syncDriver)
		logger.Info().Str("backing", "gcs-sync").Msg("registered storage backing driver")
	} else {
		logger.Warn().Err(err).Msg("gcs-sync driver init failed — operators using `gcs-sync` Backing will see resolve errors")
	}

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
	// Typed.Exec wiring: route through s.ExecStart (the cloudrun
	// override) rather than the reverse-agent driver directly. The
	// override's `execStartViaInvoke` POSTs an envelope to the
	// materialized pod-Service URL — required for the GH actions/runner
	// pattern where the bootstrap can't dial back to register a
	// reverse-agent (the runner-task is a Cloud Run Job without a public
	// URL). Reverse-agent stays as a fallback inside s.ExecStart for
	// interactive (TTY+stdin) execs.
	s.Typed.Exec = core.WrapLegacyExecStart(
		func(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
			return s.ExecStart(id, opts)
		},
		s.Store,
		s.Desc.Driver, "cloudrun-self-dispatch",
	)
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
	// Wire ContainerAttach via the legacy adapter so opts.Stdin attaches
	// route through cloudrun.ContainerAttach (which sets up the stdin
	// pipe + hijacked attach stream for overlay containers). Read-only
	// attaches (no Stdin) fall through to AttachViaCloudLogs inside
	// cloudrun.ContainerAttach.
	s.Typed.Attach = core.WrapLegacyContainerAttach(s.ContainerAttach,
		"cloudrun", "ContainerAttach")

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
