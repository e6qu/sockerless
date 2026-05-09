package gcf

import (
	"context"
	"fmt"
	"io"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// Server is the Cloud Run Functions backend server.
type Server struct {
	*core.BaseServer
	config    Config
	gcp       *GCPClients
	images    *core.ImageManager
	ipCounter atomic.Int32

	gcsVolumeState

	// storageBackings resolves SharedVolume.Backing → driver. Populated
	// at NewServer time with EmptyDirDriver + GCSFuseDriver + (when
	// available) GCSSyncDriver. The volume_translator.go helpers route
	// every materialize/exec through this registry — adding a new
	// backing means registering a driver here, no per-call-site changes.
	storageBackings *core.StorageBackingRegistry

	// Reverse-agent registry for docker top / cp / stat / diff via a
	// bootstrap running inside the function container.
	reverseAgents *core.ReverseAgentRegistry

	// deployFutures maps containerID → *deployFuture so ContainerStart can
	// wait for the asynchronous CreateFunction work that ContainerCreate
	// kicked off in a goroutine, AND can cancel that goroutine if the
	// container turns out to be a member of a network-pod that should
	// materialize as a multi-container Service revision (per
	// network_pod.go::shouldDeferOrMaterializeNetworkPod). Synchronous
	// CreateFunction.Wait + UpdateService swap blocks 150-200s and exceeds
	// gitlab-runner's 120s docker daemon timeout, so ContainerCreate
	// returns 201 immediately and the wait is deferred into ContainerStart
	// to keep the Docker contract honest. The cancellation context
	// resolves the conflict with deferred pod materialization:
	// ContainerStart calls future.Cancel() on every
	// sibling member's deploy when it decides to materialize a pod, then
	// invokes materializePodFunction; the goroutines respect ctx.Err()
	// at every cloud-API boundary and unwind (releasing any pool claim).
	deployFutures sync.Map

	// stdinPipes buffers stdin bytes written via the hijacked attach
	// connection's Write so invokePodServiceMain can replay them as the
	// bootstrap's exec envelope `Stdin` payload at deferred-invoke
	// time. Mirror of backends/cloudrun/server.go::stdinPipes — the
	// gitlab-runner attach pattern (create container with OpenStdin=true
	// + hijack-attach + start + pipe-script-and-half-close) requires
	// this because Cloud Run Service has no remote stdin channel.
	stdinPipes sync.Map

	// attachStreams maps containerID → *attachStream so the deferred
	// invoke (invokePodServiceMain) can publish the bootstrap response
	// (mux-framed stdout + stderr) back to the gitlab-runner attach
	// reader. One stream per container at a time; stage cycle clears
	// it. Mirror of backends/cloudrun/server.go::attachStreams.
	attachStreams sync.Map
}

// errDeployCancelled is the sentinel sent on a deployFuture when the
// caller (ContainerStart) cancels the in-flight async deploy because the
// container turned out to be a member of a network-pod that needs
// multi-container materialization instead. ContainerStart treats this
// as success — the materialize path will provision the right thing.
var errDeployCancelled = fmt.Errorf("deploy cancelled — container is a network-pod member, materializing as multi-container service")

// deployFuture pairs the result channel with the cancellation func that
// stops the in-flight deployFunctionAsync goroutine. Cancel + drain via
// awaitOrCancel; LoadAndDelete from s.deployFutures atomically so two
// callers can't both fire the cancel.
type deployFuture struct {
	ch     chan error
	cancel context.CancelFunc
}

// NewServer creates a new Cloud Run Functions backend server.
func NewServer(config Config, gcpClients *GCPClients, logger zerolog.Logger) *Server {
	if config.BootstrapBinaryHash == "" && config.BootstrapBinaryPath != "" {
		if hash, err := gcpcommon.HashBootstrapBinary(config.BootstrapBinaryPath); err == nil {
			config.BootstrapBinaryHash = hash
			logger.Info().Str("path", config.BootstrapBinaryPath).Str("hash", hash).Msg("hashed gcf bootstrap binary for overlay-tag invalidation")
		} else {
			logger.Warn().Err(err).Str("path", config.BootstrapBinaryPath).Msg("failed to hash gcf bootstrap binary; overlay cache will key on path only")
		}
	}
	s := &Server{
		config:         config,
		gcp:            gcpClients,
		gcsVolumeState: gcsVolumeState{buckets: gcpcommon.NewBucketManager(gcpClients.Storage, config.Project, config.Region)},
	}
	s.ipCounter.Store(2)

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
	}, logger)
	s.images = &core.ImageManager{
		Base:   s.BaseServer,
		Auth:   gcpcommon.NewARAuthProvider(s.ctx, logger),
		Logger: logger,
	}
	if svc, err := gcpcommon.NewGCPBuildService(context.Background(), config.Project, config.BuildBucket, "", config.EndpointURL, logger); err == nil && svc != nil {
		s.images.BuildService = svc
	}
	s.CloudState = &gcfCloudState{server: s}

	// Storage backing registry. EmptyDirDriver always registered;
	// GCSFuseDriver kept for legacy SharedVolumes (tar-pack persist);
	// GCSSyncDriver registered when the GCS client constructs
	// successfully — its absence is logged but not fatal so backends
	// without GCS access still boot for unit tests.
	s.storageBackings = core.NewStorageBackingRegistry()
	s.storageBackings.Register(&gcpcommon.GCSFuseDriver{
		MountOptions: gcpcommon.RunnerWorkspaceMountOptions(),
	})
	if syncDriver, err := gcpcommon.NewGCSSyncDriver(context.Background()); err == nil {
		s.storageBackings.Register(syncDriver)
		logger.Info().Str("backing", "gcs-sync").Msg("registered storage backing driver")
	} else {
		logger.Warn().Err(err).Msg("gcs-sync driver init failed — falling back to gcs-fuse for shared volumes")
	}
	s.storageBackings.Register(gcpcommon.NewPDEphemeralDriver(config.Region+"-a", 10))

	s.SetSelf(s)
	// gcf uses /etc/hosts injection within multi-container revisions
	// (SOCKERLESS_HOST_ALIASES); the host-aliases in-process driver
	// tracks peer registrations so the pod-Service materializer can
	// read them at container-create time. Cloud-DNS is not used by gcf
	// (Cloud Functions Gen2 invocations are HTTP-fronted, not
	// CNAME-discovered).
	s.NetworkDiscovery = core.NewHostAliasesDiscovery()
	s.Access = newIDTokenAccess(s)

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
			"project":         config.Project,
			"service_account": config.ServiceAccount,
		},
	}

	registerUI(s.BaseServer)

	// Reverse-agent registry + WS endpoint.
	s.reverseAgents = core.NewReverseAgentRegistry()
	s.Mux.HandleFunc("/v1/gcf/reverse", core.HandleReverseAgentWS(s.reverseAgents, logger))
	s.Drivers.Exec = &core.ReverseAgentExecDriver{Registry: s.reverseAgents, Logger: logger}
	s.Drivers.Stream = &core.ReverseAgentStreamDriver{Registry: s.reverseAgents, Logger: logger}
	// Typed.Exec wiring: route through s.ExecStart (the gcf override)
	// rather than the reverse-agent driver directly. The override's
	// `execStartViaInvoke` POSTs an envelope to the materialized
	// pod-Service URL — required for the GH actions/runner pattern
	// where the bootstrap can't dial back to register a reverse-agent
	// (the runner-task is a Cloud Run Job without a public URL).
	// Reverse-agent stays as a fallback inside s.ExecStart for
	// interactive (TTY+stdin) execs.
	s.Typed.Exec = core.WrapLegacyExecStart(
		func(id string, opts api.ExecStartRequest) (io.ReadWriteCloser, error) {
			return s.ExecStart(id, opts)
		},
		s.Store,
		s.Desc.Driver, "gcf-self-dispatch",
	)
	s.Typed.ProcList = core.NewReverseAgentProcListDriver(s.reverseAgents, "gcf")
	s.Typed.FSDiff = core.NewReverseAgentFSDiffDriver(s.reverseAgents, "gcf")
	s.Typed.FSRead = core.NewReverseAgentFSReadDriver(s.reverseAgents, "gcf")
	s.Typed.FSWrite = core.NewReverseAgentFSWriteDriver(s.reverseAgents, "gcf")
	s.Typed.FSExport = core.NewReverseAgentFSExportDriver(s.reverseAgents, "gcf")
	s.Typed.Commit = core.NewReverseAgentCommitDriver(s.BaseServer, s.reverseAgents, "gcf")

	// Cloud-native typed drivers for Logs + Attach. Both go through
	// Cloud Logging via a per-container fetcher factory.
	logFactory := func(containerID string) core.CloudLogFetchFunc {
		return s.buildCloudLogsFetcher(containerID)
	}
	s.Typed.Logs = core.NewCloudLogsLogsDriver(s.BaseServer, logFactory,
		core.StreamCloudLogsOptions{CheckLogBuffers: true},
		"gcf", "CloudLogging")
	// Route Typed.Attach through gcf's ContainerAttach delegate so
	// hijacked /containers/{id}/attach calls register a stdinPipe +
	// attachStream for the gitlab-runner attach pattern (mirror of
	// cloudrun's GREEN cell 7 architecture). Read-only attaches (no
	// Stdin) fall through to AttachViaCloudLogs inside the delegate.
	// Previously gcf used the read-only NewCloudLogsAttachDriver
	// which silently dropped gitlab-runner's stdin, causing the build
	// container's pipe never to be populated.
	s.Typed.Attach = core.WrapLegacyContainerAttach(s.ContainerAttach,
		"gcf", "ContainerAttach")

	// Pre-warm the function pool for any operator-configured overlays.
	// Each entry deploys N free Functions tagged with the overlay's
	// content-hash so the FIRST ContainerCreate for that image hits a
	// warm pool, bypassing the regional CPU quota cost of a fresh
	// per-container deploy. Runs in the background so NewServer doesn't
	// block on Cloud Build + N CreateFunction roundtrips at boot.
	if len(config.PrewarmOverlays) > 0 && s.images.BuildService != nil {
		go s.prewarmAllOverlays(context.Background())
	}

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
