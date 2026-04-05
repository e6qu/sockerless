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
	VolumeState  *core.StateStore[VolumeState]
	ipCounter    atomic.Int32
}

// NewServer creates a new ECS backend server.
func NewServer(config Config, awsClients *AWSClients, logger zerolog.Logger) *Server {
	s := &Server{
		config:       config,
		aws:          awsClients,
		ECS:          core.NewStateStore[ECSState](),
		NetworkState: core.NewStateStore[NetworkState](),
		VolumeState:  core.NewStateStore[VolumeState](),
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
	buildSvc := awscommon.NewCodeBuildService(
		awsClients.CodeBuild, awsClients.S3,
		config.CodeBuildProject, config.BuildBucket, "", config.Region, logger,
	)
	s.images = &core.ImageManager{
		Base:         s.BaseServer,
		Auth:         awscommon.NewECRAuthProvider(awsClients.ECR, logger, s.ctx),
		BuildService: buildSvc,
		Logger:       logger,
	}
	s.SetSelf(s)
	s.StatsProvider = &ecsStatsProvider{server: s}

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

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
