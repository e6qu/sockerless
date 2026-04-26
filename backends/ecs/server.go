package ecs

import (
	"context"
	"strings"
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
		Architecture:    "amd64",
		NCPU:            2,
		MemTotal:        4294967296,
	}, logger)
	s.volumeState = volumeState{efs: awscommon.NewEFSManager(awsClients.EFS, awscommon.EFSManagerConfig{
		AgentEFSID:     config.AgentEFSID,
		Subnets:        config.Subnets,
		SecurityGroups: config.SecurityGroups,
		PollInterval:   config.PollInterval,
		InstanceID:     s.Desc.InstanceID,
	})}
	s.images = &core.ImageManager{
		Base:   s.BaseServer,
		Auth:   awscommon.NewECRAuthProvider(awsClients.ECR, logger, s.ctx),
		Logger: logger,
	}
	if svc := awscommon.NewCodeBuildService(
		awsClients.CodeBuild, awsClients.S3,
		config.CodeBuildProject, config.BuildBucket, "", config.Region, logger,
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

	// Cloud-native typed Logs + Attach driving CloudWatch via the per-
	// container fetcher factory. Bypasses the legacy s.self.ContainerLogs
	// /ContainerAttach round-trip.
	logFactory := func(containerID string) core.CloudLogFetchFunc {
		return s.buildCloudWatchFetcher(containerID)
	}
	s.Typed.Logs = core.NewCloudLogsLogsDriver(s.BaseServer, logFactory,
		core.StreamCloudLogsOptions{},
		"ecs", "CloudWatchLogs")
	s.Typed.Attach = core.NewCloudLogsAttachDriver(s.BaseServer, logFactory,
		"ecs", "CloudLogsReadOnlyAttach")

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
