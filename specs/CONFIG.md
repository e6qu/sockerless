# Configuration Specification

Sockerless supports two configuration methods: a unified YAML config file and per-backend environment variables. The config file takes precedence when present.

## Unified Config File

**Default path:** `~/.sockerless/config.yaml` (override with `$SOCKERLESS_CONFIG`)

**Active context:** `~/.sockerless/active` contains the name of the active environment (override with `$SOCKERLESS_CONTEXT`)

```yaml
simulators:
  local-aws:
    cloud: aws
    port: 4566
    grpcPort: 0        # optional
    logLevel: info     # optional

environments:
  dev:
    backend: ecs                    # required: ecs|lambda|cloudrun|cloudrun-functions|aca|azure-functions|docker
    addr: ":9100"                   # optional listen address
    logLevel: debug                 # optional
    simulator: local-aws            # optional reference to simulator
    common:
      agentImage: sockerless/agent:latest
      agentToken: ""
      callbackURL: ""
      endpointURL: ""              # auto-set to http://localhost:{port} when simulator has port
      pollInterval: "2s"
      agentTimeout: "30s"
    aws:
      region: eu-west-1
      ecs:
        cluster: sockerless-live
        subnets: [subnet-abc, subnet-def]
        securityGroups: [sg-123]
        taskRoleARN: arn:aws:iam::123:role/task
        executionRoleARN: arn:aws:iam::123:role/exec
        logGroup: /sockerless/live/containers
        assignPublicIP: true
        agentEFSID: fs-abc123
      lambda:
        roleARN: arn:aws:iam::123:role/lambda
        logGroup: /sockerless/lambda
        memorySize: 1024
        timeout: 900
        subnetIDs: []
        securityGroupIDs: []
    gcp:
      project: my-project
      cloudrun:
        region: us-central1
        vpcConnector: ""
        logID: sockerless
        logTimeout: "30s"
      gcf:
        region: us-central1
        serviceAccount: ""
        timeout: 3600
        memory: "1Gi"
        cpu: "1"
        logTimeout: "30s"
    azure:
      subscriptionID: sub-123
      aca:
        resourceGroup: rg-sockerless
        environment: sockerless
        location: eastus
        logAnalyticsWorkspace: ""
        storageAccount: ""
      azf:
        resourceGroup: rg-sockerless
        location: eastus
        storageAccount: sa-sockerless
        registry: ""
        appServicePlan: ""
        logAnalyticsWorkspace: ""
        timeout: 600
```

### Config Loading

Source: `backends/core/configfile.go`

1. `LoadConfigFile(path)` — reads YAML, unmarshals to `UnifiedConfig`
2. `ActiveEnvironment()` — reads active context name, returns `*Environment`
3. `ResolveSimulator(env)` — returns `*SimulatorConfig` referenced by environment
4. `Validate()` — checks required fields, verifies simulator references
5. `Save()` — atomic write with file locking (`syscall.Flock`)

### Config Precedence

In `cmd/sockerless-backend-ecs/main.go` (same pattern for all backends):

```go
if cfg, env, _, err := core.ActiveEnvironmentWithConfig(); err == nil {
    sim, _ := cfg.ResolveSimulator(env)
    config = backend.ConfigFromEnvironment(env, sim)
} else {
    core.LoadContextEnv(logger)
    config = backend.ConfigFromEnv()
}
```

Config file is tried first. If unavailable, falls back to environment variables.

## Per-Backend Environment Variables

### Common (all backends)

| Variable | Default | Description |
|----------|---------|-------------|
| `SOCKERLESS_CALLBACK_URL` | | Backend URL for reverse agent connections |
| `SOCKERLESS_ENDPOINT_URL` | | Custom endpoint URL (simulator mode) |
| `SOCKERLESS_POLL_INTERVAL` | `2s` | Cloud API poll interval |
| `SOCKERLESS_AGENT_TIMEOUT` | `30s` | Agent health check timeout |

### ECS

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `AWS_REGION` | `us-east-1` | | AWS region |
| `SOCKERLESS_ECS_CLUSTER` | `sockerless` | | ECS cluster name |
| `SOCKERLESS_ECS_SUBNETS` | | **yes** | Comma-separated subnet IDs |
| `SOCKERLESS_ECS_SECURITY_GROUPS` | | | Comma-separated SG IDs |
| `SOCKERLESS_ECS_TASK_ROLE_ARN` | | | IAM task role ARN |
| `SOCKERLESS_ECS_EXECUTION_ROLE_ARN` | | **yes** | IAM execution role ARN |
| `SOCKERLESS_ECS_LOG_GROUP` | `/sockerless` | | CloudWatch log group |
| `SOCKERLESS_AGENT_IMAGE` | `sockerless/agent:latest` | | Agent sidecar image |
| `SOCKERLESS_AGENT_EFS_ID` | | | EFS filesystem for agent binary |
| `SOCKERLESS_AGENT_TOKEN` | | | Agent auth token |
| `SOCKERLESS_ECS_PUBLIC_IP` | `false` | | Assign public IP to tasks |

### Lambda

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `AWS_REGION` | `us-east-1` | | AWS region |
| `SOCKERLESS_LAMBDA_ROLE_ARN` | | **yes** | Lambda execution role ARN |
| `SOCKERLESS_LAMBDA_LOG_GROUP` | `/sockerless/lambda` | | CloudWatch log group |
| `SOCKERLESS_LAMBDA_MEMORY_SIZE` | `1024` | | Lambda memory (MB) |
| `SOCKERLESS_LAMBDA_TIMEOUT` | `900` | | Lambda timeout (seconds) |
| `SOCKERLESS_LAMBDA_SUBNETS` | | | VPC subnet IDs (CSV) |
| `SOCKERLESS_LAMBDA_SECURITY_GROUPS` | | | VPC security group IDs (CSV) |

### Cloud Run

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SOCKERLESS_GCR_PROJECT` | | **yes** | GCP project ID |
| `SOCKERLESS_GCR_REGION` | `us-central1` | | GCP region |
| `SOCKERLESS_GCR_VPC_CONNECTOR` | | | VPC connector name |
| `SOCKERLESS_GCR_LOG_ID` | `sockerless` | | Cloud Logging log ID |
| `SOCKERLESS_GCR_AGENT_IMAGE` | `sockerless/agent:latest` | | Agent image |
| `SOCKERLESS_GCR_AGENT_TOKEN` | | | Agent auth token |
| `SOCKERLESS_LOG_TIMEOUT` | `30s` | | Log fetch timeout |

### Cloud Run Functions

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SOCKERLESS_GCF_PROJECT` | | **yes** | GCP project ID |
| `SOCKERLESS_GCF_REGION` | `us-central1` | | GCP region |
| `SOCKERLESS_GCF_SERVICE_ACCOUNT` | | | GCP service account |
| `SOCKERLESS_GCF_TIMEOUT` | `3600` | | Function timeout (seconds) |
| `SOCKERLESS_GCF_MEMORY` | `1Gi` | | Function memory |
| `SOCKERLESS_GCF_CPU` | `1` | | Function CPU |
| `SOCKERLESS_LOG_TIMEOUT` | `30s` | | Log fetch timeout |

### Azure Container Apps

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SOCKERLESS_ACA_SUBSCRIPTION_ID` | | **yes** | Azure subscription |
| `SOCKERLESS_ACA_RESOURCE_GROUP` | | **yes** | Azure resource group |
| `SOCKERLESS_ACA_ENVIRONMENT` | `sockerless` | | ACA environment name |
| `SOCKERLESS_ACA_LOCATION` | `eastus` | | Azure region |
| `SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE` | | | Log Analytics workspace |
| `SOCKERLESS_ACA_STORAGE_ACCOUNT` | | | Storage account for volumes |
| `SOCKERLESS_ACA_AGENT_IMAGE` | `sockerless/agent:latest` | | Agent image |
| `SOCKERLESS_ACA_AGENT_TOKEN` | | | Agent auth token |

### Azure Functions

| Variable | Default | Required | Description |
|----------|---------|----------|-------------|
| `SOCKERLESS_AZF_SUBSCRIPTION_ID` | | **yes** | Azure subscription |
| `SOCKERLESS_AZF_RESOURCE_GROUP` | | **yes** | Azure resource group |
| `SOCKERLESS_AZF_LOCATION` | `eastus` | | Azure region |
| `SOCKERLESS_AZF_STORAGE_ACCOUNT` | | **yes** | Storage account |
| `SOCKERLESS_AZF_REGISTRY` | | | Container registry |
| `SOCKERLESS_AZF_APP_SERVICE_PLAN` | | | App Service plan |
| `SOCKERLESS_AZF_TIMEOUT` | `600` | | Function timeout (seconds) |
| `SOCKERLESS_AZF_LOG_ANALYTICS_WORKSPACE` | | | Log Analytics workspace |

### Docker

No Sockerless-specific env vars. Uses standard Docker environment (`DOCKER_HOST`, etc.).

## Validation

Each backend's `Config.Validate()` checks required fields:

| Backend | Required |
|---------|----------|
| ECS | cluster, subnets (≥1), execution_role_arn |
| Lambda | role_arn |
| Cloud Run | project |
| Cloud Run Functions | project |
| ACA | subscription_id, resource_group |
| Azure Functions | subscription_id, resource_group, storage_account |
| Docker | (none) |
