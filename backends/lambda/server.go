package lambda

import (
	"context"
	"fmt"
	"strings"
	"sync"
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
	// stdinPipes buffers stdin bytes written via the hijacked attach
	// connection for containers created with OpenStdin && AttachStdin.
	// ContainerStart drains the pipe and bakes the buffered bytes into
	// the Lambda Invoke Payload (the bootstrap pipes Payload to the
	// user entrypoint as stdin).
	stdinPipes sync.Map
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
		// Architecture reflects the Lambda function arch (x86_64 /
		// arm64), translated to Docker's amd64 / arm64 spelling —
		// sockerless's own host arch is irrelevant (client/server
		// model: Docker clients on any host arch report the *server*
		// arch via `docker info`, and our server is the cloud
		// workload).
		Architecture: dockerArchFromLambda(config.Architecture),
		NCPU:         2,
		MemTotal:     4294967296,
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

	// Route `docker exec` against Lambda. Two implementations exist
	// (per `specs/CLOUD_RESOURCE_MAPPING.md` § "Lambda exec semantics");
	// the deployment-time decision picks ONE — no runtime fallback:
	//
	//   - Path A (CallbackURL set): reverse-agent WebSocket. Bootstrap
	//     dials back during init, sockerless pushes TypeExec messages.
	//     Preserves Docker fidelity (multiple execs share /tmp).
	//     Requires inbound network for the dial-back.
	//   - Path B (CallbackURL empty): exec-via-Invoke. Each docker
	//     exec triggers a fresh `lambda.Invoke` whose payload is a
	//     JSON envelope; bootstrap parses, runs, returns. Native to
	//     Lambda's primitive — no inbound network needed.
	if config.CallbackURL != "" {
		s.Drivers.Exec = &lambdaExecDriver{Registry: s.reverseAgents, Logger: logger}
		s.Typed.Exec = core.WrapLegacyExec(s.Drivers.Exec, "lambda", "ReverseAgentExec")
	} else {
		invokeDriver := &lambdaInvokeExecDriver{server: s, logger: logger}
		s.Drivers.Exec = invokeDriver
		s.Typed.Exec = core.WrapLegacyExec(invokeDriver, "lambda", "InvokeExec")
	}
	s.Drivers.Stream = &lambdaStreamDriver{Registry: s.reverseAgents, Logger: logger}
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
	s.Typed.Attach = &lambdaStdinAttachDriver{s: s}

	return s
}

// ctx returns a background context.
func (s *Server) ctx() context.Context {
	return context.Background()
}

// dockerArchFromLambda translates AWS Lambda Architectures values
// (`x86_64` / `arm64`) into Docker's image-arch spelling
// (`amd64` / `arm64`). Empty or unknown values pass through verbatim;
// Config.Validate refuses to start the server with an empty or
// unrecognised value so this branch should never fire in production.
func dockerArchFromLambda(lambdaArch string) string {
	switch strings.ToLower(lambdaArch) {
	case "arm64":
		return "arm64"
	case "x86_64":
		return "amd64"
	default:
		return strings.ToLower(lambdaArch)
	}
}
