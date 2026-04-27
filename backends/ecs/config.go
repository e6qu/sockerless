package ecs

import (
	"fmt"
	"os"
	"strings"
	"time"

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
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Region:           envOrDefault("AWS_REGION", "us-east-1"),
		Cluster:          envOrDefault("SOCKERLESS_ECS_CLUSTER", "sockerless"),
		Subnets:          splitCSV(os.Getenv("SOCKERLESS_ECS_SUBNETS")),
		SecurityGroups:   splitCSV(os.Getenv("SOCKERLESS_ECS_SECURITY_GROUPS")),
		TaskRoleARN:      os.Getenv("SOCKERLESS_ECS_TASK_ROLE_ARN"),
		ExecutionRoleARN: os.Getenv("SOCKERLESS_ECS_EXECUTION_ROLE_ARN"),
		LogGroup:         envOrDefault("SOCKERLESS_ECS_LOG_GROUP", "/sockerless"),
		AgentEFSID:       os.Getenv("SOCKERLESS_AGENT_EFS_ID"),
		AssignPublicIP:   os.Getenv("SOCKERLESS_ECS_PUBLIC_IP") == "true",
		CodeBuildProject: os.Getenv("SOCKERLESS_AWS_CODEBUILD_PROJECT"),
		BuildBucket:      os.Getenv("SOCKERLESS_AWS_BUILD_BUCKET"),
		EndpointURL:      os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		CpuArchitecture:  os.Getenv("SOCKERLESS_ECS_CPU_ARCHITECTURE"),
		PollInterval:     parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
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
		c.PollInterval = parseDuration(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
	}
	return c
}

// Validate checks required configuration.
func (c Config) Validate() error {
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
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func parseDuration(s string, def time.Duration) time.Duration {
	if s == "" {
		return def
	}
	d, err := time.ParseDuration(s)
	if err != nil {
		return def
	}
	return d
}

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
