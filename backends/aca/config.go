package aca

import (
	"fmt"
	"os"
	"time"

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
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
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
		EndpointURL:           os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:          parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		UseApp:                os.Getenv("SOCKERLESS_ACA_USE_APP") == "1",
		CallbackURL:           os.Getenv("SOCKERLESS_CALLBACK_URL"),
	}
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Environment:  "sockerless",
		Location:     "eastus",
		PollInterval: 2 * time.Second,
	}
	if env.Azure != nil {
		c.SubscriptionID = env.Azure.SubscriptionID
		c.BuildStorageAccount = env.Azure.BuildStorageAccount
		c.BuildContainer = env.Azure.BuildContainer
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
	return c
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if c.SubscriptionID == "" {
		return fmt.Errorf("SOCKERLESS_ACA_SUBSCRIPTION_ID is required")
	}
	if c.ResourceGroup == "" {
		return fmt.Errorf("SOCKERLESS_ACA_RESOURCE_GROUP is required")
	}
	if c.UseApp && c.Environment == "" {
		return fmt.Errorf("SOCKERLESS_ACA_USE_APP=1 requires SOCKERLESS_ACA_ENVIRONMENT — Apps need an existing managed environment with VNet integration for peer-reachable internal FQDNs")
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
