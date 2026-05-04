package gcf

import (
	"fmt"
	"os"
	"strconv"
	"strings"
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

	// SharedVolumes mirrors cloudrun.SharedVolumes / ecs / lambda.
	// Cloud Functions Gen2 are backed by Cloud Run Service, which
	// supports `Volume{Gcs{Bucket}}` on its template. The runner
	// inside the function does `docker create -v /tmp/runner-work:/__w`;
	// sockerless translates the host bind to a named-volume reference
	// whose GCS bucket is shared with the runner-task. Format:
	// SOCKERLESS_GCP_SHARED_VOLUMES="name=path=bucket,name2=path2=bucket2"
	// (BUG-909).
	SharedVolumes []SharedVolume
}

// SharedVolume mirrors `cloudrun.SharedVolume`. GCS bucket backs the
// volume; Cloud Run Service ServiceV2.Template.Volumes is the runtime
// mount mechanism (Cloud Functions Gen2 builds on Cloud Run).
type SharedVolume struct {
	Name          string
	ContainerPath string
	Bucket        string
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Project:        os.Getenv("SOCKERLESS_GCF_PROJECT"),
		Region:         envOrDefault("SOCKERLESS_GCF_REGION", "us-central1"),
		ServiceAccount: os.Getenv("SOCKERLESS_GCF_SERVICE_ACCOUNT"),
		Timeout:        envOrDefaultInt("SOCKERLESS_GCF_TIMEOUT", 3600),
		Memory:         envOrDefault("SOCKERLESS_GCF_MEMORY", "1Gi"),
		CPU:            envOrDefault("SOCKERLESS_GCF_CPU", "0.5"),
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
		PoolMax:       envOrDefaultInt("SOCKERLESS_GCF_POOL_MAX", 10),
		SharedVolumes: parseSharedVolumes(os.Getenv("SOCKERLESS_GCP_SHARED_VOLUMES")),
	}
}

// parseSharedVolumes parses SOCKERLESS_GCP_SHARED_VOLUMES
// (`name=path=bucket,...`). Returns nil for empty input.
func parseSharedVolumes(s string) []SharedVolume {
	if s == "" {
		return nil
	}
	var out []SharedVolume
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "=")
		if len(parts) != 3 {
			continue
		}
		sv := SharedVolume{
			Name:          strings.TrimSpace(parts[0]),
			ContainerPath: strings.TrimSpace(parts[1]),
			Bucket:        strings.TrimSpace(parts[2]),
		}
		if sv.Name == "" || sv.ContainerPath == "" || sv.Bucket == "" {
			continue
		}
		out = append(out, sv)
	}
	return out
}

// LookupSharedVolumeBySourcePath returns the SharedVolume entry whose
// ContainerPath equals the given path, or nil if none matches.
func (c Config) LookupSharedVolumeBySourcePath(path string) *SharedVolume {
	for i := range c.SharedVolumes {
		if c.SharedVolumes[i].ContainerPath == path {
			return &c.SharedVolumes[i]
		}
	}
	return nil
}

// LookupSharedVolumeByName returns the SharedVolume entry whose Name
// equals the given volume name, or nil if none matches.
func (c Config) LookupSharedVolumeByName(name string) *SharedVolume {
	for i := range c.SharedVolumes {
		if c.SharedVolumes[i].Name == name {
			return &c.SharedVolumes[i]
		}
	}
	return nil
}

// isSubPathOfSharedVolume reports whether path is a strict sub-path
// (descendant) of any SharedVolume's ContainerPath.
func isSubPathOfSharedVolume(path string, vols []SharedVolume) bool {
	for i := range vols {
		base := vols[i].ContainerPath
		if base == "" {
			continue
		}
		if strings.HasPrefix(path, base+"/") {
			return true
		}
	}
	return false
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:       "us-central1",
		Timeout:      3600,
		Memory:       "1Gi",
		CPU:          "0.5",
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
	if c.BuildBucket == "" {
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
