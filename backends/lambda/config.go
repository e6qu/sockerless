package lambda

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

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
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Region:               envOrDefault("AWS_REGION", "us-east-1"),
		RoleARN:              os.Getenv("SOCKERLESS_LAMBDA_ROLE_ARN"),
		LogGroup:             envOrDefault("SOCKERLESS_LAMBDA_LOG_GROUP", "/sockerless/lambda"),
		MemorySize:           envOrDefaultInt("SOCKERLESS_LAMBDA_MEMORY_SIZE", 1024),
		Timeout:              envOrDefaultInt("SOCKERLESS_LAMBDA_TIMEOUT", 900),
		SubnetIDs:            splitCSV(os.Getenv("SOCKERLESS_LAMBDA_SUBNETS")),
		SecurityGroupIDs:     splitCSV(os.Getenv("SOCKERLESS_LAMBDA_SECURITY_GROUPS")),
		CodeBuildProject:     os.Getenv("SOCKERLESS_AWS_CODEBUILD_PROJECT"),
		BuildBucket:          os.Getenv("SOCKERLESS_AWS_BUILD_BUCKET"),
		EndpointURL:          os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:         parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		CallbackURL:          os.Getenv("SOCKERLESS_CALLBACK_URL"),
		AgentBinaryPath:      envOrDefault("SOCKERLESS_AGENT_BINARY", "/opt/sockerless/sockerless-agent"),
		BootstrapBinaryPath:  envOrDefault("SOCKERLESS_LAMBDA_BOOTSTRAP", "/opt/sockerless/sockerless-lambda-bootstrap"),
		PrebuiltOverlayImage: os.Getenv("SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE"),
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
		c.PollInterval = parseDuration(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
	}
	return c
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if c.RoleARN == "" {
		return fmt.Errorf("SOCKERLESS_LAMBDA_ROLE_ARN is required")
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}

func envOrDefaultInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
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
