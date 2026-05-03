package cloudrun

import (
	"fmt"
	"os"
	"strings"
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

	// SharedVolumes mirrors the ECS / Lambda backends' same-named
	// field. When sockerless-backend-cloudrun runs inside a Cloud Run
	// Job that has GCS volumes mounted at known paths, and the
	// caller (e.g. github-actions-runner) does
	// `docker create -v /tmp/runner-work:/__w alpine`, sockerless
	// translates the host bind mount into a named-volume reference
	// whose GCS bucket is shared with the runner-task. Sub-tasks
	// (spawned as further Cloud Run Jobs) mount the same bucket.
	// Format: SOCKERLESS_GCP_SHARED_VOLUMES="name=path=bucket,name2=path2=bucket2"
	SharedVolumes []SharedVolume

	// BootstrapBinaryPath is the on-disk path of the
	// sockerless-cloudrun-bootstrap binary. Required for the Phase 122g
	// overlay path: when set, ContainerCreate stages the bootstrap into
	// every per-image overlay built by Cloud Build so the resulting
	// Cloud Run Service hosts an HTTP endpoint that the backend's
	// ContainerExec POSTs envelope payloads against (Path B model —
	// specs/CLOUD_RESOURCE_MAPPING.md § Lesson 8). Empty ⇒ overlay path
	// disabled, ContainerCreate stays on the legacy Job path.
	// Set via `SOCKERLESS_CLOUDRUN_BOOTSTRAP=/opt/sockerless/sockerless-cloudrun-bootstrap`.
	BootstrapBinaryPath string
}

// SharedVolume describes a workspace volume mounted via GCS that the
// caller (the runner Cloud Run Job spawned by github-runner-dispatcher-gcp)
// shares with sockerless. When `docker create` sees a bind mount whose
// source matches ContainerPath, the bind is rewritten to a named volume
// named Name backed by the GCS bucket Bucket. Mirror of `ecs.SharedVolume`
// + `lambda.SharedVolume`, but using GCS buckets as the volume backing
// (Cloud Run Jobs natively support `Volume{Gcs{Bucket}}`).
type SharedVolume struct {
	Name          string // logical volume name used in spawned sub-tasks
	ContainerPath string // path inside the calling container (= the bind-mount source)
	Bucket        string // GCS bucket backing this volume (no `gs://` prefix)
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Project:             os.Getenv("SOCKERLESS_GCR_PROJECT"),
		Region:              envOrDefault("SOCKERLESS_GCR_REGION", "us-central1"),
		VPCConnector:        os.Getenv("SOCKERLESS_GCR_VPC_CONNECTOR"),
		LogID:               envOrDefault("SOCKERLESS_GCR_LOG_ID", "sockerless"),
		BuildBucket:         os.Getenv("SOCKERLESS_GCP_BUILD_BUCKET"),
		EndpointURL:         os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:        parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		LogTimeout:          parseDuration(os.Getenv("SOCKERLESS_LOG_TIMEOUT"), 30*time.Second),
		UseService:          os.Getenv("SOCKERLESS_GCR_USE_SERVICE") == "1",
		CallbackURL:         os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EnableCommit:        os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		SharedVolumes:       parseSharedVolumes(os.Getenv("SOCKERLESS_GCP_SHARED_VOLUMES")),
		BootstrapBinaryPath: os.Getenv("SOCKERLESS_CLOUDRUN_BOOTSTRAP"),
	}
}

// parseSharedVolumes parses SOCKERLESS_GCP_SHARED_VOLUMES
// (`name=path=bucket,name2=path2=bucket2`) into SharedVolume entries.
// Returns nil for empty input.
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
