# Simulators

In-memory cloud API simulators for local development and testing. Each simulator implements the subset of a cloud provider's APIs used by the Sockerless backends, allowing the full system to run without real cloud credentials or infrastructure.

## Overview

| Simulator | Default Port | Services |
|-----------|-------------|----------|
| [aws](aws/) | `:4566` | ECS, ECR, Lambda, EC2, S3, IAM, STS, CloudWatch Logs, EFS, Cloud Map |
| [gcp](gcp/) | `:4567` | Cloud Run Jobs, Cloud Functions, GCS, Artifact Registry, Cloud DNS, Cloud Logging, Compute, IAM, VPC Access, Service Usage |
| [azure](azure/) | `:4568` | Container Apps, Azure Functions, ACR, Storage, Monitor, App Insights, Private DNS, Network, Managed Identity, Authorization |

All simulators share a common framework (`shared/`) providing HTTP server setup, middleware (request ID, logging, auth passthrough), thread-safe state management, and provider-specific routing/error formatting.

## Running

### Standalone

```sh
cd simulators/aws && go run .
cd simulators/gcp && go run .
cd simulators/azure && go run .
```

### Docker Compose

```sh
cd simulators
docker compose up
```

This starts all three simulators on their default ports with health checks.

### Environment variables

| Variable | Default | Description |
|----------|---------|-------------|
| `SIM_LISTEN_ADDR` | `:8443` | Listen address (overridden per-provider) |
| `SIM_AWS_PORT` / `SIM_GCP_PORT` / `SIM_AZURE_PORT` | | Provider-specific port override |
| `SIM_TLS_CERT` | | TLS certificate file (enables HTTPS) |
| `SIM_TLS_KEY` | | TLS private key file |
| `SIM_LOG_LEVEL` | `info` | Log level: trace, debug, info, warn, error |

## Testing

Each simulator has three test suites in separate Go modules:

- **sdk-tests/** — Uses the official cloud SDK to exercise API endpoints
- **cli-tests/** — Uses the cloud CLI (aws/gcloud/az) to exercise endpoints
- **terraform-tests/** — Runs `terraform init/apply/destroy` against the simulator

```sh
# Run all test types for a single cloud
cd simulators/aws/sdk-tests && go test -v ./...
cd simulators/aws/cli-tests && go test -v ./...
cd simulators/aws/terraform-tests && go test -v ./...
```

Or use the top-level Makefile:

```sh
make docker-test         # Docker-based tests for all clouds
make sim-test-all        # Simulator-backend integration tests
```

## Test counts

| Cloud | SDK tests | CLI tests | Terraform tests |
|-------|-----------|-----------|-----------------|
| AWS | 17 | 21 | ~30s apply/destroy |
| GCP | 20 | 15 | ~5s apply/destroy |
| Azure | 13 | 14 | ~1s apply/destroy |

## Shared framework

The `shared/` directory (vendored into each simulator as a Go module replace) provides:

- **server.go** — HTTP server with health check, graceful shutdown, optional TLS
- **middleware.go** — Request ID, structured logging, auth passthrough (extracts identity from SigV4/Bearer tokens)
- **router.go** — Provider-specific routing: `AWSRouter` (X-Amz-Target header), `AWSQueryRouter` (Action parameter), `GCPRouter`/`AzureRouter` (path-based)
- **state.go** — Generic `StateStore[T]` with thread-safe CRUD operations
- **errors.go** — Error response formatting per provider (AWS JSON, EC2 XML, S3 XML, GCP JSON, Azure JSON)
- **config.go** — Environment variable configuration loading

## Guides

Each simulator has usage guides for the official cloud tools:

| Cloud | CLI | Terraform | Python SDK |
|-------|-----|-----------|------------|
| AWS | [AWS CLI](aws/docs/cli.md) | [hashicorp/aws](aws/docs/terraform.md) | [boto3](aws/docs/python-sdk.md) |
| GCP | [gcloud CLI](gcp/docs/cli.md) | [hashicorp/google](gcp/docs/terraform.md) | [google-cloud-*](gcp/docs/python-sdk.md) |
| Azure | [az CLI](azure/docs/cli.md) | [hashicorp/azurestack](azure/docs/terraform.md) | [azure-mgmt-*](azure/docs/python-sdk.md) |

## Agent integration

The ECS, Lambda, Cloud Run Jobs, Cloud Functions, Container Apps, and Azure Functions simulators support agent process management. When a container/function/job is started with a `SOCKERLESS_AGENT_CALLBACK_URL` environment variable, the simulator spawns a `sockerless-agent` subprocess in reverse-connect mode, enabling full exec/attach through the simulated cloud resources.
