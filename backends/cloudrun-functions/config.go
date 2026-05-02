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
	EndpointURL    string        // Custom endpoint URL
	PollInterval   time.Duration // Cloud API poll interval (default 2s)
	LogTimeout     time.Duration // Cloud Logging query timeout (default 30s)

	// CallbackURL is the reverse-agent WebSocket URL injected into
	// the function container env so a bootstrap inside can dial back
	// to the backend's /v1/gcf/reverse endpoint. Empty ⇒ exec/top
	// NotImplemented.
	CallbackURL string

	// EnableCommit opts into the agent-driven `docker commit` path.
	// See backends/core.CommitContainerViaAgent. Set via
	// `SOCKERLESS_ENABLE_COMMIT=1`.
	EnableCommit bool

	// BootstrapBinaryPath is the host filesystem path of the
	// sockerless-gcf-bootstrap binary. The backend tar-packages this
	// alongside a generated Dockerfile and submits to Cloud Build to
	// produce the per-content-hash overlay image in AR. Defaults to the
	// SOCKERLESS_GCF_BOOTSTRAP env var (or /opt/sockerless/sockerless-gcf-bootstrap).
	BootstrapBinaryPath string

	// PoolMax caps the number of free Functions kept warm per
	// overlay-content-hash. On `docker rm`, if free count >= PoolMax the
	// function is deleted; otherwise its `sockerless_allocation` label is
	// cleared and it returns to the reuse pool. Set 0 to disable pooling
	// (every container creates+deletes a fresh Function). See
	// specs/CLOUD_RESOURCE_MAPPING.md § Stateless image cache + Function pool.
	// SOCKERLESS_GCF_POOL_MAX, default 10.
	PoolMax int
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
		EndpointURL:    os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:   parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		LogTimeout:     parseDuration(os.Getenv("SOCKERLESS_LOG_TIMEOUT"), 30*time.Second),
		CallbackURL:    os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EnableCommit:   os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		BootstrapBinaryPath: envOrDefault(
			"SOCKERLESS_GCF_BOOTSTRAP",
			"/opt/sockerless/sockerless-gcf-bootstrap",
		),
		PoolMax: envOrDefaultInt("SOCKERLESS_GCF_POOL_MAX", 10),
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
		return fmt.Errorf("SOCKERLESS_GCF_PROJECT is required")
	}
	// BuildBucket is required when targeting real GCP (Cloud Functions
	// Gen2 needs a GCS source archive for the stub-Buildpacks-Go
	// CreateFunction call). The sim doesn't go through that path —
	// EndpointURL is set in sim mode, in which case we tolerate an
	// unset bucket so integration tests don't have to fabricate one.
	if c.BuildBucket == "" && c.EndpointURL == "" {
		return fmt.Errorf("SOCKERLESS_GCP_BUILD_BUCKET is required (Cloud Functions Gen2 source archive lands here)")
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
