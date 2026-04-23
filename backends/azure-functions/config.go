package azf

import (
	"fmt"
	"os"
	"strconv"
	"time"

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
	// to the backend's /v1/azf/reverse endpoint. Phase 98.
	CallbackURL string
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
	}
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
