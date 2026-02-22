package ecs

import (
	"fmt"
	"os"
	"strings"
)

// Config holds ECS backend configuration.
type Config struct {
	Region          string
	Cluster         string
	Subnets         []string
	SecurityGroups  []string
	TaskRoleARN     string
	ExecutionRoleARN string
	LogGroup        string
	AgentImage      string   // Image containing the agent binary
	AgentEFSID      string   // EFS filesystem ID for agent binary
	AgentToken      string   // Default agent token
	AssignPublicIP  bool
	CallbackURL     string   // Backend URL for reverse agent connections
	EndpointURL     string   // Custom endpoint URL for simulator mode
}

// ConfigFromEnv loads configuration from environment variables.
func ConfigFromEnv() Config {
	return Config{
		Region:           envOrDefault("AWS_REGION", "us-east-1"),
		Cluster:          envOrDefault("SOCKERLESS_ECS_CLUSTER", "sockerless"),
		Subnets:          splitCSV(os.Getenv("SOCKERLESS_ECS_SUBNETS")),
		SecurityGroups:   splitCSV(os.Getenv("SOCKERLESS_ECS_SECURITY_GROUPS")),
		TaskRoleARN:      os.Getenv("SOCKERLESS_ECS_TASK_ROLE_ARN"),
		ExecutionRoleARN: os.Getenv("SOCKERLESS_ECS_EXECUTION_ROLE_ARN"),
		LogGroup:         envOrDefault("SOCKERLESS_ECS_LOG_GROUP", "/sockerless"),
		AgentImage:       envOrDefault("SOCKERLESS_AGENT_IMAGE", "sockerless/agent:latest"),
		AgentEFSID:       os.Getenv("SOCKERLESS_AGENT_EFS_ID"),
		AgentToken:       envOrDefault("SOCKERLESS_AGENT_TOKEN", ""),
		AssignPublicIP:   os.Getenv("SOCKERLESS_ECS_PUBLIC_IP") == "true",
		CallbackURL:      os.Getenv("SOCKERLESS_CALLBACK_URL"),
		EndpointURL:      os.Getenv("SOCKERLESS_ENDPOINT_URL"),
	}
}

// Validate checks required configuration.
func (c Config) Validate() error {
	if c.EndpointURL != "" {
		return nil // simulator mode: skip infra checks
	}
	if c.Cluster == "" {
		return fmt.Errorf("ECS cluster name is required")
	}
	if len(c.Subnets) == 0 {
		return fmt.Errorf("at least one subnet is required")
	}
	if c.ExecutionRoleARN == "" {
		return fmt.Errorf("execution role ARN is required")
	}
	return nil
}

func envOrDefault(key, def string) string {
	if v := os.Getenv(key); v != "" {
		return v
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
