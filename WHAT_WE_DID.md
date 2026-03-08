# Sockerless — What We Built

## The Idea

Sockerless presents an HTTP REST API identical to Docker's. CI runners (GitHub Actions via `act`, GitLab Runner, `gitlab-ci-local`) talk to it as if it were Docker, but instead of running containers locally, Sockerless farms work to cloud backends — ECS, Lambda, Cloud Run, Cloud Functions, Azure Container Apps, Azure Functions — or passes through to a local Docker daemon.

Cloud simulators stand in for real AWS/GCP/Azure APIs during development and testing, validated against official cloud SDKs, CLIs, and Terraform providers.

## Architecture

```
CI Runner (act, gitlab-runner, gitlab-ci-local)
    |
    v
Frontend (Docker REST API)
    |
    v
Backend (ecs | lambda | cloudrun | gcf | aca | azf | docker)
    |
    v
Cloud Simulator (AWS | GCP | Azure)
Agent (inside container or reverse-connected)
```

**7 backends** share a common core (`backends/core/`) with driver interfaces (Exec, Filesystem, Stream, Network). Cloud backends delegate image management to per-cloud shared modules (`aws-common`, `gcp-common`, `azure-common`).

**3 simulators** (`simulators/{aws,gcp,azure}/`) implement enough cloud API surface for the backends to work.

## Completed Phases

| Phase | What |
|---|---|
| 1-10 | Foundation: 3 simulators, 7 backends (8 originally; memory removed in Phase 90), agent, Docker REST API frontend |
| 11-34 | E2E tests (371 workflows), driver interfaces, Docker build |
| 35-52 | bleephub (GitHub API + runner), CLI, crash safety, pods, service containers |
| 53-56 | Production Docker API: TLS, auth, logs, DNS, restart, events, filters |
| 57-61 | CI runners: GitHub Actions multi-job/matrix/secrets + GitLab CI DAG/expressions |
| 62-67 | API hardening, Compose E2E, webhooks, GitHub Apps, OTel, network isolation |
| 69-72 | ARM64, simulator fidelity, SDK/CLI verification, full-stack E2E |
| 73-77 | UI: Bun/Vite/React 19 monorepo, 14 SPAs, LogViewer |
| 79-82 | Admin: dashboard, docs, process management, project bundles |
| 83-86 | Type-safe API: goverter mappers, api.Backend, self-dispatch, in-process wiring |
| 90 | Remove memory backend, spec-driven state machine tests |

## Bug Fix Sprints

583 bugs fixed across 45 sprints (BUG-001 through BUG-583). 0 open bugs. Per-sprint details in `_tasks/done/BUG-SPRINT-*.md`.

## Unified Image Management

Consolidated all image management into per-cloud shared modules: `core.ImageManager` + `core.AuthProvider` interface, 3 shared cloud modules (ECR, Artifact Registry, ACR). ~2000 lines of duplication eliminated.

## Cloud Commons Consolidation

Moved duplicated infrastructure into shared per-cloud modules (`aws-common`, `gcp-common`, `azure-common`):
- **Error mappers**: `MapAWSError`/`MapGCPError`/`MapAzureError` + `containsAny` helper
- **SignalToExitCode**: Exported from core, replaced 6 local copies
- **MergeEnvByKey**: Replaced 3 local `mergeEnvByKey` copies with `core.MergeEnvByKey`
- **StreamCloudLogs**: Core helper with `CloudLogFetchFunc` pattern for cloud log streaming (pipe, follow-mode, tail, Docker mux framing)
- **StorageDriver interface**: `CreateVolume`/`DeleteVolume`/`MountSpec` with `NoOpStorageDriver` default
- **ServiceDiscoveryDriver interface**: `Register`/`Deregister`/`Resolve` with `NoOpServiceDiscoveryDriver` default
- **CloudExecDriver interface**: `Exec`/`Attach`/`Supported` with `NoOpCloudExecDriver` default
- **CloudNetworkDriver interface**: `EnsureNetwork`/`DeleteNetwork`/`AttachContainer`/`DetachContainer` with `NoOpCloudNetworkDriver` default

### Cloud-Native Driver Implementations

Replaced all cloud common module stubs with real implementations on backend Server structs:

- **ECS Exec**: `ExecuteCommand` API + SSM Session Manager WebSocket bridge (`wsBridge` adapter)
- **ECS Networking**: VPC Security Groups (create/delete/self-referencing ingress), per-container SG association
- **ECS Service Discovery**: AWS Cloud Map — private DNS namespace, service registration/deregistration/discovery
- **CloudRun Networking**: Cloud DNS managed zones (create/delete/record cleanup)
- **CloudRun Service Discovery**: Cloud DNS A records (register/deregister/resolve by FQDN)
- **ACA Exec**: Container Apps exec API via WebSocket (`wsBridge` adapter)
- **ACA Networking**: NSG name/rule tracking (state management for Azure Network SDK integration)
- **ACA Service Discovery**: In-process DNS registry (hostname→IP per network, with container cleanup)

All cloud network/service-discovery methods wired into `NetworkCreate`, `NetworkRemove`, `NetworkConnect`, `NetworkDisconnect`, `ContainerStart`, and `ContainerRemove` handlers. ~878 lines of logging boilerplate eliminated by `StreamCloudLogs`. ~12 dead stub files removed from common modules.

## Project Stats

See [STATUS.md](STATUS.md) for current test counts.

- **85 phases**, 756 tasks completed
- **45 bug sprints**, 583 bugs fixed, 0 open
- **16 Go modules** across backends, simulators, agent, API, frontend, bleephub, CLI, admin, tests
- **7 backends** sharing a common driver architecture
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform
