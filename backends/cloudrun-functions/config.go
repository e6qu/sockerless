package gcf

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// Config holds Cloud Run Functions backend configuration.
type Config struct {
	Project        string
	Region         string
	ServiceAccount string
	Timeout        int
	Memory         string
	CPU            string
	BuildBucket    string // GCS bucket for Cloud Build context upload
	BuildPlatform  string // Docker build platform for overlay images
	EndpointURL    string // Custom endpoint URL (sim/emulator) for cloudfunctions.googleapis.com peers
	// LogAdminEndpoint is the host:port for Cloud Logging admin gRPC.
	// In real GCP this is logging.googleapis.com:443; in sim mode this
	// points at the simulator's gRPC listener. Required whenever
	// EndpointURL is set (distinct APIs in real GCP).
	LogAdminEndpoint string
	PollInterval     time.Duration // Cloud API poll interval (default 2s)
	LogTimeout       time.Duration // Cloud Logging query timeout (default 30s)

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

	// BootstrapBinaryHash is a hex-encoded SHA-256 prefix of the bootstrap
	// binary at BootstrapBinaryPath. Computed once at server start (see
	// core.HashBootstrapBinary) and stamped into every OverlayImageSpec
	// so updating the binary at the same path invalidates cached overlay
	// images automatically — required so in-place bootstrap upgrades
	// flow into fresh Function deployments instead of hitting cached
	// overlays forever.
	BootstrapBinaryHash string

	// PoolMax caps the number of free Functions kept warm per
	// overlay-content-hash. On `docker rm`, if free count >= PoolMax the
	// function is deleted; otherwise its `sockerless_allocation` label is
	// cleared and it returns to the reuse pool. Set 0 to disable pooling
	// (every container creates+deletes a fresh Function). See
	// specs/CLOUD_RESOURCE_MAPPING.md § Stateless image cache + Function pool.
	// SOCKERLESS_GCF_POOL_MAX, default 10.
	PoolMax int

	// PrewarmOverlays lists overlay images to materialise into hot pool
	// entries at backend startup. Each entry pre-deploys N free Functions
	// tagged with the overlay's content-hash, so the FIRST ContainerCreate
	// for that image hits a warm pool instead of paying the per-deploy
	// regional CPU quota cost. gitlab-runner cache-permission containers
	// all share one image, so a small pool covers the entire pipeline's
	// parallel concurrency.
	//
	// Format: SOCKERLESS_GCF_PREWARM_OVERLAYS="image1:size1,image2:size2"
	// Example: "registry.gitlab.com/.../gitlab-runner-helper:v17.5.0:3"
	// Empty → no prewarm; pool fills lazily via the existing release path.
	PrewarmOverlays []PrewarmOverlay

	// SharedVolumes are the Cloud Storage buckets the runner function
	// already mounts. Cloud Run Functions run on a Cloud Run service, which
	// mounts `Volume{Gcs{Bucket}}` on its template. When the runner inside
	// the function does `docker create -v /tmp/runner-work:/__w`,
	// sockerless translates the host bind to a named-volume reference
	// whose bucket is shared with the runner task. Format:
	// gcpcommon.SharedVolumeFormat, read from SOCKERLESS_GCP_SHARED_VOLUMES.
	SharedVolumes core.SharedVolumes

	// sharedVolumesErr carries a SOCKERLESS_GCP_SHARED_VOLUMES parse
	// failure from ConfigFromEnv to Validate so misconfiguration fails
	// startup loudly instead of silently dropping entries.
	sharedVolumesErr error

	// VPCConnector is the Serverless VPC Connector resource path used
	// for cross-Cloud-Run service communication. Required for the
	// network-pod path: when gitlab-runner-gcf POSTs to a per-step
	// sockerless-svc-* over the Cloud Run regional URL, Cloud Run rejects
	// same-project requests that come from outside the VPC as "external".
	// With VpcAccess + ALL_TRAFFIC, the call appears as in-VPC source and
	// IAM-gated invoke succeeds. SOCKERLESS_GCF_VPC_CONNECTOR — empty disables.
	VPCConnector string

	// NetworkDiscovery selects the per-backend driver wired into
	// s.NetworkDiscovery. GCF's native is host-aliases (multi-container
	// revisions share loopback; bootstrap writes /etc/hosts at
	// materialize time). Operators may override to nat-gateway-only
	// (no peer discovery). Cloud-DNS is not supported: the gcf backend has
	// no NetworkState model or Cloud DNS zone wiring yet.
	// Set via SOCKERLESS_GCF_NETWORK_DISCOVERY.
	NetworkDiscovery api.NetworkDiscoveryKind

	// GCSWorkloadEndpoint is the storage endpoint the in-container
	// bootstrap's gcs-sync workspace restore/save uses, injected as the
	// standard `STORAGE_EMULATOR_HOST` on the workload (main container). A
	// coordinate: empty ⇒ real GCS + ADC; a sim harness sets it to a
	// workload-reachable sim storage address (the in-backend
	// SOCKERLESS_ENDPOINT_URL is not reachable from inside the workload).
	// Set via `SOCKERLESS_GCS_WORKLOAD_ENDPOINT`.
	GCSWorkloadEndpoint string
}

// PrewarmOverlay describes one entry in SOCKERLESS_GCF_PREWARM_OVERLAYS:
// the user-image to wrap with an overlay + the number of free Functions
// to keep warm in the pool for that overlay's content-hash.
type PrewarmOverlay struct {
	Image string
	Size  int
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	sharedVolumes, sharedVolumesErr := core.ParseSharedVolumes(os.Getenv("SOCKERLESS_GCP_SHARED_VOLUMES"), gcpcommon.SharedVolumeFormat)
	return Config{
		Project:          os.Getenv("SOCKERLESS_GCF_PROJECT"),
		Region:           core.EnvOrDefault("SOCKERLESS_GCF_REGION", "us-central1"),
		ServiceAccount:   os.Getenv("SOCKERLESS_GCF_SERVICE_ACCOUNT"),
		Timeout:          core.EnvOrDefaultInt("SOCKERLESS_GCF_TIMEOUT", 3600),
		Memory:           core.EnvOrDefault("SOCKERLESS_GCF_MEMORY", "4Gi"),
		CPU:              core.EnvOrDefault("SOCKERLESS_GCF_CPU", "1"),
		BuildBucket:      os.Getenv("SOCKERLESS_GCP_BUILD_BUCKET"),
		BuildPlatform:    core.EnvOrDefault("SOCKERLESS_GCP_BUILD_PLATFORM", "linux/amd64"),
		EndpointURL:      os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		LogAdminEndpoint: os.Getenv("SOCKERLESS_GCP_LOGADMIN_ENDPOINT"),
		PollInterval:     core.DurationOrDefault(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		LogTimeout:       core.DurationOrDefault(os.Getenv("SOCKERLESS_LOG_TIMEOUT"), 30*time.Second),
		CallbackURL:      os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EnableCommit:     os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		BootstrapBinaryPath: core.EnvOrDefault(
			"SOCKERLESS_GCF_BOOTSTRAP",
			"/opt/sockerless/sockerless-gcf-bootstrap",
		),
		PoolMax:             core.EnvOrDefaultInt("SOCKERLESS_GCF_POOL_MAX", 10),
		PrewarmOverlays:     parsePrewarmOverlays(os.Getenv("SOCKERLESS_GCF_PREWARM_OVERLAYS")),
		SharedVolumes:       sharedVolumes,
		sharedVolumesErr:    sharedVolumesErr,
		VPCConnector:        os.Getenv("SOCKERLESS_GCF_VPC_CONNECTOR"),
		NetworkDiscovery:    core.NetworkDiscoveryFromEnv("SOCKERLESS_GCF_NETWORK_DISCOVERY", api.NetworkDiscoveryHostAliases),
		GCSWorkloadEndpoint: os.Getenv("SOCKERLESS_GCS_WORKLOAD_ENDPOINT"),
	}
}

// parsePrewarmOverlays parses SOCKERLESS_GCF_PREWARM_OVERLAYS. Format:
// `image:size,image:size,...`. Image references that themselves contain
// colons (e.g. `host:port/repo:tag`) split on the LAST colon so the
// `:size` suffix is unambiguous. Malformed entries (non-positive size,
// missing image) are dropped with a stderr warning so misconfiguration
// fails loud instead of silently disabling prewarm.
func parsePrewarmOverlays(s string) []PrewarmOverlay {
	s = strings.TrimSpace(s)
	if s == "" {
		return nil
	}
	var out []PrewarmOverlay
	for _, raw := range strings.Split(s, ",") {
		entry := strings.TrimSpace(raw)
		if entry == "" {
			continue
		}
		idx := strings.LastIndex(entry, ":")
		if idx <= 0 || idx == len(entry)-1 {
			fmt.Fprintf(os.Stderr, "sockerless-gcf: SOCKERLESS_GCF_PREWARM_OVERLAYS entry %q malformed (want image:size)\n", entry)
			continue
		}
		image := strings.TrimSpace(entry[:idx])
		sizeStr := strings.TrimSpace(entry[idx+1:])
		size, err := strconv.Atoi(sizeStr)
		if err != nil || size <= 0 {
			fmt.Fprintf(os.Stderr, "sockerless-gcf: SOCKERLESS_GCF_PREWARM_OVERLAYS entry %q has invalid size %q\n", entry, sizeStr)
			continue
		}
		if image == "" {
			fmt.Fprintf(os.Stderr, "sockerless-gcf: SOCKERLESS_GCF_PREWARM_OVERLAYS entry %q has empty image\n", entry)
			continue
		}
		out = append(out, PrewarmOverlay{Image: image, Size: size})
	}
	return out
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:        "us-central1",
		Timeout:       3600,
		Memory:        "4Gi",
		CPU:           "1",
		BuildPlatform: "linux/amd64",
		PollInterval:  2 * time.Second,
		LogTimeout:    30 * time.Second,
	}
	if env.GCP != nil {
		c.Project = env.GCP.Project
		c.BuildBucket = env.GCP.BuildBucket
		if env.GCP.BuildPlatform != "" {
			c.BuildPlatform = env.GCP.BuildPlatform
		}
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
				c.LogTimeout = core.DurationOrDefault(gcf.LogTimeout, c.LogTimeout)
			}
		}
	}
	c.EndpointURL = env.Common.EndpointURL
	c.LogAdminEndpoint = os.Getenv("SOCKERLESS_GCP_LOGADMIN_ENDPOINT")
	if env.Common.PollInterval != "" {
		c.PollInterval = core.DurationOrDefault(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
		if sim.GRPCPort > 0 {
			c.LogAdminEndpoint = fmt.Sprintf("localhost:%d", sim.GRPCPort)
		}
	}
	c.NetworkDiscovery = core.NetworkDiscoveryFromEnv("SOCKERLESS_GCF_NETWORK_DISCOVERY", api.NetworkDiscoveryHostAliases)
	c.SharedVolumes, c.sharedVolumesErr = core.ParseSharedVolumes(os.Getenv("SOCKERLESS_GCP_SHARED_VOLUMES"), gcpcommon.SharedVolumeFormat)
	c.GCSWorkloadEndpoint = os.Getenv("SOCKERLESS_GCS_WORKLOAD_ENDPOINT")
	return c
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if err := core.ValidateDurationEnvs("SOCKERLESS_POLL_INTERVAL", "SOCKERLESS_LOG_TIMEOUT"); err != nil {
		return err
	}
	if err := core.ValidateJobTimeoutEnv(); err != nil {
		return err
	}
	if c.sharedVolumesErr != nil {
		return fmt.Errorf("SOCKERLESS_GCP_SHARED_VOLUMES: %w", c.sharedVolumesErr)
	}
	if c.Project == "" {
		return fmt.Errorf("SOCKERLESS_GCF_PROJECT is required")
	}
	if c.BuildBucket == "" {
		return fmt.Errorf("SOCKERLESS_GCP_BUILD_BUCKET is required (Cloud Functions Gen2 source archive lands here)")
	}
	if c.EndpointURL != "" && c.LogAdminEndpoint == "" {
		return fmt.Errorf("SOCKERLESS_GCP_LOGADMIN_ENDPOINT is required when SOCKERLESS_ENDPOINT_URL is set " +
			"(Cloud Logging is a distinct API in real GCP; set both URLs explicitly)")
	}
	switch c.NetworkDiscovery {
	case api.NetworkDiscoveryHostAliases, api.NetworkDiscoveryNATGatewayOnly:
		// supported
	default:
		return fmt.Errorf("SOCKERLESS_GCF_NETWORK_DISCOVERY=%q not supported by gcf (one of host-aliases, nat-gateway-only required; the gcf backend has no Cloud DNS zone wiring)", c.NetworkDiscovery)
	}
	return nil
}
