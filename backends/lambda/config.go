package lambda

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sockerless/api"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// Config holds Lambda backend configuration.
type Config struct {
	Region           string
	RoleARN          string
	LogGroup         string
	MemorySize       int
	Timeout          int
	SubnetIDs        []string
	SecurityGroupIDs []string
	// AgentEFSID (optional) lets operators reuse an existing EFS filesystem
	// for Lambda volumes instead of sockerless provisioning a fresh one.
	AgentEFSID       string
	CodeBuildProject string        // AWS CodeBuild project for docker build
	BuildBucket      string        // S3 bucket for build context upload
	EndpointURL      string        // Custom endpoint URL
	PollInterval     time.Duration // Cloud API poll interval (default 2s)
	CallbackURL      string        // Reverse-agent callback URL injected into Lambda functions; must be reachable from Lambda (public or VPC endpoint). Empty => exec unsupported.

	// Overlay image build. Used when CallbackURL is set, to layer the
	// agent + bootstrap binaries on top of the user's requested image
	// so `docker exec` can reach a running invocation. Paths are
	// resolved against the running backend's binary environment
	// (typically a container image that bundles the binaries alongside
	// the backend).
	AgentBinaryPath     string // path to sockerless-agent; defaults to SOCKERLESS_AGENT_BINARY or /opt/sockerless/sockerless-agent
	BootstrapBinaryPath string // path to sockerless-lambda-bootstrap; defaults to SOCKERLESS_LAMBDA_BOOTSTRAP or /opt/sockerless/sockerless-lambda-bootstrap

	// PrebuiltOverlayImage, when non-empty, bypasses the
	// BuildAndPushOverlayImage call and uses this image URI directly.
	// Used by operators who pre-bake their own overlay images (e.g.
	// cached in ECR at deploy time, or built through a CI pipeline
	// rather than at container-create time). Also used by the
	// end-to-end test to exercise the reverse-agent path without
	// requiring insecure-registry config on the docker daemon.
	PrebuiltOverlayImage string

	// OverlayECRRepo is the ECR repository sockerless pushes overlay-
	// converted user images to. Tags are content-addressed via
	// `OverlayContentTag(spec)` so identical inputs share a tag — Lambda
	// CreateFunction reuses the cached image. Format:
	// `<account>.dkr.ecr.<region>.amazonaws.com/<repo>` (no tag). When
	// empty, the lambda backend defaults to
	// `<account>.dkr.ecr.<region>.amazonaws.com/sockerless-live-lambda`.
	// Set by `SOCKERLESS_LAMBDA_OVERLAY_ECR_REPO`.
	OverlayECRRepo string

	// EnableCommit opts into the agent-driven `docker commit` path
	// (backends/core.CommitContainerViaAgent). Off by default because
	// the result isn't a traditional diff-against-base-image commit —
	// sockerless can't read the base image's rootfs from the backend
	// host, so the whole container filesystem becomes a single
	// new layer. Users who understand that tradeoff set
	// SOCKERLESS_ENABLE_COMMIT=1 and accept the larger image.
	EnableCommit bool

	// Architecture is the Lambda function architecture: "x86_64"
	// (default) or "arm64". The sockerless backend reports this value
	// (Docker-style: amd64 / arm64) via `docker info` so clients pull
	// single-arch images that actually run on the cloud workload —
	// sockerless's own host arch is irrelevant (client/server model:
	// Docker clients on any host arch report the *server* arch, and our
	// server is the cloud workload). Set via SOCKERLESS_LAMBDA_ARCHITECTURE.
	Architecture string

	// SharedVolumes are the EFS access points the runner-Lambda already
	// mounts (via FileSystemConfigs on the Lambda function). When the
	// runner inside the invocation does
	// `docker create -v /home/runner/_work:/__w alpine`, sockerless
	// translates the host bind mount into a named-volume reference whose
	// EFS access point is shared with the runner-Lambda. Format:
	// awscommon.LambdaSharedVolumeFormat, read from
	// SOCKERLESS_LAMBDA_SHARED_VOLUMES. Lambda allows one FileSystemConfig
	// per function, so volumes sharing an access point are told apart by
	// EFSSubpath.
	SharedVolumes core.SharedVolumes

	// sharedVolumesErr carries a SOCKERLESS_LAMBDA_SHARED_VOLUMES parse
	// failure from ConfigFromEnv to Validate so misconfiguration fails
	// startup loudly instead of silently dropping entries.
	sharedVolumesErr error

	// PoolMax caps the number of free Lambda functions kept warm per
	// overlay-content-hash. On `docker rm`, if free count >= PoolMax the
	// function is deleted; otherwise its `sockerless-allocation` tag is
	// cleared and it returns to the reuse pool. Set 0 to disable pooling
	// (every container creates+deletes a fresh function — preserves the
	// shape but eliminates amortization). Default 10. Set via
	// SOCKERLESS_LAMBDA_POOL_MAX. See specs/CLOUD_RESOURCE_MAPPING.md
	// § Stateless image cache + Function/Site reuse pool.
	PoolMax int

	// NetworkDiscovery selects the per-backend driver wired into
	// s.NetworkDiscovery. Lambda's native is nat-gateway-only —
	// per-invocation IPs aren't reachable from peers. Operators may
	// override to host-aliases (in-process registry) for the
	// multi-container-revision pattern, or service-mesh (AWS Cloud
	// Map) for read-only peer resolution from inside Lambda
	// invocations attached to a configured VPC. service-mesh requires
	// SOCKERLESS_LAMBDA_SUBNETS to be set (Cloud Map namespaces are
	// VPC-bound).
	// Set via SOCKERLESS_LAMBDA_NETWORK_DISCOVERY.
	NetworkDiscovery api.NetworkDiscoveryKind
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	sharedVolumes, sharedVolumesErr := core.ParseSharedVolumes(os.Getenv("SOCKERLESS_LAMBDA_SHARED_VOLUMES"), awscommon.LambdaSharedVolumeFormat)
	return Config{
		Region:               core.EnvOrDefault("AWS_REGION", "us-east-1"),
		RoleARN:              os.Getenv("SOCKERLESS_LAMBDA_ROLE_ARN"),
		LogGroup:             core.EnvOrDefault("SOCKERLESS_LAMBDA_LOG_GROUP", "/sockerless/lambda"),
		MemorySize:           core.EnvOrDefaultInt("SOCKERLESS_LAMBDA_MEMORY_SIZE", 1024),
		Timeout:              core.EnvOrDefaultInt("SOCKERLESS_LAMBDA_TIMEOUT", 900),
		SubnetIDs:            core.SplitCSV(os.Getenv("SOCKERLESS_LAMBDA_SUBNETS")),
		SecurityGroupIDs:     core.SplitCSV(os.Getenv("SOCKERLESS_LAMBDA_SECURITY_GROUPS")),
		AgentEFSID:           core.FirstNonEmpty(os.Getenv("SOCKERLESS_LAMBDA_AGENT_EFS_ID"), os.Getenv("SOCKERLESS_AGENT_EFS_ID")),
		CodeBuildProject:     core.FirstNonEmpty(os.Getenv("SOCKERLESS_LAMBDA_CODEBUILD_PROJECT"), os.Getenv("SOCKERLESS_CODEBUILD_PROJECT"), os.Getenv("SOCKERLESS_AWS_CODEBUILD_PROJECT")),
		BuildBucket:          core.FirstNonEmpty(os.Getenv("SOCKERLESS_LAMBDA_BUILD_BUCKET"), os.Getenv("SOCKERLESS_BUILD_BUCKET"), os.Getenv("SOCKERLESS_AWS_BUILD_BUCKET")),
		EndpointURL:          os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:         core.DurationOrDefault(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		CallbackURL:          os.Getenv("SOCKERLESS_CALLBACK_URL"),
		AgentBinaryPath:      core.EnvOrDefault("SOCKERLESS_AGENT_BINARY", "/opt/sockerless/sockerless-agent"),
		BootstrapBinaryPath:  core.EnvOrDefault("SOCKERLESS_LAMBDA_BOOTSTRAP", "/opt/sockerless/sockerless-lambda-bootstrap"),
		PrebuiltOverlayImage: os.Getenv("SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE"),
		OverlayECRRepo:       os.Getenv("SOCKERLESS_LAMBDA_OVERLAY_ECR_REPO"),
		EnableCommit:         os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		Architecture:         os.Getenv("SOCKERLESS_LAMBDA_ARCHITECTURE"),
		SharedVolumes:        sharedVolumes,
		sharedVolumesErr:     sharedVolumesErr,
		PoolMax:              core.EnvOrDefaultInt("SOCKERLESS_LAMBDA_POOL_MAX", 10),
		NetworkDiscovery:     core.NetworkDiscoveryFromEnv("SOCKERLESS_LAMBDA_NETWORK_DISCOVERY", api.NetworkDiscoveryNATGatewayOnly),
	}
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:       "us-east-1",
		LogGroup:     "/sockerless/lambda",
		MemorySize:   1024,
		Timeout:      900,
		PollInterval: 2 * time.Second,
	}
	if env.AWS != nil {
		if env.AWS.Region != "" {
			c.Region = env.AWS.Region
		}
		c.CodeBuildProject = env.AWS.CodeBuildProject
		c.BuildBucket = env.AWS.BuildBucket
		if l := env.AWS.Lambda; l != nil {
			c.RoleARN = l.RoleARN
			if l.LogGroup != "" {
				c.LogGroup = l.LogGroup
			}
			if l.MemorySize > 0 {
				c.MemorySize = l.MemorySize
			}
			if l.Timeout > 0 {
				c.Timeout = l.Timeout
			}
			c.SubnetIDs = l.Subnets
			c.SecurityGroupIDs = l.SecurityGroups
		}
	}
	c.EndpointURL = env.Common.EndpointURL
	if env.Common.PollInterval != "" {
		c.PollInterval = core.DurationOrDefault(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
	}
	c.NetworkDiscovery = core.NetworkDiscoveryFromEnv("SOCKERLESS_LAMBDA_NETWORK_DISCOVERY", api.NetworkDiscoveryNATGatewayOnly)
	c.SharedVolumes, c.sharedVolumesErr = core.ParseSharedVolumes(os.Getenv("SOCKERLESS_LAMBDA_SHARED_VOLUMES"), awscommon.LambdaSharedVolumeFormat)
	return c
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if err := core.ValidateDurationEnvs("SOCKERLESS_POLL_INTERVAL"); err != nil {
		return err
	}
	if err := core.ValidateJobTimeoutEnv(); err != nil {
		return err
	}
	if c.sharedVolumesErr != nil {
		return fmt.Errorf("SOCKERLESS_LAMBDA_SHARED_VOLUMES: %w", c.sharedVolumesErr)
	}
	if c.RoleARN == "" {
		return fmt.Errorf("SOCKERLESS_LAMBDA_ROLE_ARN is required")
	}
	switch strings.ToLower(c.Architecture) {
	case "x86_64", "arm64":
		// ok
	default:
		return fmt.Errorf("SOCKERLESS_LAMBDA_ARCHITECTURE must be set to x86_64 or arm64 (no default — sockerless reports the cloud workload's architecture, not its own host arch); got %q", c.Architecture)
	}
	switch c.NetworkDiscovery {
	case api.NetworkDiscoveryNATGatewayOnly, api.NetworkDiscoveryHostAliases, api.NetworkDiscoveryServiceMesh:
		// supported
	default:
		return fmt.Errorf("SOCKERLESS_LAMBDA_NETWORK_DISCOVERY=%q not supported by lambda (one of nat-gateway-only, host-aliases, service-mesh required)", c.NetworkDiscovery)
	}
	if c.NetworkDiscovery == api.NetworkDiscoveryServiceMesh && len(c.SubnetIDs) == 0 {
		return fmt.Errorf("SOCKERLESS_LAMBDA_NETWORK_DISCOVERY=service-mesh requires SOCKERLESS_LAMBDA_SUBNETS — Cloud Map private DNS namespaces are bound to a VPC, resolved from the first configured subnet")
	}
	return nil
}
