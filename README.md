# Sockerless

A Docker-compatible REST API daemon that executes containers on cloud serverless backends instead of a local Docker Engine. Standard Docker clients (`docker run`, Docker SDK, CI runners) connect to Sockerless exactly as they would to a real Docker daemon — but containers run on AWS ECS, Google Cloud Run, Azure Container Apps, and more.

## Why

No existing project fills this niche. Docker Engine, Podman, Colima, and Rancher Desktop all run containers locally. No cloud service exposes a Docker-compatible REST API. Sockerless bridges this gap: **Docker API on top, cloud serverless capacity on the bottom.**

Primary use cases:

- **CI runners** — GitLab Runner (docker-executor) and GitHub Actions Runner (container jobs) work without modification
- **Docker Compose** — `docker compose up/down/ps/logs` works for basic service stacks
- **General usage** — `docker run`, `docker exec`, `docker logs` against cloud infrastructure

## Architecture

```
Docker Client (CLI / SDK / CI Runner)
        │
        ▼
┌─────────────────────────────┐
│  Frontend                   │  Docker REST API v1.44
│  sockerless-frontend-docker │  Listens on :2375 or unix socket
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│  Backend                    │  Cloud-specific (pick one)
│  sockerless-backend-{name}  │  Listens on :9100
└──────────┬──────────────────┘
           │
           ▼
┌─────────────────────────────┐
│  Cloud Workload             │
│  ┌───────┐  ┌────────────┐  │
│  │ Agent │←→│ User Image │  │
│  └───────┘  └────────────┘  │
└─────────────────────────────┘
```

The **frontend** is a stateless Docker API translator. The **backend** manages cloud resources. The **agent** runs inside each workload as a sidecar and provides exec/attach over WebSocket.

## Backends

| Backend | Cloud | Type | Module |
|---------|-------|------|--------|
| `ecs` | AWS | Container (Fargate) | `backends/ecs/` |
| `lambda` | AWS | FaaS | `backends/lambda/` |
| `cloudrun` | GCP | Container (Cloud Run Jobs) | `backends/cloudrun/` |
| `gcf` | GCP | FaaS (Cloud Functions 2nd gen) | `backends/cloudrun-functions/` |
| `aca` | Azure | Container (Container Apps Jobs) | `backends/aca/` |
| `azf` | Azure | FaaS (Azure Functions) | `backends/azure-functions/` |
| `memory` | — | In-memory + WASM sandbox | `backends/memory/` |
| `docker` | — | Docker passthrough | `backends/docker/` |

Container backends inject the agent as a sidecar. FaaS backends bake the agent into the function image and use reverse WebSocket connections.

## Project Layout

```
api/                          Shared types and error definitions
agent/                        WebSocket agent (exec/attach inside workloads)
frontends/docker/             Docker REST API v1.44 frontend
sandbox/                      WASM sandbox (wazero + busybox + mvdan.cc/sh)
backends/
  core/                       Shared backend library (BaseServer, StateStore, handlers)
  memory/                     In-memory backend (WASM sandbox for real command execution)
  docker/                     Docker daemon passthrough
  ecs/                        AWS ECS Fargate
  lambda/                     AWS Lambda
  cloudrun/                   GCP Cloud Run Jobs
  cloudrun-functions/         GCP Cloud Functions
  aca/                        Azure Container Apps Jobs
  azure-functions/            Azure Functions
simulators/
  aws/                        AWS API simulator (ECS, ECR, IAM, VPC, EFS, Lambda, ...)
  gcp/                        GCP API simulator (Cloud Run, Compute, DNS, GCS, AR, ...)
  azure/                      Azure API simulator (ACA, ACR, Storage, Functions, ...)
terraform/
  modules/                    Terraform modules (one per backend)
  environments/               Terragrunt environments (live + simulator per backend)
tests/                        Integration tests (Docker SDK, 105 tests)
smoke-tests/                  Real CI runner validation (act + gitlab-runner)
spec/                         Specification documents
```

Each backend, the agent, the frontend, and the test suite are separate Go modules connected via `go.work`.

## Prerequisites

- Go 1.23+
- Docker (for smoke tests and terraform integration tests)

For terraform operations:
- Terraform >= 1.5
- Terragrunt >= 0.50

## Quick Start

```bash
# Build frontend + memory backend
go build -o sockerless-frontend-docker ./frontends/docker/cmd
go build -o sockerless-backend-memory  ./backends/memory/cmd/sockerless-backend-memory

# Start backend and frontend
./sockerless-backend-memory --addr :9100 &
./sockerless-frontend-docker --addr :2375 --backend http://localhost:9100 &

# Use with Docker CLI
export DOCKER_HOST=tcp://localhost:2375
docker version
docker run --rm alpine echo "hello from sockerless"   # real WASM execution
docker ps -a
```

## Development

### Running Tests

```bash
# Unit/integration tests against the memory backend (~2s)
make test

# Sandbox unit tests (WASM execution, shell, volumes)
cd sandbox && go test -v ./...

# Simulator integration tests — all 6 cloud backends (~170s)
# Builds simulators, starts them, runs backends against them
make sim-test-all

# Individual backend
make sim-test-ecs
make sim-test-lambda
make sim-test-cloudrun
make sim-test-gcf
make sim-test-aca
make sim-test-azf

# Per cloud
make sim-test-aws      # ECS + Lambda
make sim-test-gcp      # CloudRun + GCF
make sim-test-azure    # ACA + AZF
```

### Simulator Tests

Each simulator (`simulators/{aws,gcp,azure}/`) has its own test suite covering SDK, CLI, and Terraform compatibility:

```bash
cd simulators/aws && make test          # SDK + CLI + Terraform tests
cd simulators/aws && make docker-test   # Same, inside Docker (includes CLI)
```

### Smoke Tests

Validate that real, unmodified CI runners complete jobs through Sockerless:

```bash
# GitHub Actions (act)
make smoke-test-act              # Memory backend
make smoke-test-act-ecs          # ECS via simulator
make smoke-test-act-cloudrun     # Cloud Run via simulator
make smoke-test-act-aca          # ACA via simulator

# GitLab Runner
make smoke-test-gitlab           # Memory backend
make smoke-test-gitlab-ecs       # ECS via simulator
```

### Terraform Integration Tests

Run the full terraform modules against local simulators (Docker-only):

```bash
make tf-int-test-all     # All 6 backends (~10-15 min)
make tf-int-test-aws     # ECS (21 resources) + Lambda (5 resources)
make tf-int-test-gcp     # CloudRun (13) + GCF (7)
make tf-int-test-azure   # ACA (18) + AZF (11)
```

### Terraform

Infrastructure-as-code for all backends lives in `terraform/`. See [`terraform/README.md`](terraform/README.md) for details.

```bash
cd terraform
make fmt                  # Format .tf files
make validate             # Validate all modules
make plan-ecs-simulator   # Plan against local simulator
make apply-ecs-simulator  # Apply against local simulator
```

### Adding a New Backend

1. Create `backends/<name>/` as a new Go module
2. Import `backends/core` and implement a `RouteOverrides` struct — only override handlers that differ from the default memory-like behavior
3. Add a `cmd/sockerless-backend-<name>/main.go` entry point
4. Add the module to `go.work`
5. Add integration tests in `tests/`
6. Add a simulator in `simulators/` if targeting a new cloud
7. Add a terraform module in `terraform/modules/`

## Deploying to Cloud

See [`DEPLOYMENT.md`](DEPLOYMENT.md) for step-by-step instructions covering all 6 backends across AWS, GCP, and Azure — including terraform state bootstrap, image push, environment variable mapping, validation, and cost estimates.

## Documentation

| Document | Description |
|----------|-------------|
| [`spec/SOCKERLESS_SPEC.md`](spec/SOCKERLESS_SPEC.md) | Full specification (API surface, architecture, protocols) |
| [`DEPLOYMENT.md`](DEPLOYMENT.md) | Cloud deployment guide |
| [`terraform/README.md`](terraform/README.md) | Terraform module and environment reference |
| [`PLAN.md`](PLAN.md) | Implementation plan and task tracking |
| [`STATUS.md`](STATUS.md) | Project status and phase history |
