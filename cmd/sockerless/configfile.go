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
	Login     *loginConfig `yaml:"login,omitempty"`
	AWS       *awsConfig   `yaml:"aws,omitempty"`
	GCP       *gcpConfig   `yaml:"gcp,omitempty"`
	Azure     *azureConfig `yaml:"azure,omitempty"`
	Common    commonConfig `yaml:"common,omitempty"`
}

// loginConfig holds the Shauth OpenID Connect coordinates `sockerless login`
// signs in against. client_secret stays empty for a public client
// (token_endpoint_auth_method "none"); when the deployment registered the CLI
// as a confidential client the secret is a coordinate like the rest.
type loginConfig struct {
	Issuer       string `yaml:"issuer,omitempty"`
	ClientID     string `yaml:"client_id,omitempty"`
	ClientSecret string `yaml:"client_secret,omitempty"`
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
	Login  *awsLoginConfig  `yaml:"login,omitempty"`
	ECS    *ecsEnvConfig    `yaml:"ecs,omitempty"`
	Lambda *lambdaEnvConfig `yaml:"lambda,omitempty"`
}

type gcpConfig struct {
	Project       string             `yaml:"project,omitempty"`
	BuildBucket   string             `yaml:"build_bucket,omitempty"`
	BuildPlatform string             `yaml:"build_platform,omitempty"`
	Login         *gcpLoginConfig    `yaml:"login,omitempty"`
	CloudRun      *cloudRunEnvConfig `yaml:"cloudrun,omitempty"`
	GCF           *gcfEnvConfig      `yaml:"gcf,omitempty"`
}

type azureConfig struct {
	SubscriptionID string            `yaml:"subscription_id,omitempty"`
	Login          *azureLoginConfig `yaml:"login,omitempty"`
	ACA            *acaEnvConfig     `yaml:"aca,omitempty"`
	AZF            *azfEnvConfig     `yaml:"azf,omitempty"`
}

// awsLoginConfig wires the Shauth sign-in to Amazon Web Services: the CLI
// writes an `~/.aws/config` profile whose role_arn + web_identity_token_file
// make the aws CLI and SDKs run AssumeRoleWithWebIdentity themselves. The
// region comes from awsConfig.Region. endpoint_url points the profile at a
// deployed simulator; empty targets real AWS.
type awsLoginConfig struct {
	RoleARN     string `yaml:"role_arn,omitempty"`
	EndpointURL string `yaml:"endpoint_url,omitempty"`
	Profile     string `yaml:"profile,omitempty"`
}

// gcpLoginConfig wires the Shauth sign-in to Google Cloud Workforce Identity
// Federation: the CLI writes an external_account Application Default
// Credentials file naming the workforce pool provider audience and the
// Security Token Service coordinate. sts_endpoint and api_endpoint default to
// real Google (https://sts.googleapis.com and no api_endpoint_overrides).
type gcpLoginConfig struct {
	WorkforceAudience        string `yaml:"workforce_audience,omitempty"`
	STSEndpoint              string `yaml:"sts_endpoint,omitempty"`
	APIEndpoint              string `yaml:"api_endpoint,omitempty"`
	WorkforcePoolUserProject string `yaml:"workforce_pool_user_project,omitempty"`
	Configuration            string `yaml:"configuration,omitempty"`
}

// azureLoginConfig wires the Shauth sign-in to Microsoft Entra Workload
// Identity Federation: the CLI runs `az login --service-principal
// --federated-token` for the federation client. authority_endpoint and
// resource_manager_endpoint default to the Azure public cloud; set both to
// target a deployed simulator (the az CLI requires HTTPS coordinates, so a
// simulator target serves TLS and ca_bundle names the trust bundle az reads
// through REQUESTS_CA_BUNDLE).
type azureLoginConfig struct {
	Tenant                  string `yaml:"tenant,omitempty"`
	ClientID                string `yaml:"client_id,omitempty"`
	AuthorityEndpoint       string `yaml:"authority_endpoint,omitempty"`
	ResourceManagerEndpoint string `yaml:"resource_manager_endpoint,omitempty"`
	CABundle                string `yaml:"ca_bundle,omitempty"`
	CloudName               string `yaml:"cloud_name,omitempty"`
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
	// 0600: the unified config embeds the agent token (commonConfig.AgentToken).
	// On a shared host a 0644 file lets any local user read another's token.
	tmp := path + ".tmp"
	if err := os.WriteFile(tmp, data, 0o600); err != nil {
		return err
	}
	return os.Rename(tmp, path)
}

// configFileExists returns true if the unified config file exists.
func configFileExists() bool {
	_, err := os.Stat(configFilePath())
	return err == nil
}
