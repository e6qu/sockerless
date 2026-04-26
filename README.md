# Sockerless

[![CI](https://github.com/e6qu/sockerless/actions/workflows/ci.yml/badge.svg)](https://github.com/e6qu/sockerless/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Docker API](https://img.shields.io/badge/Docker_API-v1.44-2496ED?logo=docker&logoColor=white)](https://docs.docker.com/engine/api/v1.44/)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![AWS](https://img.shields.io/badge/AWS-ECS_|_Lambda-FF9900?logo=amazonwebservices&logoColor=white)](#backends)
[![GCP](https://img.shields.io/badge/GCP-Cloud_Run_|_GCF-4285F4?logo=googlecloud&logoColor=white)](#backends)
[![Azure](https://img.shields.io/badge/Azure-ACA_|_AZF-0078D4?logo=microsoftazure&logoColor=white)](#backends)

[![Go](https://img.shields.io/badge/Go-90.4k_lines-00ADD8?logo=go&logoColor=white)](#module-sizes)
[![TypeScript](https://img.shields.io/badge/TypeScript-8.5k_lines-3178C6?logo=typescript&logoColor=white)](#module-sizes)
[![Tests](https://img.shields.io/badge/Tests-47.6k_lines-brightgreen)](#module-sizes)
[![Coverage](https://img.shields.io/badge/Core_Coverage-40%25-yellow)](#module-sizes)
[![Modules](https://img.shields.io/badge/Go_Modules-34-informational)](#module-sizes)

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
│  Sockerless Backend         │  Docker REST API v1.44
│  sockerless-backend-{name}  │  Listens on :3375 or unix socket
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

Each backend is a single binary that serves the Docker REST API v1.44 and manages cloud resources. The **agent** runs inside each workload as a sidecar and provides exec/attach over WebSocket. See [ARCHITECTURE.md](ARCHITECTURE.md) for detailed diagrams and component descriptions.

## Backends

| Backend | Cloud | Type | Module |
|---------|-------|------|--------|
| `ecs` | AWS | Container (Fargate) | `backends/ecs/` |
| `lambda` | AWS | FaaS | `backends/lambda/` |
| `cloudrun` | GCP | Container (Cloud Run Jobs) | `backends/cloudrun/` |
| `gcf` | GCP | FaaS (Cloud Functions 2nd gen) | `backends/cloudrun-functions/` |
| `aca` | Azure | Container (Container Apps Jobs) | `backends/aca/` |
| `azf` | Azure | FaaS (Azure Functions) | `backends/azure-functions/` |
| `docker` | — | Docker passthrough | `backends/docker/` |

Container backends inject the agent as a sidecar. FaaS backends bake the agent into the function image and use reverse WebSocket connections.

## Project Layout

```
api/                          Shared types and error definitions
agent/                        WebSocket agent (exec/attach inside workloads)
backends/
  core/                       Shared backend library (BaseServer, Docker API, StateStore)
  docker/                     Docker daemon passthrough
  ecs/                        AWS ECS Fargate
  lambda/                     AWS Lambda
  cloudrun/                   GCP Cloud Run Jobs
  cloudrun-functions/         GCP Cloud Functions
  aca/                        Azure Container Apps Jobs
  azure-functions/            Azure Functions
bleephub/                     GitHub Actions runner service API (official runner support)
cmd/sockerless/               CLI tool (context management, server control)
cmd/sockerless-admin/         Admin dashboard server (aggregates all components)
ui/                           React SPA monorepo (Bun, Vite, Tailwind, TanStack)
simulators/
  aws/                        AWS API simulator (ECS, ECR, IAM, VPC, EFS, Lambda, ...)
  gcp/                        GCP API simulator (Cloud Run, Compute, DNS, GCS, AR, ...)
  azure/                      Azure API simulator (ACA, ACR, Storage, Functions, ...)
terraform/
  modules/                    Terraform modules (one per backend)
  environments/               Terragrunt environments (live + simulator per backend)
tests/                        Integration tests (Docker SDK, 59 test functions)
smoke-tests/                  Real CI runner validation (act + gitlab-runner)
specs/                        Specification documents
```

Each backend, the agent, and the test suite are separate Go modules connected via `go.work`. Major components embed React dashboards at `/ui/`.

### Module Sizes

**Go**

![core](https://img.shields.io/badge/core-15.8k-00ADD8)
![bleephub](https://img.shields.io/badge/bleephub-14.5k-00ADD8)
![sim/aws](https://img.shields.io/badge/sim%2Faws-9.1k-00ADD8)
![sim/azure](https://img.shields.io/badge/sim%2Fazure-7.1k-00ADD8)
![sim/gcp](https://img.shields.io/badge/sim%2Fgcp-6.3k-00ADD8)
![admin](https://img.shields.io/badge/admin-3.3k-00ADD8)
![ecs](https://img.shields.io/badge/ecs-5.8k-5BC0DE)
![cloudrun](https://img.shields.io/badge/cloudrun-3.9k-5BC0DE)
![aca](https://img.shields.io/badge/aca-4.2k-5BC0DE)
![docker](https://img.shields.io/badge/docker-2.5k-5BC0DE)
![agent](https://img.shields.io/badge/agent-2.3k-5BC0DE)
![api](https://img.shields.io/badge/api-1.8k-5BC0DE)
![azf](https://img.shields.io/badge/azf-2.3k-A0D8EF)
![cli](https://img.shields.io/badge/cli-1.6k-A0D8EF)
![gcf](https://img.shields.io/badge/gcf-2.2k-A0D8EF)
![lambda](https://img.shields.io/badge/lambda-2.8k-A0D8EF)

**TypeScript**

![ui/admin](https://img.shields.io/badge/ui%2Fadmin-3.9k-3178C6)
![ui/core](https://img.shields.io/badge/ui%2Fcore-1.5k-3178C6)
![ui/bleephub](https://img.shields.io/badge/ui%2Fbleephub-1.1k-3178C6)
![ui/sim-aws](https://img.shields.io/badge/ui%2Fsim--aws-391-6295D2)
![ui/sim-gcp](https://img.shields.io/badge/ui%2Fsim--gcp-372-6295D2)
![ui/sim-azure](https://img.shields.io/badge/ui%2Fsim--azure-366-6295D2)
![ui/frontend-docker](https://img.shields.io/badge/ui%2Ffrontend--docker-230-6295D2)

### Coverage

![core](https://img.shields.io/badge/core-40%25-yellow)
![agent](https://img.shields.io/badge/agent-36%25-yellow)

> Cloud backends are tested via simulator integration tests and e2e smoke tests rather than unit tests.

## Prerequisites

- Go 1.25+
- Docker (for smoke tests and terraform integration tests)

For terraform operations:
- Terraform >= 1.5
- Terragrunt >= 0.50

## Quick Start

```bash
# Build the CLI and ECS backend
go build -o sockerless ./cmd/sockerless
go build -o sockerless-backend-ecs ./backends/ecs/cmd/sockerless-backend-ecs

# Option 1: config.yaml (preferred)
cat > ~/.sockerless/config.yaml <<EOF
environments:
  ecs-dev:
    backend: ecs
    aws:
      region: us-east-1
      ecs:
        cluster: sockerless
        subnets: [subnet-abc123]
        execution_role_arn: arn:aws:iam::123456789012:role/ecsExec
EOF
sockerless context use ecs-dev
sockerless server start

# Option 2: context commands
sockerless context create ecs-dev --backend ecs \
  --set AWS_REGION=us-east-1 \
  --set SOCKERLESS_ECS_CLUSTER=sockerless \
  --set SOCKERLESS_ECS_SUBNETS=subnet-abc123 \
  --set SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::123456789012:role/ecsExec
sockerless server start

# Use with Docker CLI
export DOCKER_HOST=tcp://localhost:3375
docker version
docker run --rm alpine echo "hello from sockerless"
docker ps -a
```

See [`cmd/sockerless/README.md`](cmd/sockerless/README.md) for the full `config.yaml` format and all CLI commands.

## Development

### Running Tests

```bash
# Core unit/integration tests
cd backends/core && go test -v ./...

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
make smoke-test-act-ecs          # ECS via simulator
make smoke-test-act-cloudrun     # Cloud Run via simulator
make smoke-test-act-aca          # ACA via simulator

# GitLab Runner
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
2. Import `backends/core`, embed `core.BaseServer`, and implement the `api.Backend` interface — only override methods that need cloud-specific logic
3. Add an entry point `main.go` that creates and starts the server
4. Add the module to `go.work`
5. Add integration tests in `tests/`
6. Add a simulator in `simulators/` if targeting a new cloud
7. Add a terraform module in `terraform/modules/`

## Deploying to Cloud

Each backend has a complete deployment walkthrough in its `examples/terraform/` directory covering infrastructure provisioning, image push, environment setup, validation, and tear-down.

- **Infrastructure provisioning** — [`terraform/README.md`](terraform/README.md) (modules, state backends, CI/CD workflows)
- **Step-by-step walkthroughs** — each backend's [`examples/terraform/README.md`](backends/ecs/examples/terraform/) (terraform apply through validation)
- **Configuration reference** — each backend's [`README.md`](backends/) (env vars, terraform output mapping)
- **Live test results** — [`PLAN_ECS_MANUAL_TESTING.md`](PLAN_ECS_MANUAL_TESTING.md) (ECS Fargate tested against real AWS, 11/12 phases pass)

## Documentation

| Document | Description |
|----------|-------------|
| [`specs/`](specs/) | Specifications: [main spec](specs/SOCKERLESS_SPEC.md), [config](specs/CONFIG.md), [backends](specs/BACKENDS.md), [drivers](specs/DRIVERS.md), [API](specs/API_SURFACE.md), [images](specs/IMAGE_MANAGEMENT.md) |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | System architecture, component diagrams, test architecture |
| [`terraform/README.md`](terraform/README.md) | Terraform modules, state backends, and CI/CD deployment |
| [`FEATURE_MATRIX.md`](FEATURE_MATRIX.md) | Docker API compatibility, cloud service mappings, test results |
| [`simulators/README.md`](simulators/README.md) | Cloud simulators: services, state management, CLI usage, bash tests |
| [`backends/*/README.md`](backends/) | Per-backend configuration and terraform output mapping |
| [`docs/GITHUB_RUNNER.md`](docs/GITHUB_RUNNER.md) | GitHub Actions E2E test guide (act + official runner) |
| [`docs/GITLAB_RUNNER_DOCKER.md`](docs/GITLAB_RUNNER_DOCKER.md) | GitLab Runner docker executor E2E test guide |
| [`AGENTS.md`](AGENTS.md) | Agent architecture (forward/reverse modes) |
| [`DECISIONS.md`](DECISIONS.md) | Technical decision log across all phases |
| [`PLAN.md`](PLAN.md) | Implementation plan and task tracking |
| [`PLAN_ECS_MANUAL_TESTING.md`](PLAN_ECS_MANUAL_TESTING.md) | ECS live testing plan, results, and bugs found |
| [`STATUS.md`](STATUS.md) | Project status and phase history |
