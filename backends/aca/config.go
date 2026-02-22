package aca

import (
	"fmt"
	"os"
)

// Config holds ACA backend configuration.
type Config struct {
	SubscriptionID        string
	ResourceGroup         string
	Environment           string
	Location              string
	LogAnalyticsWorkspace string
	StorageAccount        string
	AgentImage            string
	AgentToken            string
	CallbackURL           string // Backend URL for reverse agent connections
	EndpointURL           string
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
		AgentImage:            envOrDefault("SOCKERLESS_ACA_AGENT_IMAGE", "sockerless/agent:latest"),
		AgentToken:            os.Getenv("SOCKERLESS_ACA_AGENT_TOKEN"),
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
		return fmt.Errorf("SOCKERLESS_ACA_SUBSCRIPTION_ID is required")
	}
	if c.ResourceGroup == "" {
		return fmt.Errorf("SOCKERLESS_ACA_RESOURCE_GROUP is required")
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
