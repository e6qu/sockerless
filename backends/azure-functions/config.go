package azf

import (
	"fmt"
	"os"
	"strconv"
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
	CallbackURL           string // Backend URL for reverse agent connections
	EndpointURL           string // Custom endpoint URL for simulator mode
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
		CallbackURL:           os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EndpointURL:           os.Getenv("SOCKERLESS_ENDPOINT_URL"),
	}
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if c.EndpointURL != "" {
		return nil
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
	return nil
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
