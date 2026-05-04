package ecs

import (
	"context"
	"strings"
	"sync"
	"sync/atomic"

	"github.com/rs/zerolog"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// Server is the ECS backend server.
type Server struct {
	*core.BaseServer
	config       Config
	aws          *AWSClients
	images       *core.ImageManager
	ECS          *core.StateStore[ECSState]
	NetworkState *core.StateStore[NetworkState]
	ipCounter    atomic.Int32
	volumeState
	// stdinPipes buffers stdin bytes written via the hijacked attach
	// connection for containers created with OpenStdin && AttachStdin.
	// Keyed by container ID. ContainerStart drains the pipe and bakes
	// the buffered bytes into the task definition's command override
	// (Fargate has no remote stdin channel for a running task).
	stdinPipes sync.Map
}

// NewServer creates a new ECS backend server.
func NewServer(config Config, awsClients *AWSClients, logger zerolog.Logger) *Server {
	s := &Server{
		config:       config,
		aws:          awsClients,
		ECS:          core.NewStateStore[ECSState](),
		NetworkState: core.NewStateStore[NetworkState](),
	}

	s.ipCounter.Store(2)

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "ecs-backend-" + config.Cluster,
		Name:            "sockerless-ecs",
		ServerVersion:   "0.1.0",
		Driver:          "ecs-fargate",
		OperatingSystem: "AWS Fargate",
		OSType:          "linux",
		// Architecture reflects the Fargate task arch (X86_64 / ARM64),
		// translated to Docker's amd64 / arm64 spelling — sockerless's
		// own host arch is irrelevant (client/server model: clients
		// running on any host arch report the *server* arch via
		// `docker info`, and our server is the cloud workload).
		Architecture: dockerArchFromAWS(config.CpuArchitecture),
		NCPU:         2,
		MemTotal:     4294967296,
	}, logger)
	s.volumeState = volumeState{efs: awscommon.NewEFSManager(awsClients.EFS, awscommon.EFSManagerConfig{
		AgentEFSID:     config.AgentEFSID,
		Subnets:        config.Subnets,
		SecurityGroups: config.SecurityGroups,
		PollInterval:   config.PollInterval,
		InstanceID:     s.Desc.InstanceID,
	})}
	ecrAuth := awscommon.NewECRAuthProvider(awsClients.ECR, logger, s.ctx)
	s.images = &core.ImageManager{
		Base:   s.BaseServer,
		Auth:   ecrAuth,
		Logger: logger,
	}
	if svc := awscommon.NewCodeBuildService(
		awsClients.CodeBuild, awsClients.S3,
		config.CodeBuildProject, config.BuildBucket, "", config.Region, ecrAuth, logger,
	); svc != nil {
		s.images.BuildService = svc
	}
	s.SetSelf(s)
	s.StatsProvider = &ecsStatsProvider{server: s}
	s.CloudState = &ecsCloudState{
		ecs:      awsClients.ECS,
		ecr:      awsClients.ECR,
		cluster:  config.Cluster,
		region:   config.Region,
		config:   config,
		registry: s.Registry,
	}

	mode := "cloud"
	if config.EndpointURL != "" {
		mode = "custom-endpoint"
	}
	s.ProviderInfo = &core.ProviderInfo{
		Provider: "aws",
		Mode:     mode,
		Region:   config.Region,
		Endpoint: config.EndpointURL,
		Resources: map[string]string{
			"cluster":            config.Cluster,
			"subnets":            strings.Join(config.Subnets, ","),
			"execution_role_arn": config.ExecutionRoleARN,
			"log_group":          config.LogGroup,
		},
	}

	registerUI(s.BaseServer)

	// Use the metadata-only network driver. BaseServer.InitDrivers
	// installs the Linux platform driver (real `ip netns add` + veth
	// pairs) which is correct for the local docker backend (where
	// docker networks are kernel netns) but wrong for ECS, where
	// docker networks map to *cloud* networking primitives — VPC
	// security groups + Cloud Map namespaces (provisioned by
	// `cloudNetworkCreate` / `cloudNamespaceCreate` in
	// `backend_impl_network.go`). Linux kernel netns inside the
	// runner-task are irrelevant: spawned sub-tasks each get their
	// own Fargate ENI and netns from ECS itself.
	//
	// `SyntheticNetworkDriver` is the in-memory metadata store for
	// docker networks — the "synthetic" name is historical, not a
	// signal that this is fake or stub behavior. It records that the
	// docker network exists so subsequent `docker network ls` /
	// `docker network inspect` calls work; the actual cloud-side
	// networking lives in the ECS NetworkCreate wrapper that runs
	// after the BaseServer call returns.
	s.Drivers.Network = &core.SyntheticNetworkDriver{Store: s.Store, IPAlloc: s.Store.IPAlloc}

	// Cloud-native typed Logs + Attach driving CloudWatch via the per-
	// container fetcher factory. Bypasses the legacy s.self.ContainerLogs
	// /ContainerAttach round-trip.
	logFactory := func(containerID string) core.CloudLogFetchFunc {
		return s.buildCloudWatchFetcher(containerID)
	}
	s.Typed.Logs = core.NewCloudLogsLogsDriver(s.BaseServer, logFactory,
		core.StreamCloudLogsOptions{},
		"ecs", "CloudWatchLogs")
	s.Typed.Attach = &ecsStdinAttachDriver{s: s}

	// SSM-based typed drivers — bypass the api.Backend round-trip and
	// dispatch directly through ContainerXxxViaSSM helpers.
	s.Typed.ProcList = &ssmProcListDriver{s: s}
	s.Typed.FSDiff = &ssmFSDiffDriver{s: s}
	s.Typed.FSRead = &ssmFSReadDriver{s: s}
	s.Typed.FSWrite = &ssmFSWriteDriver{s: s}
	s.Typed.FSExport = &ssmFSExportDriver{s: s}
	s.Typed.Signal = &ssmSignalDriver{s: s}

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}

// dockerArchFromAWS translates AWS ECS RuntimePlatform.CpuArchitecture
// values into Docker's image-arch spelling. AWS uses X86_64 / ARM64;
// Docker / OCI use amd64 / arm64. Unknown / empty values pass through
// verbatim so misconfiguration surfaces in `docker info` instead of
// being silently coerced — Config.Validate refuses to start the
// server with an empty value, so this branch should never fire in
// production.
func dockerArchFromAWS(awsArch string) string {
	switch strings.ToUpper(awsArch) {
	case "ARM64":
		return "arm64"
	case "X86_64":
		return "amd64"
	default:
		return strings.ToLower(awsArch)
	}
}
