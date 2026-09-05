package ecs

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sockerless/api"
	awscommon "github.com/sockerless/aws-common"
	core "github.com/sockerless/backend-core"
)

// Config holds ECS backend configuration.
type Config struct {
	Region           string
	Cluster          string
	Subnets          []string
	SecurityGroups   []string
	TaskRoleARN      string
	ExecutionRoleARN string
	LogGroup         string
	AgentEFSID       string // EFS filesystem ID for bind mount volumes
	AssignPublicIP   bool
	CodeBuildProject string // AWS CodeBuild project for docker build
	BuildBucket      string // S3 bucket for build context upload
	EndpointURL      string // Custom endpoint URL
	// CpuArchitecture maps to ECS RuntimePlatform.CpuArchitecture.
	// Valid: "X86_64" (Fargate default) or "ARM64" (Graviton). The
	// sockerless backend reports this value (Docker-style) via
	// `docker info` so clients pull single-arch images that actually
	// run on the cloud workload — sockerless's own host architecture
	// is irrelevant (client/server: the Docker client may run on any
	// arch, what matters is the server side).
	CpuArchitecture string
	PollInterval    time.Duration // Cloud API poll interval (default 2s)

	// SharedVolumes maps host bind-mount paths the calling docker
	// client sees (in its own container's filesystem) to EFS access
	// points already mounted in the calling task at the same path.
	// When sockerless runs as a sidecar (or single-container with
	// sockerless baked in) inside an ECS task that has EFS mounts at
	// e.g. `/home/runner/_work`, the runner inside the task does
	// `docker create -v /home/runner/_work:/__w alpine`. Without this
	// config, sockerless rejects the host bind mount because Fargate
	// has no host filesystem. With it, sockerless translates the bind
	// mount to a named volume reference whose EFS access point is
	// shared with the runner-task — both the runner-task and the
	// spawned sub-task see the same workspace via EFS.
	//
	// Format: awscommon.ECSSharedVolumeFormat, read from
	// SOCKERLESS_ECS_SHARED_VOLUMES. The file system defaults to AgentEFSID.
	SharedVolumes core.SharedVolumes

	// sharedVolumesErr carries a SOCKERLESS_ECS_SHARED_VOLUMES parse
	// failure from ConfigFromEnv to Validate so misconfiguration fails
	// startup loudly instead of silently dropping entries.
	sharedVolumesErr error

	// NetworkDiscovery selects the per-backend driver wired into
	// s.NetworkDiscovery. ECS's native is service-mesh (AWS Cloud
	// Map). Operators may override to host-aliases (in-process
	// registry) or nat-gateway-only (no peer discovery).
	// Set via SOCKERLESS_ECS_NETWORK_DISCOVERY.
	NetworkDiscovery api.NetworkDiscoveryKind
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	sharedVolumes, sharedVolumesErr := core.ParseSharedVolumes(os.Getenv("SOCKERLESS_ECS_SHARED_VOLUMES"), awscommon.ECSSharedVolumeFormat)
	return Config{
		Region:           core.EnvOrDefault("AWS_REGION", "us-east-1"),
		Cluster:          core.EnvOrDefault("SOCKERLESS_ECS_CLUSTER", "sockerless"),
		Subnets:          core.SplitCSV(os.Getenv("SOCKERLESS_ECS_SUBNETS")),
		SecurityGroups:   core.SplitCSV(os.Getenv("SOCKERLESS_ECS_SECURITY_GROUPS")),
		TaskRoleARN:      os.Getenv("SOCKERLESS_ECS_TASK_ROLE_ARN"),
		ExecutionRoleARN: os.Getenv("SOCKERLESS_ECS_EXECUTION_ROLE_ARN"),
		LogGroup:         core.EnvOrDefault("SOCKERLESS_ECS_LOG_GROUP", "/sockerless"),
		AgentEFSID:       os.Getenv("SOCKERLESS_AGENT_EFS_ID"),
		AssignPublicIP:   os.Getenv("SOCKERLESS_ECS_PUBLIC_IP") == "true",
		CodeBuildProject: os.Getenv("SOCKERLESS_AWS_CODEBUILD_PROJECT"),
		BuildBucket:      os.Getenv("SOCKERLESS_AWS_BUILD_BUCKET"),
		EndpointURL:      os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		CpuArchitecture:  os.Getenv("SOCKERLESS_ECS_CPU_ARCHITECTURE"),
		PollInterval:     core.DurationOrDefault(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		SharedVolumes:    sharedVolumes,
		sharedVolumesErr: sharedVolumesErr,
		NetworkDiscovery: core.NetworkDiscoveryFromEnv("SOCKERLESS_ECS_NETWORK_DISCOVERY", api.NetworkDiscoveryServiceMesh),
	}
}

// ConfigFromEnvironment creates Config from a unified config environment.
// When sim is non-nil, EndpointURL is derived from the simulator port.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:       "us-east-1",
		Cluster:      "sockerless",
		LogGroup:     "/sockerless",
		PollInterval: 2 * time.Second,
	}
	if env.AWS != nil {
		if env.AWS.Region != "" {
			c.Region = env.AWS.Region
		}
		c.CodeBuildProject = env.AWS.CodeBuildProject
		c.BuildBucket = env.AWS.BuildBucket
		if ecs := env.AWS.ECS; ecs != nil {
			if ecs.Cluster != "" {
				c.Cluster = ecs.Cluster
			}
			c.Subnets = ecs.Subnets
			c.SecurityGroups = ecs.SecurityGroups
			c.TaskRoleARN = ecs.TaskRoleARN
			c.ExecutionRoleARN = ecs.ExecutionRoleARN
			if ecs.LogGroup != "" {
				c.LogGroup = ecs.LogGroup
			}
			c.AssignPublicIP = ecs.AssignPublicIP
			c.AgentEFSID = ecs.AgentEFSID
		}
	}
	c.EndpointURL = env.Common.EndpointURL
	if env.Common.PollInterval != "" {
		c.PollInterval = core.DurationOrDefault(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
	}
	c.NetworkDiscovery = core.NetworkDiscoveryFromEnv("SOCKERLESS_ECS_NETWORK_DISCOVERY", api.NetworkDiscoveryServiceMesh)
	c.SharedVolumes, c.sharedVolumesErr = core.ParseSharedVolumes(os.Getenv("SOCKERLESS_ECS_SHARED_VOLUMES"), awscommon.ECSSharedVolumeFormat)
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
		return fmt.Errorf("SOCKERLESS_ECS_SHARED_VOLUMES: %w", c.sharedVolumesErr)
	}
	if c.Cluster == "" {
		return fmt.Errorf("ECS cluster name is required")
	}
	if len(c.Subnets) == 0 {
		return fmt.Errorf("at least one subnet is required")
	}
	if c.ExecutionRoleARN == "" {
		return fmt.Errorf("execution role ARN is required")
	}
	switch strings.ToUpper(c.CpuArchitecture) {
	case "X86_64", "ARM64":
		// ok
	default:
		return fmt.Errorf("SOCKERLESS_ECS_CPU_ARCHITECTURE must be set to X86_64 or ARM64 (no default — sockerless reports the cloud workload's architecture, not its own host arch); got %q", c.CpuArchitecture)
	}
	switch c.NetworkDiscovery {
	case api.NetworkDiscoveryServiceMesh, api.NetworkDiscoveryHostAliases, api.NetworkDiscoveryNATGatewayOnly:
		// supported
	default:
		return fmt.Errorf("SOCKERLESS_ECS_NETWORK_DISCOVERY=%q not supported by ecs (one of service-mesh, host-aliases, nat-gateway-only required)", c.NetworkDiscovery)
	}
	return nil
}
