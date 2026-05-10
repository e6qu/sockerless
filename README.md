# Sockerless

[![CI](https://github.com/e6qu/sockerless/actions/workflows/ci.yml/badge.svg)](https://github.com/e6qu/sockerless/actions/workflows/ci.yml)
[![Go](https://img.shields.io/badge/Go-1.25-00ADD8?logo=go&logoColor=white)](https://go.dev)
[![Docker API](https://img.shields.io/badge/Docker_API-v1.44-2496ED?logo=docker&logoColor=white)](https://docs.docker.com/engine/api/v1.44/)
[![License: AGPL-3.0](https://img.shields.io/badge/License-AGPL--3.0-blue.svg)](LICENSE)
[![AWS](https://img.shields.io/badge/AWS-ECS_|_Lambda-FF9900?logo=amazonwebservices&logoColor=white)](#backends)
[![GCP](https://img.shields.io/badge/GCP-Cloud_Run_|_GCF-4285F4?logo=googlecloud&logoColor=white)](#backends)
[![Azure](https://img.shields.io/badge/Azure-ACA_|_AZF-0078D4?logo=microsoftazure&logoColor=white)](#backends)

[![Go](https://img.shields.io/badge/Go-123.5k_lines-00ADD8?logo=go&logoColor=white)](#module-sizes)
[![TypeScript](https://img.shields.io/badge/TypeScript-16.1k_lines-3178C6?logo=typescript&logoColor=white)](#module-sizes)
[![Tests](https://img.shields.io/badge/Tests-62.1k_lines-brightgreen)](#module-sizes)
[![Coverage](https://img.shields.io/badge/Core_Coverage-40%25-yellow)](#module-sizes)
[![Modules](https://img.shields.io/badge/Go_Modules-34-informational)](#module-sizes)

A Docker-compatible REST API daemon that executes containers on cloud serverless backends instead of a local Docker Engine. Standard Docker clients (`docker run`, Docker SDK, CI runners) connect to Sockerless exactly as they would to a real Docker daemon — but containers run on AWS ECS, Google Cloud Run, Azure Container Apps, and more.

> **2026-05-07 — 8/8 runner-integration cells GREEN.** GitHub × {ECS, Lambda, Cloud Run, GCF} and GitLab × the same four are all running the full probe + git-clone + go-build + arithmetic suite end-to-end against real cloud infrastructure. See [STATUS.md](STATUS.md) for live URLs. The closing milestone shipped Phase 123, the **storage backing driver abstraction** — `gcs-sync` replaces FUSE-on-object-store for shared workspaces. That driver pattern (cloud-agnostic core interface + per-cloud impls + operator-pluggable selection at config time + no-fallbacks discipline) is the proven precedent for a wider driver-generalization plan covering networking, DNS, and access — see [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md) and [PLAN.md](PLAN.md) Phases 124-127.

## Why

No existing project fills this niche. Docker Engine, Podman, Colima, and Rancher Desktop all run containers locally. No cloud service exposes a Docker-compatible REST API. Sockerless bridges this gap: **Docker API on top, cloud serverless capacity on the bottom.**

Primary use cases:

- **CI runners** — GitLab Runner (docker-executor) and GitHub Actions Runner (container jobs) work without modification — see [`docs/RUNNERS.md`](docs/RUNNERS.md) for the wiring guide
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

![core](https://img.shields.io/badge/core-19.1k-00ADD8)
![bleephub](https://img.shields.io/badge/bleephub-16k-00ADD8)
![sim/aws](https://img.shields.io/badge/sim%2Faws-11.3k-00ADD8)
![sim/azure](https://img.shields.io/badge/sim%2Fazure-9.1k-00ADD8)
![sim/gcp](https://img.shields.io/badge/sim%2Fgcp-8.6k-00ADD8)
![admin](https://img.shields.io/badge/admin-3.3k-00ADD8)
![ecs](https://img.shields.io/badge/ecs-6.7k-5BC0DE)
![cloudrun](https://img.shields.io/badge/cloudrun-6k-5BC0DE)
![aca](https://img.shields.io/badge/aca-4.2k-5BC0DE)
![docker](https://img.shields.io/badge/docker-2.6k-5BC0DE)
![agent](https://img.shields.io/badge/agent-5.9k-5BC0DE)
![api](https://img.shields.io/badge/api-2.1k-5BC0DE)
![azf](https://img.shields.io/badge/azf-2.7k-A0D8EF)
![cli](https://img.shields.io/badge/cli-1.6k-A0D8EF)
![gcf](https://img.shields.io/badge/gcf-5.9k-A0D8EF)
![lambda](https://img.shields.io/badge/lambda-5.4k-A0D8EF)

**TypeScript**

![ui/admin](https://img.shields.io/badge/ui%2Fadmin-7.6k-3178C6)
![ui/core](https://img.shields.io/badge/ui%2Fcore-3.6k-3178C6)
![ui/bleephub](https://img.shields.io/badge/ui%2Fbleephub-2.8k-3178C6)
![ui/sim-aws](https://img.shields.io/badge/ui%2Fsim--aws-247-6295D2)
![ui/sim-gcp](https://img.shields.io/badge/ui%2Fsim--gcp-228-6295D2)
![ui/sim-azure](https://img.shields.io/badge/ui%2Fsim--azure-221-6295D2)
![ui/frontend-docker](https://img.shields.io/badge/ui%2Ffrontend--docker-263-6295D2)

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
# Bring up a full local dev stack (sim + backend + admin) for any
# cloud × backend combination. Sim runs on its native port (4566 /
# 4567 / 4568); backend on :3375; admin UI on :9090/ui/.
make stack-aws-ecs            # AWS ECS
make stack-gcp-cloudrun       # GCP Cloud Run
make stack-azure-aca          # Azure Container Apps
# (other combos: stack-aws-lambda, stack-gcp-gcf, stack-azure-azf)

# Use with Docker CLI
export DOCKER_HOST=tcp://localhost:3375
docker version
docker run --rm alpine echo "hello from sockerless"
docker ps -a

# Browse the operator UI:
#   admin       http://localhost:9090/ui/
#   sim AWS     http://localhost:4566 (or :4567 GCP / :4568 Azure)

# Tear down when done:
make stack-down               # stops sim + backend + admin
```

For real cloud deployment (instead of the local sim), use the `sockerless` CLI to register a context and start the backend pointing at real cloud:

```bash
make cmd/sockerless/build     # build the CLI

cat > ~/.sockerless/config.yaml <<EOF
environments:
  ecs-prod:
    backend: ecs
    aws:
      region: us-east-1
      ecs:
        cluster: sockerless
        subnets: [subnet-abc123]
        execution_role_arn: arn:aws:iam::123456789012:role/ecsExec
EOF
./cmd/sockerless/sockerless context use ecs-prod
./cmd/sockerless/sockerless server start

export DOCKER_HOST=tcp://localhost:3375
docker run --rm alpine echo "hello from cloud sockerless"
```

See [`cmd/sockerless/README.md`](cmd/sockerless/README.md) for the full `config.yaml` format and all CLI commands.

## Make targets

Sockerless uses a uniform Makefile layout across all 33 leaf apps (Go binaries, UI packages, test directories) plus a thin top-level orchestrator. Every leaf implements the same 7-target surface; the top-level fans out and adds a `stack-<cloud>-<backend>` orchestration layer.

Specification: [`docs/MAKEFILE_STANDARD.md`](docs/MAKEFILE_STANDARD.md).

### Standardized target surface (every app)

```
make help               # list this app's targets
make install            # fetch deps (go mod download / bun install)
make build              # build the artefact (UI embedded if present)
make build-noui         # build the binary without embedded UI (Go apps)
make test               # unit tests
make test-integration   # integration tests (build-tag + env-var gated)
make lint               # go vet (or tsc --noEmit) + gofmt + golangci-lint when available
make run                # run the binary in the foreground
make dev                # Go server (no UI) + Vite dev server (UI apps)
make embed              # copy UI dist into local dist/ (Go-with-UI apps)
make clean              # remove build artefacts
```

### Top-level fan-out

Run any standardized target across every app:

```bash
make build              # build all 17 binaries + 14 UI bundles
make test               # run every unit-test suite (admin + bleephub + core all green; backends + sims as configured)
make lint               # lint every Go module + tsc --noEmit every UI package
make clean              # remove every build artefact
make install            # install deps everywhere
```

### Path-based delegation

Run any standardized target on a single app via its directory path:

```bash
make backends/ecs/build         # build one backend
make backends/ecs/test          # test one backend
make backends/ecs/run           # foreground; defaults --addr :3375 + sim env-var
make backends/ecs/clean         # clean one backend

make ui/packages/admin/run      # vite dev server for admin UI
make ui/packages/bleephub/test  # vitest for bleephub UI

make simulators/aws/run         # foreground sim on :4566
make simulators/aws/sdk-test    # SDK tests against the sim (sim-specific target)
```

The pattern works for any app + any standardized target.

### Stack orchestration

Two layers, both writing PID + log files under `.stack-pids/<name>.{pid,log}` so `stack-status` / `stack-down` find every component.

**Pre-canned stacks** — the common 1-sim + 1-backend + admin shape:

| Target | Stack |
|---|---|
| `make stack-aws-ecs` | sim-aws (:4566) + backend-ecs (:3375) + admin (:9090) |
| `make stack-aws-lambda` | sim-aws + backend-lambda + admin |
| `make stack-gcp-cloudrun` | sim-gcp (:4567) + backend-cloudrun + admin |
| `make stack-gcp-gcf` | sim-gcp + backend-gcf + admin |
| `make stack-azure-aca` | sim-azure (:4568) + backend-aca + admin |
| `make stack-azure-azf` | sim-azure + backend-azf + admin |
| `make stack-bleephub-up` | also start bleephub on :5555 (run after a stack-X-Y) |
| `make stack-status` | show running components |
| `make stack-down` | stop all running components, clean PIDs |

**Granular per-component lifecycle** — for arbitrary topologies (any number of sims + backends + bleephubs in any combination); admin's REST surface drives these too. See `docs/ADMIN_ORCHESTRATION.md` for the `sockerless.yaml` schema that admin reads.

| Target | Purpose |
|---|---|
| `make start-component KIND=sim CLOUD=aws NAME=… PORT=…` | start one sim |
| `make start-component KIND=backend CLOUD=… BACKEND=… NAME=… PORT=… SIM_PORT=…` | start one backend (SIM_PORT links to a running sim's port) |
| `make start-component KIND=bleephub NAME=… PORT=…` | start one bleephub |
| `make stop-component NAME=…` | SIGTERM the named component |
| `make rebuild-component KIND=… [CLOUD=…] [BACKEND=…]` | `make build` for that component's dir |
| `make logs-component NAME=… [LINES=200]` | tail one component's log |
| `make status-components` / `make stop-components` | sweep across every running component |

Logs land in `.stack-pids/<NAME>.log` (one per component instance). The pre-canned `stack-X-Y` macros use `sim` / `backend` / `admin` as the names, so back-compat with `stack-status` / `stack-down` is preserved.

### Per-app shortcuts

Every app's own Makefile is the source of truth. To work on a single app, `cd` into it and use the standardized targets directly — no top-level needed:

```bash
cd backends/ecs
make help                # shows everything available
make build               # builds with embedded UI if dist/ available, else falls back
make run                 # foreground server with sensible defaults
make test                # go test ./...
```

This works for any of the 17 Go-binary apps, 14 UI packages, and the test-category dirs.

### Apps inventory

**Go binaries with optional embedded UI (12)** — each has the full target surface plus `build-noui` and `embed`:

```
cmd/sockerless-admin                 # admin server, port :9090
bleephub                             # GitHub-API simulator, port :5555
backends/{docker,ecs,lambda}         # AWS-side + local Docker
backends/{cloudrun,cloudrun-functions}  # GCP-side
backends/{aca,azure-functions}       # Azure-side
simulators/{aws,gcp,azure}           # Per-cloud REST simulators (ports :4566/:4567/:4568)
```

**Go-only binaries (5)**:

```
cmd/sockerless                       # The CLI (no UI to embed)
agent                                # Sockerless-agent + 3 cloud bootstrap binaries
github-runner-dispatcher-{aws,gcp,azure}  # Per-cloud runner dispatcher daemons
```

The agent module also exposes `make build-bootstraps` to cross-compile the three Lambda / Cloud Run / GCF bootstrap binaries (linux/amd64).

**UI packages (14)** — Vite + Bun, all in one Bun workspace:

```
ui/packages/admin                    # Operator dashboard
ui/packages/bleephub                 # GitHub-API simulator UI
ui/packages/backend-{docker,ecs,lambda,cloudrun,gcf,aca,azf}
ui/packages/simulator-{aws,gcp,azure}
ui/packages/frontend-docker          # Docker frontend proxy UI
ui/packages/core                     # Shared library (no own dist)
```

**Test categories (10)** — each has `make test`, `make lint`, `make clean`:

```
tests                                # Cross-backend e2e suite
smoke-tests                          # Per-cloud Docker-in-Docker smoke
simulators/{aws,gcp,azure}/{sdk-tests,cli-tests,terraform-tests}
tests/runners/{github,gitlab,gcp-cells,internal}
```

### Sim-specific extra targets

Each `simulators/<cloud>/Makefile` keeps the historical test breakdown CI relies on:

```bash
cd simulators/aws
make sdk-test           # go test in sdk-tests/ (own go.mod)
make cli-test           # go test in cli-tests/ (own go.mod)
make terraform-test     # go test in terraform-tests/ (own go.mod)
make shared-test        # go test in shared/ (own go.mod)
make test-all           # all four
make docker-build       # build the docker image
make docker-run         # docker run with port + env wired
make docker-test        # run all tests inside docker
```

### Legacy aliases (preserved at top level)

For backward-compat with existing CI workflows and developer muscle memory:

```bash
make sim-test-{ecs,lambda,cloudrun,gcf,aca,azf}      # per-backend integration tests
make sim-test-{aws,gcp,azure,all}                    # per-cloud aggregates
make smoke-test-{act,act-ecs,act-cloudrun,act-aca,act-all}
make smoke-test-{gitlab,gitlab-ecs,gitlab-cloudrun,gitlab-aca,gitlab-all}
make tf-int-test-{ecs,lambda,cloudrun,gcf,aca,azf,aws,gcp,azure,all}
make e2e-github-{ecs,lambda,cloudrun,gcf,aca,azf,all}
make e2e-gitlab-{ecs,lambda,cloudrun,gcf,aca,azf,all}
make upstream-test-{act,act-individual,act-{ecs,lambda,cloudrun,gcf,aca,azf,all}}
make upstream-test-gcl-{ecs,lambda,cloudrun,gcf,aca,azf,all}
make bleephub-test bleephub-gh-test
make test-{unit,e2e,agent,core,bleephub}
make check-backend-coverage{,-enforce}
```

These delegate to the appropriate per-app or test-category Makefile, or keep their inline Docker-build form for the smoke / e2e / upstream families.

## Development

The Make-target reference above covers most workflows. This section is the working-developer cheatsheet.

### One-shot: clean rebuild + run

```bash
make clean                        # remove every build artefact
make build                        # rebuild every binary + UI bundle
make stack-aws-ecs                # ready in seconds
```

### Iterating on a single backend

```bash
cd backends/ecs
make build && make run            # standalone, no sim
# Or, with a simulator running on :4566 already:
make run                          # already env-wired via the Makefile (SOCKERLESS_ENDPOINT_URL=http://localhost:4566)
make test                         # unit tests
make test-integration             # build-tag + env-var gated tests (against sim)
```

### Iterating on UI

```bash
cd ui/packages/admin
make run                          # vite dev server on :5173 with hot reload
                                  # (proxies /api → :9090, so admin Go server must be running)
```

In another terminal:
```bash
cd cmd/sockerless-admin
make build-noui && make run       # admin Go server with no embedded UI
```

Or build a single UI package + embed it into its consuming Go app:
```bash
cd ui/packages/bleephub && make build
cd bleephub && make build         # picks up ../ui/packages/bleephub/dist
```

### Running test suites

```bash
make test                         # unit tests across every app

# Per-app:
make backends/ecs/test
make ui/packages/admin/test

# Sim-specific test categories (CI uses these):
cd simulators/aws
make sdk-test                     # SDK-driven tests against the sim
make cli-test                     # CLI-driven tests against the sim
make terraform-test               # terraform-driven tests against the sim
make test-all                     # all four (sdk + cli + terraform + shared)

# Cross-backend e2e:
make tests/test                   # delegates to tests/Makefile

# Smoke (Docker-in-Docker):
make smoke-test-act-ecs           # GitHub Actions (act) → ECS via sim
make smoke-test-gitlab-ecs        # GitLab Runner → ECS via sim
```

### Lint

```bash
make lint                         # every Go module + every UI package
make backends/ecs/lint            # one app
```

`make lint` runs `go vet`, `gofmt -l`, and (if installed) `golangci-lint run`. UI packages run `tsc --noEmit`. Lint passes `-tags noui` for Go modules that embed a UI, so missing `dist/` doesn't trip `//go:embed`.

### Terraform

Infrastructure-as-code for all backends lives in `terraform/`. See [`terraform/README.md`](terraform/README.md) for details.

```bash
cd terraform
make fmt                  # Format .tf files
make validate             # Validate all modules
make plan-ecs-simulator   # Plan against local simulator
make apply-ecs-simulator  # Apply against local simulator
```

### Adding a new app

The Makefile system was designed so adding a new backend / simulator / dispatcher takes one file:

1. Drop a `Makefile` in the new app's directory (5–10 lines — see [`docs/MAKEFILE_STANDARD.md`](docs/MAKEFILE_STANDARD.md) for the template per app kind).
2. Add the app's path to one of `GO_UI_APPS`, `GO_APPS`, or `UI_APPS` in the top-level `Makefile`.
3. Run `make <new-app>/help` to verify the standardized target surface picked up.

For a new backend specifically:

1. Create `backends/<name>/` as a new Go module.
2. Import `backends/core`, embed `core.BaseServer`, implement the `api.Backend` interface — only override methods that need cloud-specific logic.
3. Add an entry point `main.go` that creates and starts the server.
4. Add the module to `go.work`.
5. Add a Makefile (using the go-app.mk template) with `APP_NAME`, `GO_PACKAGE`, `UI_PACKAGE`, `DEFAULT_PORT`, `RUN_FLAGS`, `RUN_ENV`, `REPO_ROOT_REL := ../..`.
6. Add integration tests in `tests/`.
7. Add a simulator in `simulators/` if targeting a new cloud (with its own Makefile).
8. Add a terraform module in `terraform/modules/`.

## Deploying to Cloud

Each backend has a complete deployment walkthrough in its `examples/terraform/` directory covering infrastructure provisioning, image push, environment setup, validation, and tear-down.

- **Infrastructure provisioning** — [`terraform/README.md`](terraform/README.md) (modules, state backends, CI/CD workflows)
- **Step-by-step walkthroughs** — each backend's [`examples/terraform/README.md`](backends/ecs/examples/terraform/) (terraform apply through validation)
- **Configuration reference** — each backend's [`README.md`](backends/) (env vars, terraform output mapping)
- **Manual test runbooks** — [`manual-tests/`](manual-tests/) (per-cloud live-infra sweeps; AWS validated in eu-west-1)

## Documentation

| Document | Description |
|----------|-------------|
| [`specs/`](specs/) | Specifications: [main spec](specs/SOCKERLESS_SPEC.md), [config](specs/CONFIG.md), [backends](specs/BACKENDS.md), [drivers](specs/DRIVERS.md), [API](specs/API_SURFACE.md), [images](specs/IMAGE_MANAGEMENT.md) |
| [`ARCHITECTURE.md`](ARCHITECTURE.md) | System architecture, component diagrams, test architecture |
| [`terraform/README.md`](terraform/README.md) | Terraform modules, state backends, and CI/CD deployment |
| [`FEATURE_MATRIX.md`](FEATURE_MATRIX.md) | Docker API compatibility, cloud service mappings, test results |
| [`simulators/README.md`](simulators/README.md) | Cloud simulators: services, state management, CLI usage, bash tests |
| [`backends/*/README.md`](backends/) | Per-backend configuration and terraform output mapping |
| [`docs/RUNNERS.md`](docs/RUNNERS.md) | **CI runner wiring** — canonical guide: GitHub Actions + GitLab Runner against ECS + Lambda, token strategy, 4-cell coverage matrix |
| [`docs/GITHUB_RUNNER.md`](docs/GITHUB_RUNNER.md) | GitHub Actions E2E test guide (act + official runner) |
| [`docs/GITLAB_RUNNER_DOCKER.md`](docs/GITLAB_RUNNER_DOCKER.md) | GitLab Runner docker executor E2E test guide |
| [`AGENTS.md`](AGENTS.md) | Agent architecture (forward/reverse modes) |
| [`DECISIONS.md`](DECISIONS.md) | Technical decision log across all phases |
| [`PLAN.md`](PLAN.md) | Implementation plan and task tracking |
| [`manual-tests/`](manual-tests/) | Per-cloud live-infra manual test runbooks |
| [`STATUS.md`](STATUS.md) | Project status and phase history |
