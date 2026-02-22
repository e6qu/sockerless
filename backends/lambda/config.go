package lambda

import (
	"fmt"
	"os"
	"strconv"
	"strings"
)

// Config holds Lambda backend configuration.
type Config struct {
	Region           string
	RoleARN          string
	LogGroup         string
	MemorySize       int
	Timeout          int
	SubnetIDs        []string
	SecurityGroupIDs []string
	CallbackURL      string // Backend URL for reverse agent connections
	EndpointURL      string // Custom endpoint URL for simulator mode
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Region:           envOrDefault("AWS_REGION", "us-east-1"),
		RoleARN:          os.Getenv("SOCKERLESS_LAMBDA_ROLE_ARN"),
		LogGroup:         envOrDefault("SOCKERLESS_LAMBDA_LOG_GROUP", "/sockerless/lambda"),
		MemorySize:       envOrDefaultInt("SOCKERLESS_LAMBDA_MEMORY_SIZE", 1024),
		Timeout:          envOrDefaultInt("SOCKERLESS_LAMBDA_TIMEOUT", 900),
		SubnetIDs:        splitCSV(os.Getenv("SOCKERLESS_LAMBDA_SUBNETS")),
		SecurityGroupIDs: splitCSV(os.Getenv("SOCKERLESS_LAMBDA_SECURITY_GROUPS")),
		CallbackURL:      os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EndpointURL:      os.Getenv("SOCKERLESS_ENDPOINT_URL"),
	}
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if c.EndpointURL != "" {
		return nil // simulator mode: skip infra checks
	}
	if c.RoleARN == "" {
		return fmt.Errorf("SOCKERLESS_LAMBDA_ROLE_ARN is required")
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

func splitCSV(s string) []string {
	if s == "" {
		return nil
	}
	parts := strings.Split(s, ",")
	result := make([]string, 0, len(parts))
	for _, p := range parts {
		p = strings.TrimSpace(p)
		if p != "" {
			result = append(result, p)
		}
	}
	return result
}
