# Simulators

Local reimplementations of cloud provider APIs. Each simulator implements the subset of a cloud provider's services used by the Sockerless backends, providing real execution semantics — not mocks or stubs. Jobs run, functions execute, timeouts fire, and logs are produced, all driven by the same configuration knobs (replica timeouts, task timeouts, etc.) that the real cloud services honor.

The goal is full behavioral fidelity: code that works against the simulators works against the real cloud, and vice versa. Simulators run locally first, with the architecture designed to eventually distribute across multiple machines.

## Overview

| Simulator | Default Port | Services |
|-----------|-------------|----------|
| [aws](aws/) | `:4566` | ECS, ECR, Lambda, EC2, S3, IAM, STS, CloudWatch Logs, EFS, Cloud Map |
| [gcp](gcp/) | `:4567` | Cloud Run Jobs, Cloud Functions, GCS, Artifact Registry, Cloud DNS, Cloud Logging, Compute, IAM, VPC Access, Service Usage |
| [azure](azure/) | `:4568` | Container Apps, Azure Functions, ACR, Storage, Monitor, App Insights, Private DNS, Network, Managed Identity, Authorization |

All simulators share a common framework (`shared/`) providing HTTP server setup, middleware (request ID, logging, auth passthrough), thread-safe state management, and provider-specific routing/error formatting.

See [STATUS.md](../STATUS.md) for project-wide test results.

## Design philosophy

Simulators are **real implementations**, not fakes. They don't approximate cloud behavior with synthetic timers or hardcoded responses — they reimplement the actual service semantics:

- **Execution lifecycle** is driven by cloud-native configuration. Azure ACA jobs respect `replicaTimeout`. GCP Cloud Run jobs respect the task template `timeout`. AWS ECS tasks run until the process exits or `StopTask` is called, because ECS has no native execution timeout.
- **Log injection** writes entries to the same tables and log groups that the real services would, queryable through the same APIs (KQL for Azure, Cloud Logging filters for GCP, CloudWatch for AWS).
- **Agent integration** spawns real subprocesses — the same `sockerless-agent` binary used in production — enabling full exec/attach through simulated cloud resources.
- **SDK and Terraform compatibility** is tested with the real official clients, not custom HTTP calls. If the Azure SDK expects a 200 for a sync create, the simulator returns 200. If the GCP SDK expects an LRO wrapper, the simulator returns one.

The simulators run locally on a single machine today. The architecture is designed so they can eventually run distributed across multiple machines, with the same API surface.

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

Each simulator has four test suites:

- **sdk-tests/** — Uses the official cloud SDK (Go) to exercise API endpoints
- **cli-tests/** — Uses the cloud CLI (aws/gcloud/az) via Go test harness
- **terraform-tests/** — Runs `terraform init/apply/destroy` against the simulator
- **bash-tests/** — Standalone bash scripts testing the cloud CLI in both text and JSON output modes

```sh
# Run all test types for a single cloud
cd simulators/aws/sdk-tests && GOWORK=off go test -v ./...
cd simulators/aws/cli-tests && GOWORK=off go test -v ./...
cd simulators/aws/terraform-tests && GOWORK=off go test -v ./...
cd simulators/aws/bash-tests && ./test_aws_cli.sh
```

Or use the top-level Makefile:

```sh
make docker-test         # Docker-based tests for all clouds
make sim-test-all        # Simulator-backend integration tests
```

## Test counts

| Cloud | SDK tests | CLI tests | Bash tests | Terraform tests |
|-------|-----------|-----------|------------|-----------------|
| AWS | 46 | 26 | 61 | ~30s apply/destroy |
| GCP | 36 | 21 | 33 | ~5s apply/destroy |
| Azure | 48 | 19 | 42 | ~1s apply/destroy |

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

## Architecture

### State management

All simulators store resource state in generic `StateStore[T]` instances — thread-safe, in-memory maps with CRUD and filter operations. Each service (ECS, Cloud Run, Container Apps, etc.) maintains its own set of stores. State resets on simulator restart; there is no persistent storage.

Example store hierarchy for AWS:
- `clusters: StateStore[ECSCluster]`
- `taskDefs: StateStore[ECSTaskDefinition]`
- `tasks: StateStore[ECSTask]`
- `logGroups: StateStore[LogGroup]`

### Process execution

The container-oriented services (ECS, Cloud Run Jobs, Container Apps) execute real OS processes from the container `command`/`entrypoint` fields via the shared `sim.StartProcess()` helper. Process stdout/stderr is captured in real time and injected into the cloud-native log sink:

| Service | Log sink | API for retrieval |
|---------|----------|-------------------|
| ECS | CloudWatch Logs (awslogs) | `GetLogEvents` / `FilterLogEvents` |
| Cloud Run Jobs | Cloud Logging | `entries.list` (REST) / `ListLogEntries` (gRPC) |
| Container Apps | Log Analytics | KQL via `QueryWorkspace` |
| Lambda | CloudWatch Logs | `GetLogEvents` |
| Cloud Functions | Cloud Logging | `entries.list` / `ListLogEntries` |
| Azure Functions | Log Analytics (AppTraces) | KQL via `QueryWorkspace` |

FaaS simulators (Lambda, Cloud Functions, Azure Functions) also execute real processes when `SimCommand` is set, returning the result synchronously.

### ECS ExecuteCommand

The ECS simulator supports `ExecuteCommand` with WebSocket-based session bridging. When `ExecuteCommand` is called on a running task, the simulator:
1. Spawns a new process with the given command
2. Registers a WebSocket handler at `/ecs-exec/{sessionId}`
3. Returns a session with the WebSocket URL
4. Bridges stdin/stdout/stderr over the WebSocket connection

### Request routing

Each cloud uses its native protocol conventions:

| Cloud | Protocol | Routing |
|-------|----------|---------|
| AWS (ECS, ECR, CloudWatch, Cloud Map) | JSON | `X-Amz-Target` header dispatch |
| AWS (EC2, IAM, STS) | Query | `Action` form parameter dispatch |
| AWS (Lambda, S3, EFS) | REST | Path-based mux |
| GCP (all services) | REST + gRPC | Path-based mux (HTTP), proto service (gRPC on port+1) |
| Azure (all services) | ARM REST | Path-based mux with `api-version` validation |
