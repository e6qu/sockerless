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
	SubscriptionID        string
	ResourceGroup         string
	Location              string
	StorageAccount        string
	Registry              string
	AppServicePlan        string
	Timeout               int
	LogAnalyticsWorkspace string
	BuildStorageAccount   string        // Storage account for ACR build context
	BuildContainer        string        // Blob container for ACR build context
	EndpointURL           string        // Custom endpoint URL
	PollInterval          time.Duration // Cloud API poll interval (default 2s)

	// CallbackURL is the reverse-agent WebSocket URL injected into the
	// function app container env so a bootstrap inside can dial back
	// to the backend's /v1/azf/reverse endpoint.
	CallbackURL string

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
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		SubscriptionID:        os.Getenv("SOCKERLESS_AZF_SUBSCRIPTION_ID"),
		ResourceGroup:         os.Getenv("SOCKERLESS_AZF_RESOURCE_GROUP"),
		Location:              envOrDefault("SOCKERLESS_AZF_LOCATION", "eastus"),
		StorageAccount:        os.Getenv("SOCKERLESS_AZF_STORAGE_ACCOUNT"),
		Registry:              os.Getenv("SOCKERLESS_AZF_REGISTRY"),
		AppServicePlan:        os.Getenv("SOCKERLESS_AZF_APP_SERVICE_PLAN"),
		Timeout:               envOrDefaultInt("SOCKERLESS_AZF_TIMEOUT", 600),
		LogAnalyticsWorkspace: os.Getenv("SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE"),
		BuildStorageAccount:   os.Getenv("SOCKERLESS_AZURE_BUILD_STORAGE_ACCOUNT"),
		BuildContainer:        os.Getenv("SOCKERLESS_AZURE_BUILD_CONTAINER"),
		EndpointURL:           os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:          parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		CallbackURL:           os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EnableCommit:          os.Getenv("SOCKERLESS_ENABLE_COMMIT") == "1",
		NetworkDiscovery:      networkDiscoveryFromEnv("SOCKERLESS_AZF_NETWORK_DISCOVERY", api.NetworkDiscoveryNATGatewayOnly),
		Access:                accessFromEnv("SOCKERLESS_AZF_ACCESS", api.AccessMechanismNoneInternal),
		AccessPrincipal:       os.Getenv("SOCKERLESS_AZF_ACCESS_PRINCIPAL"),
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
		Location:     "eastus",
		Timeout:      600,
		PollInterval: 2 * time.Second,
	}
	if env.Azure != nil {
		c.SubscriptionID = env.Azure.SubscriptionID
		c.BuildStorageAccount = env.Azure.BuildStorageAccount
		c.BuildContainer = env.Azure.BuildContainer
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
	return c
}

// Validate checks required configuration.
func (c Config) Validate() error {
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
