package aca

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// Config holds ACA backend configuration.
type Config struct {
	SubscriptionID        string
	ResourceGroup         string
	Environment           string
	Location              string
	LogAnalyticsWorkspace string
	StorageAccount        string
	ACRName               string        // Azure Container Registry name for builds
	BuildStorageAccount   string        // Storage account for ACR build context
	BuildContainer        string        // Blob container for ACR build context
	BuildPlatform         string        // Docker build platform for overlay images
	EndpointURL           string        // Custom endpoint URL
	PollInterval          time.Duration // Cloud API poll interval (default 2s)

	// UseApp switches container execution from ACA Jobs to ACA Apps
	// with internal ingress. Required for: Jobs don't have
	// addressable per-execution IPs, so cross-container DNS
	// via Private DNS A-records is fundamentally broken. Apps with
	// `Ingress.External=false` give peer-reachable internal FQDNs.
	// Default false (Jobs path) until the Apps path is implemented.
	// Set via `SOCKERLESS_ACA_USE_APP=1`.
	UseApp bool

	// CallbackURL is the reverse-agent WebSocket URL injected into
	// container env so a bootstrap running inside the container can
	// dial back to the backend's /v1/aca/reverse endpoint. Enables
	// docker exec / attach once an overlay image with the bootstrap
	// binary is deployed.
	CallbackURL string

	// BootstrapBinaryPath is the on-disk path of the ACA-compatible
	// reverse-agent bootstrap binary. Required for the Apps path unless
	// the requested image is already a sockerless overlay image.
	// Set via SOCKERLESS_ACA_BOOTSTRAP.
	BootstrapBinaryPath string

	// BootstrapBinaryHash is the SHA-256-prefix hash of
	// BootstrapBinaryPath, computed once at server startup so overlay
	// tags invalidate when the bootstrap binary changes.
	BootstrapBinaryHash string

	// EnableCommit opts into the agent-driven `docker commit` path.
	// See backends/core.CommitContainerViaAgent. Set via
	// `SOCKERLESS_ENABLE_COMMIT=1`.
	EnableCommit bool

	// NetworkDiscovery selects the per-backend driver wired into
	// s.NetworkDiscovery. ACA's native is cloud-dns (Azure Private
	// DNS zones). Operators may override to host-aliases (in-process
	// registry) or nat-gateway-only (no peer discovery).
	// Set via SOCKERLESS_ACA_NETWORK_DISCOVERY.
	NetworkDiscovery api.NetworkDiscoveryKind

	// Access selects the ingress-auth + caller-side signer mechanism
	// wired into s.Access. ACA's native is none-internal (managed
	// environment isolates traffic at the network layer). Operators
	// may override to azure-ad to gate ACA app ingress with an Easy
	// Auth (AAD) provider; the caller-side signer mints OAuth2 bearer
	// tokens via DefaultAzureCredential.
	// Set via SOCKERLESS_ACA_ACCESS.
	Access api.AccessMechanism

	// AccessPrincipal is the workload's MSI client ID or service
	// principal AppId reported via WorkloadPrincipal. Informational —
	// the actual signing identity comes from azidentity. Empty when
	// the workload uses the platform-default identity.
	// Set via SOCKERLESS_ACA_ACCESS_PRINCIPAL.
	AccessPrincipal string

	// SharedVolumes mirrors the ECS / Lambda / Cloud Run backends'
	// same-named field. When sockerless runs inside an ACA job/app that
	// has Azure Files shares mounted at known paths, and the caller
	// (e.g. github-actions-runner) does
	// `docker create -v /home/runner/_work:/__w alpine`, sockerless
	// translates the host bind mount into a named-volume reference
	// whose Azure Files share is shared with the runner-task. Sub-tasks
	// (spawned as further ACA jobs/apps) mount the same share.
	// Format: SOCKERLESS_ACA_SHARED_VOLUMES="name=path=share=backing[=storageAccount],..."
	SharedVolumes []SharedVolume

	// sharedVolumesErr carries a SOCKERLESS_ACA_SHARED_VOLUMES parse
	// failure from ConfigFromEnv to Validate so misconfiguration fails
	// startup loudly instead of silently dropping entries.
	sharedVolumesErr error
}

// SharedVolume describes a workspace volume mounted via Azure Files
// that the caller (the runner ACA job/app) shares with sockerless.
// When `docker create` sees a bind mount whose source matches
// ContainerPath, the bind is rewritten to a named volume named Name
// backed by the Azure Files share ShareName. Mirror of
// `ecs.SharedVolume` + `cloudrun.SharedVolume`, but using Azure Files
// shares as the volume backing (ACA jobs/apps natively mount shares
// via ManagedEnvironmentsStorages).
//
// Backing is REQUIRED — no automatic fallback. Operators set it
// (typically "azure-files-ephemeral") via the
// SOCKERLESS_ACA_SHARED_VOLUMES env's 4/5-tuple format.
type SharedVolume struct {
	Name          string // logical volume name used in spawned sub-tasks
	ContainerPath string // path inside the calling container (= the bind-mount source)
	ShareName     string // Azure Files share backing this volume
	Backing       string // REQUIRED: storage backing kind (e.g. "azure-files-ephemeral")
	// StorageAccount hosting the share; defaults to Config.StorageAccount.
	StorageAccount string
}

// AccountOrDefault returns the share's storage account, falling back
// to the operator's configured default account when the volume entry
// doesn't pin one explicitly.
func (v SharedVolume) AccountOrDefault(def string) string {
	if v.StorageAccount != "" {
		return v.StorageAccount
	}
	return def
}

// AsRef returns the cloud-agnostic SharedVolumeRef the storage backing
// driver consumes. Empty Backing flows through unchanged so the
// registry's Resolve fails loudly on it.
func (v SharedVolume) AsRef(defaultAccount string) core.SharedVolumeRef {
	return core.SharedVolumeRef{
		Name:                v.Name,
		ContainerPath:       v.ContainerPath,
		Backing:             core.StorageBacking(v.Backing),
		AzureStorageAccount: v.AccountOrDefault(defaultAccount),
		AzureShareName:      v.ShareName,
	}
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	sharedVolumes, sharedVolumesErr := parseSharedVolumes(os.Getenv("SOCKERLESS_ACA_SHARED_VOLUMES"))
	return Config{
		SubscriptionID:        os.Getenv("SOCKERLESS_ACA_SUBSCRIPTION_ID"),
		ResourceGroup:         os.Getenv("SOCKERLESS_ACA_RESOURCE_GROUP"),
		Environment:           envOrDefault("SOCKERLESS_ACA_ENVIRONMENT", "sockerless"),
		Location:              envOrDefault("SOCKERLESS_ACA_LOCATION", "eastus"),
		LogAnalyticsWorkspace: os.Getenv("SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE"),
		StorageAccount:        os.Getenv("SOCKERLESS_ACA_STORAGE_ACCOUNT"),
		ACRName:               os.Getenv("SOCKERLESS_AZURE_ACR_NAME"),
		BuildStorageAccount:   os.Getenv("SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT"),
		BuildContainer:        os.Getenv("SOCKERLESS_AZURE_BUILD_CONTAINER"),
		BuildPlatform:         envOrDefault("SOCKERLESS_AZURE_BUILD_PLATFORM", "linux/amd64"),
		EndpointURL:           os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:          parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		UseApp:                os.Getenv("SOCKERLESS_ACA_USE_APP") == "1",
		CallbackURL:           os.Getenv("SOCKERLESS_CALLBACK_URL"),
		BootstrapBinaryPath:   os.Getenv("SOCKERLESS_ACA_BOOTSTRAP"),
		EnableCommit:          os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		NetworkDiscovery:      networkDiscoveryFromEnv("SOCKERLESS_ACA_NETWORK_DISCOVERY", api.NetworkDiscoveryCloudDNS),
		Access:                accessFromEnv("SOCKERLESS_ACA_ACCESS", api.AccessMechanismNoneInternal),
		AccessPrincipal:       os.Getenv("SOCKERLESS_ACA_ACCESS_PRINCIPAL"),
		SharedVolumes:         sharedVolumes,
		sharedVolumesErr:      sharedVolumesErr,
	}
}

// parseSharedVolumes parses SOCKERLESS_ACA_SHARED_VOLUMES.
//
// Format: `name=containerPath=share=backing[=storageAccount],...`
// `backing` is REQUIRED — operators MUST explicitly choose the storage
// backing (typically `azure-files-ephemeral`) per the no-fallbacks
// directive. The trailing storageAccount is optional — defaults to
// Config.StorageAccount. Returns (nil, nil) for empty input. Malformed
// entries are a hard error — the caller surfaces it via Config.Validate
// so the operator's misconfiguration fails the backend startup instead
// of silently dropping the volume mapping.
func parseSharedVolumes(s string) ([]SharedVolume, error) {
	if strings.TrimSpace(s) == "" {
		return nil, nil
	}
	var out []SharedVolume
	for _, entry := range strings.Split(s, ",") {
		entry = strings.TrimSpace(entry)
		if entry == "" {
			continue
		}
		parts := strings.Split(entry, "=")
		if len(parts) < 4 || len(parts) > 5 {
			return nil, fmt.Errorf("entry %q malformed: want name=containerPath=share=backing[=storageAccount]", entry)
		}
		sv := SharedVolume{
			Name:          strings.TrimSpace(parts[0]),
			ContainerPath: strings.TrimSpace(parts[1]),
			ShareName:     strings.TrimSpace(parts[2]),
			Backing:       strings.TrimSpace(parts[3]),
		}
		if len(parts) == 5 {
			sv.StorageAccount = strings.TrimSpace(parts[4])
		}
		if sv.Name == "" || sv.ContainerPath == "" || sv.ShareName == "" || sv.Backing == "" {
			return nil, fmt.Errorf("entry %q malformed: name, containerPath, share and backing must all be non-empty", entry)
		}
		out = append(out, sv)
	}
	return out, nil
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

// accessFromEnv reads the operator's chosen access mechanism from env
// or returns `def`. Validation against the per-backend supported set
// happens in Config.Validate.
func accessFromEnv(envVar string, def api.AccessMechanism) api.AccessMechanism {
	v := strings.TrimSpace(os.Getenv(envVar))
	if v == "" {
		return def
	}
	return api.AccessMechanism(v)
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

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Environment:   "sockerless",
		Location:      "eastus",
		BuildPlatform: "linux/amd64",
		PollInterval:  2 * time.Second,
	}
	if env.Azure != nil {
		c.SubscriptionID = env.Azure.SubscriptionID
		c.BuildStorageAccount = env.Azure.BuildStorageAccount
		c.BuildContainer = env.Azure.BuildContainer
		if env.Azure.BuildPlatform != "" {
			c.BuildPlatform = env.Azure.BuildPlatform
		}
		if aca := env.Azure.ACA; aca != nil {
			c.ResourceGroup = aca.ResourceGroup
			if aca.Environment != "" {
				c.Environment = aca.Environment
			}
			if aca.Location != "" {
				c.Location = aca.Location
			}
			c.LogAnalyticsWorkspace = aca.LogAnalyticsWorkspace
			c.StorageAccount = aca.StorageAccount
			c.ACRName = aca.ACRName
		}
	}
	c.EndpointURL = env.Common.EndpointURL
	if env.Common.PollInterval != "" {
		c.PollInterval = parseDuration(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
	}
	c.NetworkDiscovery = networkDiscoveryFromEnv("SOCKERLESS_ACA_NETWORK_DISCOVERY", api.NetworkDiscoveryCloudDNS)
	c.Access = accessFromEnv("SOCKERLESS_ACA_ACCESS", api.AccessMechanismNoneInternal)
	c.AccessPrincipal = os.Getenv("SOCKERLESS_ACA_ACCESS_PRINCIPAL")
	c.SharedVolumes, c.sharedVolumesErr = parseSharedVolumes(os.Getenv("SOCKERLESS_ACA_SHARED_VOLUMES"))
	return c
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if c.sharedVolumesErr != nil {
		return fmt.Errorf("SOCKERLESS_ACA_SHARED_VOLUMES: %w", c.sharedVolumesErr)
	}
	if c.SubscriptionID == "" {
		return fmt.Errorf("SOCKERLESS_ACA_SUBSCRIPTION_ID is required")
	}
	if c.ResourceGroup == "" {
		return fmt.Errorf("SOCKERLESS_ACA_RESOURCE_GROUP is required")
	}
	if c.UseApp && c.Environment == "" {
		return fmt.Errorf("SOCKERLESS_ACA_USE_APP=1 requires SOCKERLESS_ACA_ENVIRONMENT — Apps need an existing managed environment with VNet integration for peer-reachable internal FQDNs")
	}
	switch c.NetworkDiscovery {
	case api.NetworkDiscoveryCloudDNS, api.NetworkDiscoveryHostAliases, api.NetworkDiscoveryNATGatewayOnly:
		// supported
	default:
		return fmt.Errorf("SOCKERLESS_ACA_NETWORK_DISCOVERY=%q not supported by aca (one of cloud-dns, host-aliases, nat-gateway-only required)", c.NetworkDiscovery)
	}
	switch c.Access {
	case api.AccessMechanismNoneInternal, api.AccessMechanismAzureAD:
		// supported
	default:
		return fmt.Errorf("SOCKERLESS_ACA_ACCESS=%q not supported by aca (one of none-internal, azure-ad required)", c.Access)
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
