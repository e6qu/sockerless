package cloudrun

import (
	"fmt"
	"os"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
	gcpcommon "github.com/sockerless/gcp-common"
)

// Config holds Cloud Run backend configuration.
type Config struct {
	Project       string
	Region        string
	VPCConnector  string
	LogID         string
	BuildBucket   string // GCS bucket for Cloud Build context upload
	BuildPlatform string // Docker build platform for overlay images
	EndpointURL   string // Custom endpoint URL (sim/emulator) for run.googleapis.com peers
	// ARRegistryEndpoint is the Artifact Registry endpoint coordinate. Empty
	// on real Google Cloud, where the registry is `<region>-docker.pkg.dev`;
	// an operator relocates it — a harness points it at the simulator's
	// `/v2/` address — and the overlay build→push, the workload→pull
	// reference, the tag probe, and every registry request then target that
	// coordinate. Its host is what image references carry; its URL (https://
	// when it names a bare host) is what registry HTTP is dialed at. Set via
	// `SOCKERLESS_GCP_AR_ENDPOINT`.
	ARRegistryEndpoint string
	// LogAdminEndpoint is the host:port for Cloud Logging admin gRPC.
	// In real GCP this is logging.googleapis.com:443; in sim mode this
	// points at the simulator's gRPC listener. Required whenever
	// EndpointURL is set (no derivation from EndpointURL — they are
	// distinct APIs in real GCP).
	LogAdminEndpoint string
	PollInterval     time.Duration // Cloud API poll interval (default 2s)
	LogTimeout       time.Duration // Cloud Logging query timeout (default 30s)

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

	// GCSWorkloadEndpoint is the storage endpoint the in-container
	// bootstrap uses for gcs-sync workspace restore/save, injected as the
	// standard `STORAGE_EMULATOR_HOST` on the workload. A coordinate: empty
	// on real Cloud Run (the bootstrap uses real storage.googleapis.com +
	// ADC), set by a sim harness to a workload-reachable sim storage
	// address (the backend's own in-container endpoint is NOT reachable
	// from the workload — it reaches the sim through the same
	// host-gateway/published-port path as the reverse-agent callback).
	// Set via `SOCKERLESS_GCS_WORKLOAD_ENDPOINT`.
	GCSWorkloadEndpoint string

	// EnableCommit opts into the agent-driven `docker commit` path.
	// See backends/core.CommitContainerViaAgent. Off by default — the
	// resulting image wraps the whole rootfs as a single layer.
	// Set via `SOCKERLESS_ENABLE_COMMIT=1`.
	EnableCommit bool

	// SharedVolumes are the Cloud Storage buckets the runner Cloud Run Job
	// already mounts. When the caller (e.g. github-actions-runner) does
	// `docker create -v /tmp/runner-work:/__w alpine`, sockerless
	// translates the host bind mount into a named-volume reference whose
	// bucket is shared with the runner task. Format:
	// gcpcommon.SharedVolumeFormat, read from SOCKERLESS_GCP_SHARED_VOLUMES.
	SharedVolumes core.SharedVolumes

	// sharedVolumesErr carries a SOCKERLESS_GCP_SHARED_VOLUMES parse
	// failure from ConfigFromEnv to Validate so misconfiguration fails
	// startup loudly instead of silently dropping entries.
	sharedVolumesErr error

	// BootstrapBinaryPath is the on-disk path of the
	// sockerless-cloudrun-bootstrap binary. Required for the overlay
	// path: when set, ContainerCreate stages the bootstrap into every
	// per-image overlay built by Cloud Build so the resulting Cloud
	// Run Service hosts an HTTP endpoint that the backend's
	// ContainerExec POSTs envelope payloads against (Path B model —
	// specs/CLOUD_RESOURCE_MAPPING.md § Lesson 8). Empty ⇒ overlay path
	// disabled, ContainerCreate stays on the legacy Job path.
	// Set via `SOCKERLESS_CLOUDRUN_BOOTSTRAP=/opt/sockerless/sockerless-cloudrun-bootstrap`.
	BootstrapBinaryPath string

	// BootstrapBinaryHash is the SHA-256-prefix hash of the bootstrap
	// binary at BootstrapBinaryPath. Computed once at server startup
	// (NewServer hashes via core.HashBootstrapBinary) and stamped
	// into every OverlayImageSpec.BootstrapBinaryHash so updating the
	// bootstrap binary on disk invalidates cached overlay images
	// automatically. Without this, OverlayContentTag is computed only
	// from BaseImageRef + BootstrapBinaryPath — both stable across
	// bootstrap-only changes — so the AR cache would hit forever and
	// fresh containers would keep running stale bootstrap code.
	BootstrapBinaryHash string

	// ServiceAccount is the GCP service-account email the deployed
	// Cloud Run Service / Job runs as. Empty ⇒ Cloud Run's default
	// runtime service account. Operators set this when they need a
	// non-default principal for workload IAM bindings.
	// Set via `SOCKERLESS_CLOUDRUN_SERVICE_ACCOUNT`.
	ServiceAccount string

	// NetworkDiscovery selects the per-backend driver wired into
	// s.NetworkDiscovery. Cloudrun's native is cloud-dns; operators
	// may override to host-aliases (in-process registry, suitable when
	// peers share a single backend instance) or nat-gateway-only
	// (no peer discovery). Set via SOCKERLESS_GCR_NETWORK_DISCOVERY.
	NetworkDiscovery api.NetworkDiscoveryKind
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	sharedVolumes, sharedVolumesErr := core.ParseSharedVolumes(os.Getenv("SOCKERLESS_GCP_SHARED_VOLUMES"), gcpcommon.SharedVolumeFormat)
	return Config{
		Project:             os.Getenv("SOCKERLESS_GCR_PROJECT"),
		Region:              core.EnvOrDefault("SOCKERLESS_GCR_REGION", "us-central1"),
		VPCConnector:        os.Getenv("SOCKERLESS_GCR_VPC_CONNECTOR"),
		LogID:               core.EnvOrDefault("SOCKERLESS_GCR_LOG_ID", "sockerless"),
		BuildBucket:         os.Getenv("SOCKERLESS_GCP_BUILD_BUCKET"),
		BuildPlatform:       core.EnvOrDefault("SOCKERLESS_GCP_BUILD_PLATFORM", "linux/amd64"),
		EndpointURL:         os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		ARRegistryEndpoint:  os.Getenv("SOCKERLESS_GCP_AR_ENDPOINT"),
		LogAdminEndpoint:    os.Getenv("SOCKERLESS_GCP_LOGADMIN_ENDPOINT"),
		PollInterval:        core.DurationOrDefault(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		LogTimeout:          core.DurationOrDefault(os.Getenv("SOCKERLESS_LOG_TIMEOUT"), 30*time.Second),
		UseService:          os.Getenv("SOCKERLESS_GCR_USE_SERVICE") == "1",
		CallbackURL:         os.Getenv("SOCKERLESS_CALLBACK_URL"),
		GCSWorkloadEndpoint: os.Getenv("SOCKERLESS_GCS_WORKLOAD_ENDPOINT"),
		EnableCommit:        os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		SharedVolumes:       sharedVolumes,
		sharedVolumesErr:    sharedVolumesErr,
		BootstrapBinaryPath: os.Getenv("SOCKERLESS_CLOUDRUN_BOOTSTRAP"),
		ServiceAccount:      os.Getenv("SOCKERLESS_CLOUDRUN_SERVICE_ACCOUNT"),
		NetworkDiscovery:    core.NetworkDiscoveryFromEnv("SOCKERLESS_GCR_NETWORK_DISCOVERY", api.NetworkDiscoveryCloudDNS),
	}
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:        "us-central1",
		LogID:         "sockerless",
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
		if cr := env.GCP.CloudRun; cr != nil {
			if cr.Region != "" {
				c.Region = cr.Region
			}
			c.VPCConnector = cr.VPCConnector
			if cr.LogID != "" {
				c.LogID = cr.LogID
			}
			if cr.LogTimeout != "" {
				c.LogTimeout = core.DurationOrDefault(cr.LogTimeout, c.LogTimeout)
			}
		}
	}
	c.EndpointURL = env.Common.EndpointURL
	c.ARRegistryEndpoint = os.Getenv("SOCKERLESS_GCP_AR_ENDPOINT")
	c.LogAdminEndpoint = os.Getenv("SOCKERLESS_GCP_LOGADMIN_ENDPOINT")
	c.GCSWorkloadEndpoint = os.Getenv("SOCKERLESS_GCS_WORKLOAD_ENDPOINT")
	if env.Common.PollInterval != "" {
		c.PollInterval = core.DurationOrDefault(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
		if sim.GRPCPort > 0 {
			c.LogAdminEndpoint = fmt.Sprintf("localhost:%d", sim.GRPCPort)
		}
	}
	c.NetworkDiscovery = core.NetworkDiscoveryFromEnv("SOCKERLESS_GCR_NETWORK_DISCOVERY", api.NetworkDiscoveryCloudDNS)
	c.SharedVolumes, c.sharedVolumesErr = core.ParseSharedVolumes(os.Getenv("SOCKERLESS_GCP_SHARED_VOLUMES"), gcpcommon.SharedVolumeFormat)
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
		return fmt.Errorf("SOCKERLESS_GCR_PROJECT is required")
	}
	if c.EndpointURL != "" && c.LogAdminEndpoint == "" {
		return fmt.Errorf("SOCKERLESS_GCP_LOGADMIN_ENDPOINT is required when SOCKERLESS_ENDPOINT_URL is set " +
			"(Cloud Logging is a distinct API in real GCP; the sim mirrors that — set both URLs explicitly)")
	}
	if c.UseService && c.VPCConnector == "" {
		return fmt.Errorf("SOCKERLESS_GCR_USE_SERVICE=1 requires SOCKERLESS_GCR_VPC_CONNECTOR — Services need a VPC connector for peer-reachable internal DNS")
	}
	switch c.NetworkDiscovery {
	case api.NetworkDiscoveryCloudDNS, api.NetworkDiscoveryHostAliases, api.NetworkDiscoveryNATGatewayOnly:
		// supported
	default:
		return fmt.Errorf("SOCKERLESS_GCR_NETWORK_DISCOVERY=%q not supported by cloudrun (one of cloud-dns, host-aliases, nat-gateway-only required)", c.NetworkDiscovery)
	}
	return nil
}
