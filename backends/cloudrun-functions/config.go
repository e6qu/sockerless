package gcf

import (
	"fmt"
	"os"
	"strconv"
	"time"

	core "github.com/sockerless/backend-core"
)

// Config holds Cloud Run Functions backend configuration.
type Config struct {
	Project        string
	Region         string
	ServiceAccount string
	Timeout        int
	Memory         string
	CPU            string
	BuildBucket    string        // GCS bucket for Cloud Build context upload
	CallbackURL    string        // Backend URL for reverse agent connections
	EndpointURL    string        // Custom endpoint URL
	PollInterval   time.Duration // Cloud API poll interval (default 2s)
	LogTimeout     time.Duration // Cloud Logging query timeout (default 30s)
	AgentTimeout   time.Duration // Timeout waiting for agent callback (default 30s)
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Project:        os.Getenv("SOCKERLESS_GCF_PROJECT"),
		Region:         envOrDefault("SOCKERLESS_GCF_REGION", "us-central1"),
		ServiceAccount: os.Getenv("SOCKERLESS_GCF_SERVICE_ACCOUNT"),
		Timeout:        envOrDefaultInt("SOCKERLESS_GCF_TIMEOUT", 3600),
		Memory:         envOrDefault("SOCKERLESS_GCF_MEMORY", "1Gi"),
		CPU:            envOrDefault("SOCKERLESS_GCF_CPU", "1"),
		BuildBucket:    os.Getenv("SOCKERLESS_GCP_BUILD_BUCKET"),
		CallbackURL:    os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EndpointURL:    os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:   parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		LogTimeout:     parseDuration(os.Getenv("SOCKERLESS_LOG_TIMEOUT"), 30*time.Second),
		AgentTimeout:   parseDuration(os.Getenv("SOCKERLESS_AGENT_TIMEOUT"), 30*time.Second),
	}
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:       "us-central1",
		Timeout:      3600,
		Memory:       "1Gi",
		CPU:          "1",
		PollInterval: 2 * time.Second,
		LogTimeout:   30 * time.Second,
		AgentTimeout: 30 * time.Second,
	}
	if env.GCP != nil {
		c.Project = env.GCP.Project
		c.BuildBucket = env.GCP.BuildBucket
		if gcf := env.GCP.GCF; gcf != nil {
			if gcf.Region != "" {
				c.Region = gcf.Region
			}
			c.ServiceAccount = gcf.ServiceAccount
			if gcf.Timeout > 0 {
				c.Timeout = gcf.Timeout
			}
			if gcf.Memory != "" {
				c.Memory = gcf.Memory
			}
			if gcf.CPU != "" {
				c.CPU = gcf.CPU
			}
			if gcf.LogTimeout != "" {
				c.LogTimeout = parseDuration(gcf.LogTimeout, c.LogTimeout)
			}
		}
	}
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
		return fmt.Errorf("SOCKERLESS_GCF_PROJECT is required")
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

func envOrDefaultInt(key string, def int) int {
	if v := os.Getenv(key); v != "" {
		if n, err := strconv.Atoi(v); err == nil {
			return n
		}
	}
	return def
}
