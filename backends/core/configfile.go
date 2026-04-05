package core

import (
	"fmt"
	"os"
	"path/filepath"
	"syscall"

	"gopkg.in/yaml.v3"
)

// UnifiedConfig is the top-level structure of config.yaml.
type UnifiedConfig struct {
	Simulators   map[string]*SimulatorConfig `yaml:"simulators,omitempty"`
	Environments map[string]*Environment     `yaml:"environments"`
}

// SimulatorConfig defines a named cloud simulator.
type SimulatorConfig struct {
	Cloud    string `yaml:"cloud"`
	Port     int    `yaml:"port,omitempty"`
	GRPCPort int    `yaml:"grpc_port,omitempty"`
	LogLevel string `yaml:"log_level,omitempty"`
}

// Environment defines a single named environment.
type Environment struct {
	Backend   string       `yaml:"backend"`
	Addr      string       `yaml:"addr,omitempty"`
	LogLevel  string       `yaml:"log_level,omitempty"`
	Simulator string       `yaml:"simulator,omitempty"`
	AWS       *AWSConfig   `yaml:"aws,omitempty"`
	GCP       *GCPConfig   `yaml:"gcp,omitempty"`
	Azure     *AzureConfig `yaml:"azure,omitempty"`
	Common    CommonConfig `yaml:"common,omitempty"`
}

// CommonConfig holds fields shared across all backends.
type CommonConfig struct {
	AgentImage   string `yaml:"agent_image,omitempty"`
	AgentToken   string `yaml:"agent_token,omitempty"`
	CallbackURL  string `yaml:"callback_url,omitempty"`
	EndpointURL  string `yaml:"endpoint_url,omitempty"`
	PollInterval string `yaml:"poll_interval,omitempty"`
	AgentTimeout string `yaml:"agent_timeout,omitempty"`
}

// AWSConfig holds AWS-specific configuration.
type AWSConfig struct {
	Region           string           `yaml:"region,omitempty"`
	CodeBuildProject string           `yaml:"codebuild_project,omitempty"`
	BuildBucket      string           `yaml:"build_bucket,omitempty"`
	ECS              *ECSEnvConfig    `yaml:"ecs,omitempty"`
	Lambda           *LambdaEnvConfig `yaml:"lambda,omitempty"`
}

// GCPConfig holds GCP-specific configuration.
type GCPConfig struct {
	Project     string             `yaml:"project,omitempty"`
	BuildBucket string             `yaml:"build_bucket,omitempty"`
	CloudRun    *CloudRunEnvConfig `yaml:"cloudrun,omitempty"`
	GCF         *GCFEnvConfig      `yaml:"gcf,omitempty"`
}

// AzureConfig holds Azure-specific configuration.
type AzureConfig struct {
	SubscriptionID      string        `yaml:"subscription_id,omitempty"`
	BuildStorageAccount string        `yaml:"build_storage_account,omitempty"`
	BuildContainer      string        `yaml:"build_container,omitempty"`
	ACA                 *ACAEnvConfig `yaml:"aca,omitempty"`
	AZF                 *AZFEnvConfig `yaml:"azf,omitempty"`
}

// ECSEnvConfig holds ECS-specific settings.
type ECSEnvConfig struct {
	Cluster          string   `yaml:"cluster,omitempty"`
	Subnets          []string `yaml:"subnets,omitempty"`
	SecurityGroups   []string `yaml:"security_groups,omitempty"`
	TaskRoleARN      string   `yaml:"task_role_arn,omitempty"`
	ExecutionRoleARN string   `yaml:"execution_role_arn,omitempty"`
	LogGroup         string   `yaml:"log_group,omitempty"`
	AssignPublicIP   bool     `yaml:"assign_public_ip,omitempty"`
	AgentEFSID       string   `yaml:"agent_efs_id,omitempty"`
}

// LambdaEnvConfig holds Lambda-specific settings.
type LambdaEnvConfig struct {
	RoleARN        string   `yaml:"role_arn,omitempty"`
	LogGroup       string   `yaml:"log_group,omitempty"`
	MemorySize     int      `yaml:"memory_size,omitempty"`
	Timeout        int      `yaml:"timeout,omitempty"`
	Subnets        []string `yaml:"subnets,omitempty"`
	SecurityGroups []string `yaml:"security_groups,omitempty"`
}

// CloudRunEnvConfig holds Cloud Run-specific settings.
type CloudRunEnvConfig struct {
	Region       string `yaml:"region,omitempty"`
	VPCConnector string `yaml:"vpc_connector,omitempty"`
	LogID        string `yaml:"log_id,omitempty"`
	LogTimeout   string `yaml:"log_timeout,omitempty"`
}

// GCFEnvConfig holds Cloud Run Functions-specific settings.
type GCFEnvConfig struct {
	Region         string `yaml:"region,omitempty"`
	ServiceAccount string `yaml:"service_account,omitempty"`
	Timeout        int    `yaml:"timeout,omitempty"`
	Memory         string `yaml:"memory,omitempty"`
	CPU            string `yaml:"cpu,omitempty"`
	LogTimeout     string `yaml:"log_timeout,omitempty"`
}

// ACAEnvConfig holds Azure Container Apps-specific settings.
type ACAEnvConfig struct {
	ResourceGroup         string `yaml:"resource_group,omitempty"`
	Environment           string `yaml:"environment,omitempty"`
	Location              string `yaml:"location,omitempty"`
	LogAnalyticsWorkspace string `yaml:"log_analytics_workspace,omitempty"`
	StorageAccount        string `yaml:"storage_account,omitempty"`
	ACRName               string `yaml:"acr_name,omitempty"`
}

// AZFEnvConfig holds Azure Functions-specific settings.
type AZFEnvConfig struct {
	ResourceGroup         string `yaml:"resource_group,omitempty"`
	Location              string `yaml:"location,omitempty"`
	StorageAccount        string `yaml:"storage_account,omitempty"`
	Registry              string `yaml:"registry,omitempty"`
	AppServicePlan        string `yaml:"app_service_plan,omitempty"`
	Timeout               int    `yaml:"timeout,omitempty"`
	LogAnalyticsWorkspace string `yaml:"log_analytics_workspace,omitempty"`
}

// DefaultConfigPath returns the config file path from $SOCKERLESS_CONFIG or the default.
func DefaultConfigPath() string {
	if p := os.Getenv("SOCKERLESS_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(sockerlessHomeDir(), "config.yaml")
}

// LoadConfigFile reads and parses a unified config YAML file.
func LoadConfigFile(path string) (*UnifiedConfig, error) {
	data, err := os.ReadFile(path)
	if err != nil {
		return nil, err
	}
	var cfg UnifiedConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config %s: %w", path, err)
	}
	if cfg.Environments == nil {
		cfg.Environments = make(map[string]*Environment)
	}
	if cfg.Simulators == nil {
		cfg.Simulators = make(map[string]*SimulatorConfig)
	}
	return &cfg, nil
}

// ActiveEnvironment loads the default config file and returns the active environment.
// The active name comes from ~/.sockerless/active or $SOCKERLESS_CONTEXT (same as CLI contexts).
func ActiveEnvironment() (env *Environment, name string, err error) {
	name = activeContextName()
	if name == "" {
		return nil, "", fmt.Errorf("no active environment set")
	}
	path := DefaultConfigPath()
	cfg, err := LoadConfigFile(path)
	if err != nil {
		return nil, "", err
	}
	env, ok := cfg.Environments[name]
	if !ok {
		return nil, "", fmt.Errorf("active environment %q not found in %s", name, path)
	}
	return env, name, nil
}

// ActiveEnvironmentWithConfig loads the config and returns both the config and the active environment.
func ActiveEnvironmentWithConfig() (cfg *UnifiedConfig, env *Environment, name string, err error) {
	name = activeContextName()
	if name == "" {
		return nil, nil, "", fmt.Errorf("no active environment set")
	}
	path := DefaultConfigPath()
	cfg, err = LoadConfigFile(path)
	if err != nil {
		return nil, nil, "", err
	}
	env, ok := cfg.Environments[name]
	if !ok {
		return nil, nil, "", fmt.Errorf("active environment %q not found in %s", name, path)
	}
	return cfg, env, name, nil
}

// ResolveSimulator returns the SimulatorConfig referenced by an environment, or nil if none.
func (c *UnifiedConfig) ResolveSimulator(env *Environment) (*SimulatorConfig, error) {
	if env.Simulator == "" {
		return nil, nil
	}
	sim, ok := c.Simulators[env.Simulator]
	if !ok {
		return nil, fmt.Errorf("simulator %q not found", env.Simulator)
	}
	return sim, nil
}

// Validate checks that all simulator references are valid and backend types match cloud configs.
func (c *UnifiedConfig) Validate() error {
	for name, env := range c.Environments {
		if env.Backend == "" {
			return fmt.Errorf("environment %q: backend is required", name)
		}
		if env.Simulator != "" {
			if _, ok := c.Simulators[env.Simulator]; !ok {
				return fmt.Errorf("environment %q: simulator %q not found", name, env.Simulator)
			}
		}
	}
	for name, sim := range c.Simulators {
		switch sim.Cloud {
		case "aws", "gcp", "azure":
		default:
			return fmt.Errorf("simulator %q: invalid cloud %q", name, sim.Cloud)
		}
	}
	return nil
}

// Save writes the config to the given path atomically with file locking.
func (c *UnifiedConfig) Save(path string) error {
	data, err := yaml.Marshal(c)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}

	dir := filepath.Dir(path)
	if err := os.MkdirAll(dir, 0o755); err != nil {
		return fmt.Errorf("create config dir: %w", err)
	}

	tmp := path + ".tmp"
	f, err := os.OpenFile(tmp, os.O_WRONLY|os.O_CREATE|os.O_TRUNC, 0o644)
	if err != nil {
		return fmt.Errorf("create temp config: %w", err)
	}

	if err := syscall.Flock(int(f.Fd()), syscall.LOCK_EX); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("lock config: %w", err)
	}

	if _, err := f.Write(data); err != nil {
		f.Close()
		os.Remove(tmp)
		return fmt.Errorf("write config: %w", err)
	}
	if err := f.Close(); err != nil {
		os.Remove(tmp)
		return fmt.Errorf("close config: %w", err)
	}

	return os.Rename(tmp, path)
}

// AddEnvironment adds or replaces an environment and saves.
func (c *UnifiedConfig) AddEnvironment(name string, env *Environment, path string) error {
	if c.Environments == nil {
		c.Environments = make(map[string]*Environment)
	}
	c.Environments[name] = env
	return c.Save(path)
}

// RemoveEnvironment removes an environment and saves.
func (c *UnifiedConfig) RemoveEnvironment(name, path string) error {
	if _, ok := c.Environments[name]; !ok {
		return fmt.Errorf("environment %q not found", name)
	}
	delete(c.Environments, name)
	return c.Save(path)
}

// AddSimulator adds or replaces a simulator and saves.
func (c *UnifiedConfig) AddSimulator(name string, sim *SimulatorConfig, path string) error {
	if c.Simulators == nil {
		c.Simulators = make(map[string]*SimulatorConfig)
	}
	c.Simulators[name] = sim
	return c.Save(path)
}

// RemoveSimulator removes a simulator and saves. Fails if any environment references it.
func (c *UnifiedConfig) RemoveSimulator(name, path string) error {
	if _, ok := c.Simulators[name]; !ok {
		return fmt.Errorf("simulator %q not found", name)
	}
	for envName, env := range c.Environments {
		if env.Simulator == name {
			return fmt.Errorf("cannot remove simulator %q: referenced by environment %q", name, envName)
		}
	}
	delete(c.Simulators, name)
	return c.Save(path)
}
