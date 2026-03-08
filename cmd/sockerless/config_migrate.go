package main

import (
	"encoding/json"
	"flag"
	"fmt"
	"os"
	"path/filepath"
	"strings"

	"gopkg.in/yaml.v3"
)

func configMigrate(args []string) {
	fs := flag.NewFlagSet("config migrate", flag.ExitOnError)
	write := fs.Bool("write", false, "write config.yaml (default: print to stdout)")
	fs.Parse(args)

	contextsDir := filepath.Join(sockerlessDir(), "contexts")
	entries, err := os.ReadDir(contextsDir)
	if err != nil {
		if os.IsNotExist(err) {
			fmt.Fprintln(os.Stderr, "No contexts directory found. Nothing to migrate.")
			os.Exit(0)
		}
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	cfg := &unifiedConfig{
		Simulators:   make(map[string]*simulatorConfig),
		Environments: make(map[string]*environment),
	}

	for _, e := range entries {
		if !e.IsDir() {
			continue
		}
		name := e.Name()
		data, err := os.ReadFile(filepath.Join(contextsDir, name, "config.json"))
		if err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %q: %v\n", name, err)
			continue
		}
		var ctx contextConfig
		if err := json.Unmarshal(data, &ctx); err != nil {
			fmt.Fprintf(os.Stderr, "warning: skipping %q: %v\n", name, err)
			continue
		}

		env := migrateContext(name, &ctx, cfg)
		cfg.Environments[name] = env
	}

	if len(cfg.Environments) == 0 {
		fmt.Fprintln(os.Stderr, "No contexts found to migrate.")
		os.Exit(0)
	}

	data, err := yaml.Marshal(cfg)
	if err != nil {
		fmt.Fprintf(os.Stderr, "error: %v\n", err)
		os.Exit(1)
	}

	if *write {
		path := configFilePath()
		if err := os.MkdirAll(filepath.Dir(path), 0o755); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		if err := os.WriteFile(path, data, 0o644); err != nil {
			fmt.Fprintf(os.Stderr, "error: %v\n", err)
			os.Exit(1)
		}
		fmt.Fprintf(os.Stderr, "Wrote %s (%d environments)\n", path, len(cfg.Environments))
	} else {
		fmt.Print(string(data))
	}
}

func migrateContext(name string, ctx *contextConfig, cfg *unifiedConfig) *environment {
	env := &environment{
		Backend: ctx.Backend,
		Addr:    ctx.Addr,
	}

	// Detect simulator usage from SOCKERLESS_ENDPOINT_URL
	if endpoint := ctx.Env["SOCKERLESS_ENDPOINT_URL"]; endpoint != "" {
		cloud := inferCloud(ctx.Backend)
		if cloud != "" {
			simName := "sim-" + cloud
			if _, ok := cfg.Simulators[simName]; !ok {
				cfg.Simulators[simName] = &simulatorConfig{Cloud: cloud}
			}
			env.Simulator = simName
		}
	}

	// Map env vars to structured config
	switch ctx.Backend {
	case "ecs":
		env.AWS = migrateAWSECS(ctx.Env)
		env.Common = migrateCommonConfig(ctx.Env)
	case "lambda":
		env.AWS = migrateAWSLambda(ctx.Env)
		env.Common = migrateCommonConfig(ctx.Env)
	case "cloudrun":
		env.GCP = migrateGCPCloudRun(ctx.Env)
		env.Common = migrateCommonConfig(ctx.Env)
	case "gcf":
		env.GCP = migrateGCPGCF(ctx.Env)
		env.Common = migrateCommonConfig(ctx.Env)
	case "aca":
		env.Azure = migrateAzureACA(ctx.Env)
		env.Common = migrateCommonConfig(ctx.Env)
	case "azf":
		env.Azure = migrateAzureAZF(ctx.Env)
		env.Common = migrateCommonConfig(ctx.Env)
	}

	return env
}

func inferCloud(backend string) string {
	switch backend {
	case "ecs", "lambda":
		return "aws"
	case "cloudrun", "gcf":
		return "gcp"
	case "aca", "azf":
		return "azure"
	}
	return ""
}

func migrateCommonConfig(envVars map[string]string) commonConfig {
	return commonConfig{
		AgentImage:   envVars["SOCKERLESS_AGENT_IMAGE"],
		AgentToken:   envVars["SOCKERLESS_AGENT_TOKEN"],
		CallbackURL:  envVars["SOCKERLESS_CALLBACK_URL"],
		EndpointURL:  envVars["SOCKERLESS_ENDPOINT_URL"],
		PollInterval: envVars["SOCKERLESS_POLL_INTERVAL"],
		AgentTimeout: envVars["SOCKERLESS_AGENT_TIMEOUT"],
	}
}

func migrateAWSECS(envVars map[string]string) *awsConfig {
	return &awsConfig{
		Region: envVars["AWS_REGION"],
		ECS: &ecsEnvConfig{
			Cluster:          envVars["SOCKERLESS_ECS_CLUSTER"],
			Subnets:          splitCSV(envVars["SOCKERLESS_ECS_SUBNETS"]),
			SecurityGroups:   splitCSV(envVars["SOCKERLESS_ECS_SECURITY_GROUPS"]),
			TaskRoleARN:      envVars["SOCKERLESS_ECS_TASK_ROLE_ARN"],
			ExecutionRoleARN: envVars["SOCKERLESS_ECS_EXECUTION_ROLE_ARN"],
			LogGroup:         envVars["SOCKERLESS_ECS_LOG_GROUP"],
			AssignPublicIP:   envVars["SOCKERLESS_ECS_PUBLIC_IP"] == "true",
			AgentEFSID:       envVars["SOCKERLESS_AGENT_EFS_ID"],
		},
	}
}

func migrateAWSLambda(envVars map[string]string) *awsConfig {
	return &awsConfig{
		Region: envVars["AWS_REGION"],
		Lambda: &lambdaEnvConfig{
			RoleARN:        envVars["SOCKERLESS_LAMBDA_ROLE_ARN"],
			LogGroup:       envVars["SOCKERLESS_LAMBDA_LOG_GROUP"],
			MemorySize:     atoiOr(envVars["SOCKERLESS_LAMBDA_MEMORY_SIZE"], 0),
			Timeout:        atoiOr(envVars["SOCKERLESS_LAMBDA_TIMEOUT"], 0),
			Subnets:        splitCSV(envVars["SOCKERLESS_LAMBDA_SUBNETS"]),
			SecurityGroups: splitCSV(envVars["SOCKERLESS_LAMBDA_SECURITY_GROUPS"]),
		},
	}
}

func migrateGCPCloudRun(envVars map[string]string) *gcpConfig {
	return &gcpConfig{
		Project: envVars["SOCKERLESS_GCR_PROJECT"],
		CloudRun: &cloudRunEnvConfig{
			Region:       envVars["SOCKERLESS_GCR_REGION"],
			VPCConnector: envVars["SOCKERLESS_GCR_VPC_CONNECTOR"],
			LogID:        envVars["SOCKERLESS_GCR_LOG_ID"],
			LogTimeout:   envVars["SOCKERLESS_LOG_TIMEOUT"],
		},
	}
}

func migrateGCPGCF(envVars map[string]string) *gcpConfig {
	return &gcpConfig{
		Project: envVars["SOCKERLESS_GCF_PROJECT"],
		GCF: &gcfEnvConfig{
			Region:         envVars["SOCKERLESS_GCF_REGION"],
			ServiceAccount: envVars["SOCKERLESS_GCF_SERVICE_ACCOUNT"],
			Timeout:        atoiOr(envVars["SOCKERLESS_GCF_TIMEOUT"], 0),
			Memory:         envVars["SOCKERLESS_GCF_MEMORY"],
			CPU:            envVars["SOCKERLESS_GCF_CPU"],
			LogTimeout:     envVars["SOCKERLESS_LOG_TIMEOUT"],
		},
	}
}

func migrateAzureACA(envVars map[string]string) *azureConfig {
	return &azureConfig{
		SubscriptionID: envVars["SOCKERLESS_ACA_SUBSCRIPTION_ID"],
		ACA: &acaEnvConfig{
			ResourceGroup:         envVars["SOCKERLESS_ACA_RESOURCE_GROUP"],
			Environment:           envVars["SOCKERLESS_ACA_ENVIRONMENT"],
			Location:              envVars["SOCKERLESS_ACA_LOCATION"],
			LogAnalyticsWorkspace: envVars["SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE"],
			StorageAccount:        envVars["SOCKERLESS_ACA_STORAGE_ACCOUNT"],
		},
	}
}

func migrateAzureAZF(envVars map[string]string) *azureConfig {
	return &azureConfig{
		SubscriptionID: envVars["SOCKERLESS_AZF_SUBSCRIPTION_ID"],
		AZF: &azfEnvConfig{
			ResourceGroup:         envVars["SOCKERLESS_AZF_RESOURCE_GROUP"],
			Location:              envVars["SOCKERLESS_AZF_LOCATION"],
			StorageAccount:        envVars["SOCKERLESS_AZF_STORAGE_ACCOUNT"],
			Registry:              envVars["SOCKERLESS_AZF_REGISTRY"],
			AppServicePlan:        envVars["SOCKERLESS_AZF_APP_SERVICE_PLAN"],
			Timeout:               atoiOr(envVars["SOCKERLESS_AZF_TIMEOUT"], 0),
			LogAnalyticsWorkspace: envVars["SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE"],
		},
	}
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

func atoiOr(s string, def int) int {
	if s == "" {
		return def
	}
	n := 0
	for _, c := range s {
		if c < '0' || c > '9' {
			return def
		}
		n = n*10 + int(c-'0')
	}
	return n
}
