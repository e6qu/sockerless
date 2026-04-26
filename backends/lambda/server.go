package lambda

import (
	"context"
	"fmt"
	"sync/atomic"

	"github.com/rs/zerolog"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// Server is the Lambda backend server.
type Server struct {
	*core.BaseServer
	config        Config
	aws           *AWSClients
	images        *core.ImageManager
	Lambda        *core.StateStore[LambdaState]
	reverseAgents *reverseAgentRegistry // reverse-agent session registry
	ipCounter     atomic.Int32
	volumeState
}

// NewServer creates a new Lambda backend server.
func NewServer(config Config, awsClients *AWSClients, logger zerolog.Logger) *Server {
	s := &Server{
		config:        config,
		aws:           awsClients,
		Lambda:        core.NewStateStore[LambdaState](),
		reverseAgents: newReverseAgentRegistry(),
	}
	s.ipCounter.Store(2)

	s.BaseServer = core.NewBaseServer(core.NewStore(), core.BackendDescriptor{
		ID:              "lambda-backend",
		Name:            "sockerless-lambda",
		ServerVersion:   "0.1.0",
		Driver:          "lambda",
		OperatingSystem: "AWS Lambda",
		OSType:          "linux",
		Architecture:    "amd64",
		NCPU:            2,
		MemTotal:        4294967296,
	}, logger)
	s.volumeState = volumeState{efs: awscommon.NewEFSManager(awsClients.EFS, awscommon.EFSManagerConfig{
		AgentEFSID:     config.AgentEFSID,
		Subnets:        config.SubnetIDs,
		SecurityGroups: config.SecurityGroupIDs,
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
	s.CloudState = &lambdaCloudState{server: s}

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
			"role_arn":    config.RoleARN,
			"memory_size": fmt.Sprintf("%d", config.MemorySize),
			"timeout":     fmt.Sprintf("%d", config.Timeout),
		},
	}

	registerUI(s.BaseServer)
	s.registerReverseAgentRoutes(logger)

	// Route `docker exec` + `docker attach` through the reverse-agent
	// when a session is connected. The BaseServer's default
	// LocalExecDriver/LocalStreamDriver error out since Lambda has no
	// local container namespace; the reverse-agent pattern fills the gap.
	s.Drivers.Exec = &lambdaExecDriver{Registry: s.reverseAgents, Logger: logger}
	s.Drivers.Stream = &lambdaStreamDriver{Registry: s.reverseAgents, Logger: logger}
	// Typed Exec driver — bypasses BaseServer.ExecStart's pipeConn
	// bridge and dispatches directly to the reverse-agent driver with
	// the hijacked conn handed in by handleExecStart.
	s.Typed.Exec = core.WrapLegacyExec(s.Drivers.Exec, "lambda", "ReverseAgentExec")
	s.Typed.ProcList = core.NewReverseAgentProcListDriver(s.reverseAgents, "lambda")
	s.Typed.FSDiff = core.NewReverseAgentFSDiffDriver(s.reverseAgents, "lambda")
	s.Typed.FSRead = core.NewReverseAgentFSReadDriver(s.reverseAgents, "lambda")
	s.Typed.FSWrite = core.NewReverseAgentFSWriteDriver(s.reverseAgents, "lambda")
	s.Typed.FSExport = core.NewReverseAgentFSExportDriver(s.reverseAgents, "lambda")
	s.Typed.Commit = core.NewReverseAgentCommitDriver(s.BaseServer, s.reverseAgents, "lambda")

	// Cloud-native typed drivers for Logs + Attach. Both go through
	// CloudWatch with a per-container log-group factory so the typed
	// dispatch sites bypass the legacy s.self.ContainerLogs /
	// ContainerAttach round-trip. The factory closure resolves the
	// log group lazily at call time because Lambda creates the
	// CloudWatch stream only after the first invocation.
	logFactory := func(containerID string) core.CloudLogFetchFunc {
		return s.buildCloudWatchFetcher(containerID)
	}
	s.Typed.Logs = core.NewCloudLogsLogsDriver(s.BaseServer, logFactory,
		core.StreamCloudLogsOptions{CheckLogBuffers: true},
		"lambda", "CloudWatchLogs")
	s.Typed.Attach = core.NewCloudLogsAttachDriver(s.BaseServer, logFactory,
		"lambda", "CloudLogsReadOnlyAttach")

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}
