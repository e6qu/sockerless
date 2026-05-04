package gcf

import (
	"context"
	"fmt"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
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
	// Reverse-agent registry for docker top / cp / stat / diff via a
	// bootstrap running inside the function container.
	reverseAgents *core.ReverseAgentRegistry

	// deployFutures maps containerID → *deployFuture so ContainerStart can
	// wait for the asynchronous CreateFunction work that ContainerCreate
	// kicked off in a goroutine, AND can cancel that goroutine if the
	// container turns out to be a member of a network-pod that should
	// materialize as a multi-container Service revision (per
	// network_pod.go::shouldDeferOrMaterializeNetworkPod). Per BUG-923:
	// synchronous CreateFunction.Wait + UpdateService swap blocks
	// 150-200s and exceeds gitlab-runner's 120s docker daemon timeout.
	// Returning 201 from ContainerCreate immediately and deferring the
	// wait into ContainerStart keeps the Docker contract honest. The
	// cancellation context resolves the conflict with BUG-925's deferred
	// pod materialization: ContainerStart calls future.Cancel() on every
	// sibling member's deploy when it decides to materialize a pod, then
	// invokes materializePodFunction; the goroutines respect ctx.Err()
	// at every cloud-API boundary and unwind (releasing any pool claim).
	deployFutures sync.Map
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
	s.Typed.Exec = core.WrapLegacyExec(s.Drivers.Exec, "gcf", "ReverseAgentExec")
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
	s.Typed.Attach = core.NewCloudLogsAttachDriver(s.BaseServer, logFactory,
		"gcf", "CloudLogsReadOnlyAttach")

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
