package cloudrun

import (
	"fmt"
	"os"
	"time"

	core "github.com/sockerless/backend-core"
)

// Config holds Cloud Run backend configuration.
type Config struct {
	Project      string
	Region       string
	VPCConnector string
	LogID        string
	BuildBucket  string        // GCS bucket for Cloud Build context upload
	EndpointURL  string        // Custom endpoint URL
	PollInterval time.Duration // Cloud API poll interval (default 2s)
	LogTimeout   time.Duration // Cloud Logging query timeout (default 30s)

	// UseService switches container execution from Cloud Run Jobs to
	// Cloud Run Services with internal ingress. Required for:
	// Jobs don't have addressable per-execution IPs, so
	// cross-container DNS via Cloud DNS A-records is fundamentally
	// broken. Services + a VPC connector give peer-reachable internal
	// IPs that can back the DNS records.
	// Default false (Jobs path) until the Services path is implemented.
	// Set via `SOCKERLESS_GCR_USE_SERVICE=1`.
	UseService bool

	// CallbackURL is the reverse-agent WebSocket URL injected into
	// container env (`SOCKERLESS_CALLBACK_URL`) so a bootstrap running
	// inside the container can dial back to the backend's
	// `/v1/cloudrun/reverse` endpoint. Enables `docker exec` /
	// `docker attach` against CR Jobs/Services once an overlay image
	// with the bootstrap binary is deployed. Empty ⇒ exec NotImpl.
	CallbackURL string

	// EnableCommit opts into the agent-driven `docker commit` path.
	// See backends/core.CommitContainerViaAgent. Off by default — the
	// resulting image wraps the whole rootfs as a single layer.
	// Set via `SOCKERLESS_ENABLE_COMMIT=1`.
	EnableCommit bool
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Project:      os.Getenv("SOCKERLESS_GCR_PROJECT"),
		Region:       envOrDefault("SOCKERLESS_GCR_REGION", "us-central1"),
		VPCConnector: os.Getenv("SOCKERLESS_GCR_VPC_CONNECTOR"),
		LogID:        envOrDefault("SOCKERLESS_GCR_LOG_ID", "sockerless"),
		BuildBucket:  os.Getenv("SOCKERLESS_GCP_BUILD_BUCKET"),
		EndpointURL:  os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval: parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		LogTimeout:   parseDuration(os.Getenv("SOCKERLESS_LOG_TIMEOUT"), 30*time.Second),
		UseService:   os.Getenv("SOCKERLESS_GCR_USE_SERVICE") == "1",
		CallbackURL:  os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EnableCommit: os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
	}
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:       "us-central1",
		LogID:        "sockerless",
		PollInterval: 2 * time.Second,
		LogTimeout:   30 * time.Second,
	}
	if env.GCP != nil {
		c.Project = env.GCP.Project
		c.BuildBucket = env.GCP.BuildBucket
		if cr := env.GCP.CloudRun; cr != nil {
			if cr.Region != "" {
				c.Region = cr.Region
			}
			c.VPCConnector = cr.VPCConnector
			if cr.LogID != "" {
				c.LogID = cr.LogID
			}
			if cr.LogTimeout != "" {
				c.LogTimeout = parseDuration(cr.LogTimeout, c.LogTimeout)
			}
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
	if c.Project == "" {
		return fmt.Errorf("SOCKERLESS_GCR_PROJECT is required")
	}
	if c.UseService && c.VPCConnector == "" {
		return fmt.Errorf("SOCKERLESS_GCR_USE_SERVICE=1 requires SOCKERLESS_GCR_VPC_CONNECTOR — Services need a VPC connector for peer-reachable internal DNS")
	}
	return nil
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

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
