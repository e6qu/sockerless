package main

import (
	"fmt"
	"os"
	"path/filepath"

	"gopkg.in/yaml.v3"
)

// unifiedConfig is the top-level structure of config.yaml.
type unifiedConfig struct {
	Simulators   map[string]*simulatorConfig `yaml:"simulators,omitempty"`
	Environments map[string]*environment     `yaml:"environments"`
}

type simulatorConfig struct {
	Cloud    string `yaml:"cloud"`
	Port     int    `yaml:"port,omitempty"`
	GRPCPort int    `yaml:"grpc_port,omitempty"`
	LogLevel string `yaml:"log_level,omitempty"`
}

type environment struct {
	Backend   string       `yaml:"backend"`
	Addr      string       `yaml:"addr,omitempty"`
	LogLevel  string       `yaml:"log_level,omitempty"`
	Simulator string       `yaml:"simulator,omitempty"`
	AWS       *awsConfig   `yaml:"aws,omitempty"`
	GCP       *gcpConfig   `yaml:"gcp,omitempty"`
	Azure     *azureConfig `yaml:"azure,omitempty"`
	Common    commonConfig `yaml:"common,omitempty"`
}

type commonConfig struct {
	AgentImage   string `yaml:"agent_image,omitempty"`
	AgentToken   string `yaml:"agent_token,omitempty"`
	CallbackURL  string `yaml:"callback_url,omitempty"`
	EndpointURL  string `yaml:"endpoint_url,omitempty"`
	PollInterval string `yaml:"poll_interval,omitempty"`
	AgentTimeout string `yaml:"agent_timeout,omitempty"`
}

type awsConfig struct {
	Region string           `yaml:"region,omitempty"`
	ECS    *ecsEnvConfig    `yaml:"ecs,omitempty"`
	Lambda *lambdaEnvConfig `yaml:"lambda,omitempty"`
}

type gcpConfig struct {
	Project  string             `yaml:"project,omitempty"`
	CloudRun *cloudRunEnvConfig `yaml:"cloudrun,omitempty"`
	GCF      *gcfEnvConfig      `yaml:"gcf,omitempty"`
}

type azureConfig struct {
	SubscriptionID string        `yaml:"subscription_id,omitempty"`
	ACA            *acaEnvConfig `yaml:"aca,omitempty"`
	AZF            *azfEnvConfig `yaml:"azf,omitempty"`
}

type ecsEnvConfig struct {
	Cluster          string   `yaml:"cluster,omitempty"`
	Subnets          []string `yaml:"subnets,omitempty"`
	SecurityGroups   []string `yaml:"security_groups,omitempty"`
	TaskRoleARN      string   `yaml:"task_role_arn,omitempty"`
	ExecutionRoleARN string   `yaml:"execution_role_arn,omitempty"`
	LogGroup         string   `yaml:"log_group,omitempty"`
	AssignPublicIP   bool     `yaml:"assign_public_ip,omitempty"`
	AgentEFSID       string   `yaml:"agent_efs_id,omitempty"`
}

type lambdaEnvConfig struct {
	RoleARN        string   `yaml:"role_arn,omitempty"`
	LogGroup       string   `yaml:"log_group,omitempty"`
	MemorySize     int      `yaml:"memory_size,omitempty"`
	Timeout        int      `yaml:"timeout,omitempty"`
	Subnets        []string `yaml:"subnets,omitempty"`
	SecurityGroups []string `yaml:"security_groups,omitempty"`
}

type cloudRunEnvConfig struct {
	Region       string `yaml:"region,omitempty"`
	VPCConnector string `yaml:"vpc_connector,omitempty"`
	LogID        string `yaml:"log_id,omitempty"`
	LogTimeout   string `yaml:"log_timeout,omitempty"`
}

type gcfEnvConfig struct {
	Region         string `yaml:"region,omitempty"`
	ServiceAccount string `yaml:"service_account,omitempty"`
	Timeout        int    `yaml:"timeout,omitempty"`
	Memory         string `yaml:"memory,omitempty"`
	CPU            string `yaml:"cpu,omitempty"`
	LogTimeout     string `yaml:"log_timeout,omitempty"`
}

type acaEnvConfig struct {
	ResourceGroup         string `yaml:"resource_group,omitempty"`
	Environment           string `yaml:"environment,omitempty"`
	Location              string `yaml:"location,omitempty"`
	LogAnalyticsWorkspace string `yaml:"log_analytics_workspace,omitempty"`
	StorageAccount        string `yaml:"storage_account,omitempty"`
}

type azfEnvConfig struct {
	ResourceGroup         string `yaml:"resource_group,omitempty"`
	Location              string `yaml:"location,omitempty"`
	StorageAccount        string `yaml:"storage_account,omitempty"`
	Registry              string `yaml:"registry,omitempty"`
	AppServicePlan        string `yaml:"app_service_plan,omitempty"`
	Timeout               int    `yaml:"timeout,omitempty"`
	LogAnalyticsWorkspace string `yaml:"log_analytics_workspace,omitempty"`
}

func configFilePath() string {
	if p := os.Getenv("SOCKERLESS_CONFIG"); p != "" {
		return p
	}
	return filepath.Join(sockerlessDir(), "config.yaml")
}

func loadConfigFile() (*unifiedConfig, error) {
	data, err := os.ReadFile(configFilePath())
	if err != nil {
		return nil, err
	}
	var cfg unifiedConfig
	if err := yaml.Unmarshal(data, &cfg); err != nil {
		return nil, fmt.Errorf("parse config: %w", err)
	}
	if cfg.Environments == nil {
		cfg.Environments = make(map[string]*environment)
	}
	if cfg.Simulators == nil {
		cfg.Simulators = make(map[string]*simulatorConfig)
	}
	return &cfg, nil
}

func saveConfigFile(cfg *unifiedConfig) error {
	data, err := yaml.Marshal(cfg)
	if err != nil {
		return fmt.Errorf("marshal config: %w", err)
	}
	path := configFilePath()
	if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
		return err
	}
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o644); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// configFileExists returns true if the unified config file exists.
func configFileExists() bool {
	_, err := os.Stat(configFilePath())
	return err == nil
}
