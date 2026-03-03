package gcf

import (
	"fmt"
	"os"
	"strconv"
	"time"
)

// Config holds Cloud Run Functions backend configuration.
type Config struct {
	Project        string
	Region         string
	ServiceAccount string
	Timeout        int
	Memory         string
	CPU            string
	CallbackURL    string        // Backend URL for reverse agent connections
	EndpointURL    string        // Custom endpoint URL
	PollInterval   time.Duration // Cloud API poll interval (default 2s)
	LogTimeout     time.Duration // Cloud Logging query timeout (default 30s)
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
		CallbackURL:    os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EndpointURL:    os.Getenv("SOCKERLESS_ENDPOINT_URL"),
		PollInterval:   parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
		LogTimeout:     parseDuration(os.Getenv("SOCKERLESS_LOG_TIMEOUT"), 30*time.Second),
	}
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if c.Project == "" {
		return fmt.Errorf("SOCKERLESS_GCF_PROJECT is required")
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
