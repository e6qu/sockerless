package core

import (
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
)

// PoolConfig defines a single backend pool.
type PoolConfig struct {
	Name           string `json:"name"`
	BackendType    string `json:"backend_type"`
	MaxConcurrency int    `json:"max_concurrency"` // 0 = unlimited
	QueueSize      int    `json:"queue_size"`       // 0 = no queue, reject at capacity
}

// PoolsConfig defines the set of backend pools and which is the default.
type PoolsConfig struct {
	DefaultPool string       `json:"default_pool"`
	Pools       []PoolConfig `json:"pools"`
}

// ValidBackendTypes lists the allowed backend_type values, matching
// BackendDescriptor.Driver across all backend implementations.
var ValidBackendTypes = map[string]bool{
	"memory":              true,
	"ecs-fargate":         true,
	"lambda":              true,
	"cloudrun-jobs":       true,
	"cloud-run-functions": true,
	"aca-jobs":            true,
	"azure-functions":     true,
}

// ValidatePoolsConfig checks that a PoolsConfig is well-formed.
func ValidatePoolsConfig(cfg PoolsConfig) error {
	if len(cfg.Pools) == 0 {
		return fmt.Errorf("pools config: at least one pool is required")
	}

	seen := make(map[string]bool, len(cfg.Pools))
	for i, p := range cfg.Pools {
		if p.Name == "" {
			return fmt.Errorf("pools config: pool %d has empty name", i)
		}
		if seen[p.Name] {
			return fmt.Errorf("pools config: duplicate pool name %q", p.Name)
		}
		seen[p.Name] = true

		if !ValidBackendTypes[p.BackendType] {
			return fmt.Errorf("pools config: pool %q has invalid backend_type %q", p.Name, p.BackendType)
		}
		if p.MaxConcurrency < 0 {
			return fmt.Errorf("pools config: pool %q has negative max_concurrency %d", p.Name, p.MaxConcurrency)
		}
		if p.QueueSize < 0 {
			return fmt.Errorf("pools config: pool %q has negative queue_size %d", p.Name, p.QueueSize)
		}
	}

	if cfg.DefaultPool == "" {
		return fmt.Errorf("pools config: default_pool is required")
	}
	if !seen[cfg.DefaultPool] {
		return fmt.Errorf("pools config: default_pool %q does not match any pool", cfg.DefaultPool)
	}

	return nil
}

// DefaultPoolsConfig returns a backward-compatible single-pool configuration.
func DefaultPoolsConfig() PoolsConfig {
	return PoolsConfig{
		DefaultPool: "default",
		Pools: []PoolConfig{{
			Name:           "default",
			BackendType:    "memory",
			MaxConcurrency: 0,
			QueueSize:      0,
		}},
	}
}

// LoadPoolsConfig loads pool configuration from the environment, home directory,
// or falls back to the default single-pool config.
func LoadPoolsConfig() (PoolsConfig, error) {
	var data []byte
	var err error

	if envPath := os.Getenv("SOCKERLESS_POOLS_CONFIG"); envPath != "" {
		data, err = os.ReadFile(envPath)
		if err != nil {
			return PoolsConfig{}, fmt.Errorf("pools config: reading %s: %w", envPath, err)
		}
	} else {
		homePath := filepath.Join(sockerlessHomeDir(), "pools.json")
		data, err = os.ReadFile(homePath)
		if err != nil {
			if os.IsNotExist(err) {
				return DefaultPoolsConfig(), nil
			}
			return PoolsConfig{}, fmt.Errorf("pools config: reading %s: %w", homePath, err)
		}
	}

	var cfg PoolsConfig
	if err := json.Unmarshal(data, &cfg); err != nil {
		return PoolsConfig{}, fmt.Errorf("pools config: invalid JSON: %w", err)
	}

	if err := ValidatePoolsConfig(cfg); err != nil {
		return PoolsConfig{}, err
	}

	return cfg, nil
}

// GetPool returns the pool with the given name, or nil if not found.
func (cfg PoolsConfig) GetPool(name string) *PoolConfig {
	for i := range cfg.Pools {
		if cfg.Pools[i].Name == name {
			return &cfg.Pools[i]
		}
	}
	return nil
}

// PoolNames returns the names of all pools in order.
func (cfg PoolsConfig) PoolNames() []string {
	names := make([]string, len(cfg.Pools))
	for i, p := range cfg.Pools {
		names[i] = p.Name
	}
	return names
}
