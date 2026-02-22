package cloudrun

import (
	"fmt"
	"os"
)

// Config holds Cloud Run backend configuration.
type Config struct {
	Project      string
	Region       string
	VPCConnector string
	LogID        string
	AgentImage   string
	AgentToken   string
	CallbackURL  string // Backend URL for reverse agent connections
	EndpointURL  string
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Project:      os.Getenv("SOCKERLESS_GCR_PROJECT"),
		Region:       envOrDefault("SOCKERLESS_GCR_REGION", "us-central1"),
		VPCConnector: os.Getenv("SOCKERLESS_GCR_VPC_CONNECTOR"),
		LogID:        envOrDefault("SOCKERLESS_GCR_LOG_ID", "sockerless"),
		AgentImage:   envOrDefault("SOCKERLESS_GCR_AGENT_IMAGE", "sockerless/agent:latest"),
		AgentToken:   os.Getenv("SOCKERLESS_GCR_AGENT_TOKEN"),
		CallbackURL:  os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EndpointURL:  os.Getenv("SOCKERLESS_ENDPOINT_URL"),
	}
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if c.EndpointURL != "" {
		return nil
	}
	if c.Project == "" {
		return fmt.Errorf("SOCKERLESS_GCR_PROJECT is required")
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
	}
	return def
}
