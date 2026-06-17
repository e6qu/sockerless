package azf

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	"github.com/sockerless/api"
	core "github.com/sockerless/backend-core"
)

// Config holds Azure Functions backend configuration.
type Config struct {
	SubscriptionID string
	ResourceGroup  string
	Location       string
	StorageAccount string
	Registry       string
	AppServicePlan string
	Timeout        int
	// AttachTimeout bounds how long a buffered attach reader waits for the
	// in-function invoke to publish its captured output before returning EOF.
	// Defaults to the invoke Timeout (so a healthy invoke always publishes
	// first and this is a pure safety net); a shorter value bounds the rare
	// case where a FaaS pod stalls/expires before an instant workload runs, so
	// an attached reader fails fast instead of stranding near the invoke cap.
	// Set via SOCKERLESS_AZF_ATTACH_TIMEOUT_SEC.
	AttachTimeout         int
	LogAnalyticsWorkspace string
	BuildStorageAccount   string        // Storage account for ACR build context
	BuildContainer        string        // Blob container for ACR build context
	BuildPlatform         string        // Docker build platform for overlay images
	EndpointURL           string        // Custom endpoint URL
	PollInterval          time.Duration // Cloud API poll interval (default 2s)

	// CallbackURL is the reverse-agent WebSocket URL injected into the
	// function app container env so a bootstrap inside can dial back
	// to the backend's /v1/azf/reverse endpoint.
	CallbackURL string

	// BootstrapBinaryPath is the on-disk path of the AZF-compatible
	// reverse-agent bootstrap binary. When set, stock images are wrapped
	// through ACR Tasks so the Function App container serves HTTP and
	// dials the reverse-agent endpoint.
	// Set via SOCKERLESS_AZF_BOOTSTRAP.
	BootstrapBinaryPath string

	// BootstrapBinaryHash is a SHA-256-prefix hash of BootstrapBinaryPath,
	// computed once at startup so overlay tags invalidate when the
	// bootstrap binary changes.
	BootstrapBinaryHash string

	// EnableCommit opts into the agent-driven `docker commit` path.
	// See backends/core.CommitContainerViaAgent. Set via
	// `SOCKERLESS_ENABLE_COMMIT=1`.
	EnableCommit bool

	// NetworkDiscovery selects the per-backend driver wired into
	// s.NetworkDiscovery. AZF's native is nat-gateway-only — Azure
	// Functions don't expose per-invocation IPs. Operators may
	// override to host-aliases (in-process registry) for the
	// multi-container pattern, or cloud-dns (Azure Private DNS) for
	// proper cross-app peer resolution backed by the per-network zone
	// created at NetworkCreate time.
	// Set via SOCKERLESS_AZF_NETWORK_DISCOVERY.
	NetworkDiscovery api.NetworkDiscoveryKind

	// Access selects the ingress-auth + caller-side signer mechanism
	// wired into s.Access. AZF's native is none-internal (function
	// app keys gate access at the platform layer). Operators may
	// override to azure-ad to gate ingress with an Easy Auth (AAD
	// provider) on the function app; the caller-side signer mints
	// OAuth2 bearer tokens via DefaultAzureCredential.
	// Set via SOCKERLESS_AZF_ACCESS.
	Access api.AccessMechanism

	// AccessPrincipal is the workload's MSI client ID or service
	// principal AppId reported via WorkloadPrincipal. Informational —
	// the actual signing identity comes from azidentity.
	// Set via SOCKERLESS_AZF_ACCESS_PRINCIPAL.
	AccessPrincipal string

	// SharedVolumes mirrors the ECS / Lambda / ACA backends' same-named
	// field. When sockerless runs inside an Azure Function App that has
	// Azure Files shares mounted at known paths (via the site's
	// azurestorageaccounts config), and the caller (e.g.
	// github-actions-runner) does
	// `docker create -v /home/runner/_work:/__w alpine`, sockerless
	// translates the host bind mount into a named-volume reference
	// whose Azure Files share is shared with the runner app. Sub-tasks
	// (spawned as further Function Apps) mount the same share.
	// Format: SOCKERLESS_AZF_SHARED_VOLUMES="name=path=share=backing[=storageAccount],..."
	SharedVolumes []SharedVolume

	// sharedVolumesErr carries a SOCKERLESS_AZF_SHARED_VOLUMES parse
	// failure from ConfigFromEnv to Validate so misconfiguration fails
	// startup loudly instead of silently dropping entries.
	sharedVolumesErr error
}

// SharedVolume describes a workspace volume mounted via Azure Files
// that the caller (the runner Function App) shares with sockerless.
// When `docker create` sees a bind mount whose source matches
// ContainerPath, the bind is rewritten to a named volume named Name
// backed by the Azure Files share ShareName. Mirror of
// `ecs.SharedVolume` + `aca.SharedVolume`.
//
// Backing is REQUIRED — no automatic fallback. Operators set it
// (typically "azure-files-ephemeral") via the
// SOCKERLESS_AZF_SHARED_VOLUMES env's 4/5-tuple format.
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
	sharedVolumes, sharedVolumesErr := parseSharedVolumes(os.Getenv("SOCKERLESS_AZF_SHARED_VOLUMES"))
	return Config{
		SubscriptionID:        os.Getenv("SOCKERLESS_AZF_SUBSCRIPTION_ID"),
		ResourceGroup:         os.Getenv("SOCKERLESS_AZF_RESOURCE_GROUP"),
		Location:              envOrDefault("SOCKERLESS_AZF_LOCATION", "eastus"),
		StorageAccount:        os.Getenv("SOCKERLESS_AZF_STORAGE_ACCOUNT"),
		Registry:              os.Getenv("SOCKERLESS_AZF_REGISTRY"),
		AppServicePlan:        os.Getenv("SOCKERLESS_AZF_APP_SERVICE_PLAN"),
		Timeout:               envOrDefaultInt("SOCKERLESS_AZF_TIMEOUT", 600),
		AttachTimeout:         envOrDefaultInt("SOCKERLESS_AZF_ATTACH_TIMEOUT_SEC", envOrDefaultInt("SOCKERLESS_AZF_TIMEOUT", 600)),
		LogAnalyticsWorkspace: os.Getenv("SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE"),
		BuildStorageAccount:   os.Getenv("SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT"),
		BuildContainer:        os.Getenv("SOCKERLESS_AZURE_BUILD_CONTAINER"),
		BuildPlatform:         envOrDefault("SOCKERLESS_AZURE_BUILD_PLATFORM", "linux/amd64"),
		EndpointURL:           os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:          parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		CallbackURL:           os.Getenv("SOCKERLESS_CALLBACK_URL"),
		BootstrapBinaryPath:   os.Getenv("SOCKERLESS_AZF_BOOTSTRAP"),
		EnableCommit:          os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		NetworkDiscovery:      networkDiscoveryFromEnv("SOCKERLESS_AZF_NETWORK_DISCOVERY", api.NetworkDiscoveryNATGatewayOnly),
		Access:                accessFromEnv("SOCKERLESS_AZF_ACCESS", api.AccessMechanismNoneInternal),
		AccessPrincipal:       os.Getenv("SOCKERLESS_AZF_ACCESS_PRINCIPAL"),
		SharedVolumes:         sharedVolumes,
		sharedVolumesErr:      sharedVolumesErr,
	}
}

// parseSharedVolumes parses SOCKERLESS_AZF_SHARED_VOLUMES.
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

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Location:      "eastus",
		Timeout:       600,
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
		if azf := env.Azure.AZF; azf != nil {
			c.ResourceGroup = azf.ResourceGroup
			if azf.Location != "" {
				c.Location = azf.Location
			}
			c.StorageAccount = azf.StorageAccount
			c.Registry = azf.Registry
			c.AppServicePlan = azf.AppServicePlan
			if azf.Timeout > 0 {
				c.Timeout = azf.Timeout
			}
			c.LogAnalyticsWorkspace = azf.LogAnalyticsWorkspace
		}
	}
	c.EndpointURL = env.Common.EndpointURL
	if env.Common.PollInterval != "" {
		c.PollInterval = parseDuration(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
	}
	c.NetworkDiscovery = networkDiscoveryFromEnv("SOCKERLESS_AZF_NETWORK_DISCOVERY", api.NetworkDiscoveryNATGatewayOnly)
	c.Access = accessFromEnv("SOCKERLESS_AZF_ACCESS", api.AccessMechanismNoneInternal)
	c.AccessPrincipal = os.Getenv("SOCKERLESS_AZF_ACCESS_PRINCIPAL")
	c.BootstrapBinaryPath = os.Getenv("SOCKERLESS_AZF_BOOTSTRAP")
	c.SharedVolumes, c.sharedVolumesErr = parseSharedVolumes(os.Getenv("SOCKERLESS_AZF_SHARED_VOLUMES"))
	return c
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if err := core.ValidateDurationEnvs("SOCKERLESS_POLL_INTERVAL"); err != nil {
		return err
	}
	if err := core.ValidateJobTimeoutEnv(); err != nil {
		return err
	}
	if c.sharedVolumesErr != nil {
		return fmt.Errorf("SOCKERLESS_AZF_SHARED_VOLUMES: %w", c.sharedVolumesErr)
	}
	if c.SubscriptionID == "" {
		return fmt.Errorf("SOCKERLESS_AZF_SUBSCRIPTION_ID is required")
	}
	if c.ResourceGroup == "" {
		return fmt.Errorf("SOCKERLESS_AZF_RESOURCE_GROUP is required")
	}
	if c.StorageAccount == "" {
		return fmt.Errorf("SOCKERLESS_AZF_STORAGE_ACCOUNT is required")
	}
	switch c.NetworkDiscovery {
	case api.NetworkDiscoveryNATGatewayOnly, api.NetworkDiscoveryHostAliases, api.NetworkDiscoveryCloudDNS:
		// supported
	default:
		return fmt.Errorf("SOCKERLESS_AZF_NETWORK_DISCOVERY=%q not supported by azf (one of nat-gateway-only, host-aliases, cloud-dns required)", c.NetworkDiscovery)
	}
	switch c.Access {
	case api.AccessMechanismNoneInternal, api.AccessMechanismAzureAD:
		// supported
	default:
		return fmt.Errorf("SOCKERLESS_AZF_ACCESS=%q not supported by azf (one of none-internal, azure-ad required)", c.Access)
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
