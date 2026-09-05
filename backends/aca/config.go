package aca

import (
	"fmt"
	"os"
	"time"

	"github.com/sockerless/api"
	azurecommon "github.com/sockerless/azure-common"
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

	// SharedVolumes are the Azure Files shares the runner job or app
	// already mounts. When the caller (e.g. github-actions-runner) does
	// `docker create -v /home/runner/_work:/__w alpine`, sockerless
	// translates the host bind mount into a named-volume reference whose
	// share is shared with the runner task. Format:
	// azurecommon.SharedVolumeFormat, read from SOCKERLESS_ACA_SHARED_VOLUMES;
	// the storage account defaults to StorageAccount.
	SharedVolumes core.SharedVolumes

	// sharedVolumesErr carries a SOCKERLESS_ACA_SHARED_VOLUMES parse
	// failure from ConfigFromEnv to Validate so misconfiguration fails
	// startup loudly instead of silently dropping entries.
	sharedVolumesErr error
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	sharedVolumes, sharedVolumesErr := core.ParseSharedVolumes(os.Getenv("SOCKERLESS_ACA_SHARED_VOLUMES"), azurecommon.SharedVolumeFormat)
	return Config{
		SubscriptionID:        os.Getenv("SOCKERLESS_ACA_SUBSCRIPTION_ID"),
		ResourceGroup:         os.Getenv("SOCKERLESS_ACA_RESOURCE_GROUP"),
		Environment:           core.EnvOrDefault("SOCKERLESS_ACA_ENVIRONMENT", "sockerless"),
		Location:              core.EnvOrDefault("SOCKERLESS_ACA_LOCATION", "eastus"),
		LogAnalyticsWorkspace: os.Getenv("SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE"),
		StorageAccount:        os.Getenv("SOCKERLESS_ACA_STORAGE_ACCOUNT"),
		ACRName:               os.Getenv("SOCKERLESS_AZURE_ACR_NAME"),
		BuildStorageAccount:   os.Getenv("SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT"),
		BuildContainer:        os.Getenv("SOCKERLESS_AZURE_BUILD_CONTAINER"),
		BuildPlatform:         core.EnvOrDefault("SOCKERLESS_AZURE_BUILD_PLATFORM", "linux/amd64"),
		EndpointURL:           os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:          core.DurationOrDefault(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		UseApp:                os.Getenv("SOCKERLESS_ACA_USE_APP") == "1",
		CallbackURL:           os.Getenv("SOCKERLESS_CALLBACK_URL"),
		BootstrapBinaryPath:   os.Getenv("SOCKERLESS_ACA_BOOTSTRAP"),
		EnableCommit:          os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		NetworkDiscovery:      core.NetworkDiscoveryFromEnv("SOCKERLESS_ACA_NETWORK_DISCOVERY", api.NetworkDiscoveryCloudDNS),
		Access:                core.AccessFromEnv("SOCKERLESS_ACA_ACCESS", api.AccessMechanismNoneInternal),
		AccessPrincipal:       os.Getenv("SOCKERLESS_ACA_ACCESS_PRINCIPAL"),
		SharedVolumes:         sharedVolumes,
		sharedVolumesErr:      sharedVolumesErr,
	}
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
		c.PollInterval = core.DurationOrDefault(env.Common.PollInterval, c.PollInterval)
	}
	if sim != nil && sim.Port > 0 {
		c.EndpointURL = fmt.Sprintf("http://localhost:%d", sim.Port)
	}
	c.NetworkDiscovery = core.NetworkDiscoveryFromEnv("SOCKERLESS_ACA_NETWORK_DISCOVERY", api.NetworkDiscoveryCloudDNS)
	c.Access = core.AccessFromEnv("SOCKERLESS_ACA_ACCESS", api.AccessMechanismNoneInternal)
	c.AccessPrincipal = os.Getenv("SOCKERLESS_ACA_ACCESS_PRINCIPAL")
	c.SharedVolumes, c.sharedVolumesErr = core.ParseSharedVolumes(os.Getenv("SOCKERLESS_ACA_SHARED_VOLUMES"), azurecommon.SharedVolumeFormat)
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
