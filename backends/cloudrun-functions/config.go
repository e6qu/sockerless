package gcf

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
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

	// BootstrapBinaryHash is a hex-encoded SHA-256 prefix of the bootstrap
	// binary at BootstrapBinaryPath. Computed once at server start (see
	// gcpcommon.HashBootstrapBinary) and stamped into every OverlayImageSpec
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

	// SharedVolumes mirrors cloudrun.SharedVolumes / ecs / lambda.
	// Cloud Functions Gen2 are backed by Cloud Run Service, which
	// supports `Volume{Gcs{Bucket}}` on its template. The runner
	// inside the function does `docker create -v /tmp/runner-work:/__w`;
	// sockerless translates the host bind to a named-volume reference
	// whose GCS bucket is shared with the runner-task. Format:
	// SOCKERLESS_GCP_SHARED_VOLUMES="name=path=bucket,name2=path2=bucket2".
	SharedVolumes []SharedVolume

	// VPCConnector is the Serverless VPC Connector resource path used
	// for cross-Cloud-Run service communication. Required for the
	// network-pod path to mirror cloudrun's GREEN architecture: when
	// gitlab-runner-gcf POSTs to a per-step sockerless-svc-* over the
	// Cloud Run regional URL, Cloud Run rejects same-project requests
	// that come from outside the VPC as "external". With VpcAccess +
	// ALL_TRAFFIC, the call appears as in-VPC source and IAM-gated
	// invoke succeeds. SOCKERLESS_GCF_VPC_CONNECTOR — empty disables.
	VPCConnector string

	// NetworkDiscovery selects the per-backend driver wired into
	// s.NetworkDiscovery. GCF's native is host-aliases (multi-container
	// revisions share loopback; bootstrap writes /etc/hosts at
	// materialize time). Operators may override to nat-gateway-only
	// (no peer discovery). Cloud-DNS isn't supported until the gcf
	// NetworkState model + Cloud DNS zone wiring lands (queued under
	// 121b-finish-C/J).
	// Set via SOCKERLESS_GCF_NETWORK_DISCOVERY.
	NetworkDiscovery api.NetworkDiscoveryKind
}

// SharedVolume mirrors `cloudrun.SharedVolume`. GCS bucket backs the
// volume; Cloud Run Service ServiceV2.Template.Volumes is the runtime
// mount mechanism (Cloud Functions Gen2 builds on Cloud Run).
//
// Backing selects the storage strategy. **Required, no fallback**:
// empty Backing fails loudly at materialize/exec time per the
// no-automatic-fallbacks directive (each backing has different
// cost/scale/consistency characteristics; silent default selection
// would mask misconfiguration). Operators choose: "gcs-sync" for the
// shared workspace pattern, "gcs-fuse" for legacy tar-pack persist,
// or "emptyDir" for non-shared ephemeral.
type SharedVolume struct {
	Name          string
	ContainerPath string
	Bucket        string
	Backing       string // REQUIRED: "gcs-sync" / "gcs-fuse" / "emptyDir"
}

// AsRef returns the cloud-agnostic SharedVolumeRef the storage backing
// driver consumes. Empty Backing flows through unchanged so the
// registry's Resolve fails loudly on it — this is the operator's
// misconfiguration, not a place to silently default.
func (v SharedVolume) AsRef() core.SharedVolumeRef {
	return core.SharedVolumeRef{
		Name:          v.Name,
		ContainerPath: v.ContainerPath,
		Backing:       core.StorageBacking(v.Backing),
		GCSBucket:     v.Bucket,
	}
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
		PoolMax:          envOrDefaultInt("SOCKERLESS_GCF_POOL_MAX", 10),
		PrewarmOverlays:  parsePrewarmOverlays(os.Getenv("SOCKERLESS_GCF_PREWARM_OVERLAYS")),
		SharedVolumes:    parseSharedVolumes(os.Getenv("SOCKERLESS_GCP_SHARED_VOLUMES")),
		VPCConnector:     os.Getenv("SOCKERLESS_GCF_VPC_CONNECTOR"),
		NetworkDiscovery: networkDiscoveryFromEnv("SOCKERLESS_GCF_NETWORK_DISCOVERY", api.NetworkDiscoveryHostAliases),
	}
}

// networkDiscoveryFromEnv reads the operator's chosen kind from env or
// returns `def`. Validation against the per-backend supported set
// happens in Config.Validate.
func networkDiscoveryFromEnv(envVar string, def api.NetworkDiscoveryKind) api.NetworkDiscoveryKind {
	v := strings.TrimSpace(os.Getenv(envVar))
	if v == "" {
		return def
	}
	return api.NetworkDiscoveryKind(v)
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

// parseSharedVolumes parses SOCKERLESS_GCP_SHARED_VOLUMES.
//
// Format: `name=path=bucket=backing,name=path=bucket=backing,...`
// where `backing` is one of `gcs-sync`, `gcs-fuse`, or `emptyDir` (REQUIRED
// per the no-fallbacks directive). Returns nil for empty input. Malformed
// entries (wrong arity, empty fields) are skipped — operators see them
// missing at materialize time and the failure-loud Resolve() call surfaces
// the misconfiguration with a clear error.
//
// Backwards-compat for the legacy 3-tuple format (`name=path=bucket`,
// no backing) is INTENTIONALLY removed: every consumer must explicitly
// declare its storage strategy.
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
		if len(parts) != 4 {
			// Entry malformed — skip silently here; the volume's
			// absence at materialize time surfaces as a clearer error
			// than a parse error here would.
			continue
		}
		sv := SharedVolume{
			Name:          strings.TrimSpace(parts[0]),
			ContainerPath: strings.TrimSpace(parts[1]),
			Bucket:        strings.TrimSpace(parts[2]),
			Backing:       strings.TrimSpace(parts[3]),
		}
		if sv.Name == "" || sv.ContainerPath == "" || sv.Bucket == "" || sv.Backing == "" {
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
	c.NetworkDiscovery = networkDiscoveryFromEnv("SOCKERLESS_GCF_NETWORK_DISCOVERY", api.NetworkDiscoveryHostAliases)
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
	switch c.NetworkDiscovery {
	case api.NetworkDiscoveryHostAliases, api.NetworkDiscoveryNATGatewayOnly:
		// supported
	default:
		return fmt.Errorf("SOCKERLESS_GCF_NETWORK_DISCOVERY=%q not supported by gcf (one of host-aliases, nat-gateway-only required; cloud-dns wiring lives in 121b-finish-J)", c.NetworkDiscovery)
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
