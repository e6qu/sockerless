package lambda

import (
	"fmt"
	"os"
	"strconv"
	"strings"
	"time"

	core "github.com/sockerless/backend-core"
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
	CallbackURL      string        // Backend URL for reverse agent connections
	EndpointURL      string        // Custom endpoint URL
	PollInterval     time.Duration // Cloud API poll interval (default 2s)
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
		PollInterval:     parseDuration(os.Getenv("SOCKERLESS_POLL_INTERVAL"), 2*time.Second),
	}
}

// ConfigFromEnvironment creates Config from a unified config environment.
func ConfigFromEnvironment(env *core.Environment, sim *core.SimulatorConfig) Config {
	c := Config{
		Region:     "us-east-1",
		LogGroup:   "/sockerless/lambda",
		MemorySize: 1024,
		Timeout:    900,
		PollInterval: 2 * time.Second,
	}
	if env.AWS != nil {
		if env.AWS.Region != "" {
			c.Region = env.AWS.Region
		}
		if l := env.AWS.Lambda; l != nil {
			c.RoleARN = l.RoleARN
			if l.LogGroup != "" {
				c.LogGroup = l.LogGroup
			}
			if l.MemorySize > 0 {
				c.MemorySize = l.MemorySize
			}
			if l.Timeout > 0 {
				c.Timeout = l.Timeout
			}
			c.SubnetIDs = l.Subnets
			c.SecurityGroupIDs = l.SecurityGroups
		}
	}
	c.CallbackURL = env.Common.CallbackURL
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
