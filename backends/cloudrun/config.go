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
	AgentImage   string
	AgentToken   string
	CallbackURL  string        // Backend URL for reverse agent connections
	EndpointURL  string        // Custom endpoint URL
	PollInterval time.Duration // Cloud API poll interval (default 2s)
	AgentTimeout time.Duration // Agent health check timeout (default 30s)
	LogTimeout   time.Duration // Cloud Logging query timeout (default 30s)
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Project:      os.Getenv("SOCKERLESS_GCR_PROJECT"),
		Region:       envOrDefault("SOCKERLESS_GCR_REGION", "us-central1"),
		VPCConnector: os.Getenv("SOCKERLESS_GCR_VPC_CONNECTOR"),
		LogID:        envOrDefault("SOCKERLESS_GCR_LOG_ID", "sockerless"),
		AgentImage:   envOrDefault("SOCKERLESS_GCR_AGENT_IMAGE", "sockerless/agent:latest"),
		AgentToken:   os.Getenv("SOCKERLESS_GCR_AGENT_TOKEN"),
		CallbackURL:  os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EndpointURL:  os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval: parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		AgentTimeout: parseDuration(os.Getenv("SOCKERLESS_AGENT_TIMEOUT"), 30*time.Second),
		LogTimeout:   parseDuration(os.Getenv("SOCKERLESS_LOG_TIMEOUT"), 30*time.Second),
	}
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:       "us-central1",
		LogID:        "sockerless",
		AgentImage:   "sockerless/agent:latest",
		PollInterval: 2 * time.Second,
		AgentTimeout: 30 * time.Second,
		LogTimeout:   30 * time.Second,
	}
	if env.GCP != nil {
		c.Project = env.GCP.Project
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
	if env.Common.AgentImage != "" {
		c.AgentImage = env.Common.AgentImage
	}
	c.AgentToken = env.Common.AgentToken
	c.CallbackURL = env.Common.CallbackURL
	c.EndpointURL = env.Common.EndpointURL
	if env.Common.PollInterval != "" {
		c.PollInterval = parseDuration(env.Common.PollInterval, c.PollInterval)
	}
	if env.Common.AgentTimeout != "" {
		c.AgentTimeout = parseDuration(env.Common.AgentTimeout, c.AgentTimeout)
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
