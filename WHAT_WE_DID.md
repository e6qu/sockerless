# Sockerless — What We Built

## The Idea

Sockerless presents an HTTP REST API identical to Docker's. CI runners (GitHub Actions via `act`, GitLab Runner, `gitlab-ci-local`) talk to it as if it were Docker, but instead of running containers locally, Sockerless farms work to cloud backends — ECS, Lambda, Cloud Run, Cloud Functions, Azure Container Apps, Azure Functions — or runs everything in-process via a WASM sandbox (the "memory" backend).

For development and testing, cloud simulators stand in for real AWS/GCP/Azure APIs, providing actual execution of tasks the same way a real cloud would. The simulators are validated against official cloud SDKs, CLIs, and Terraform providers.

## Architecture

```
CI Runner (act, gitlab-runner, gitlab-ci-local)
    │
    ▼
Frontend (Docker REST API)
    │
    ▼
Backend (ecs | lambda | cloudrun | gcf | aca | azf | memory | docker)
    │                                                    │
    ▼                                                    ▼
Cloud Simulator (AWS | GCP | Azure)              WASM Sandbox
    │                                           (wazero + mvdan.cc/sh
    ▼                                            + go-busybox)
Agent (inside container or reverse-connected)
```

**8 backends** share a common core (`backends/core/`) with driver interfaces:
- **ExecDriver** — runs commands (WASM shell, forward agent, reverse agent, or synthetic echo)
- **FilesystemDriver** — manages container filesystem (temp dirs, agent bridge, staging)
- **StreamDriver** — attach/logs streaming (pipes, WebSocket relay, log buffer)
- **ProcessLifecycleDriver** — start/stop/kill/cleanup

Each driver chains: Agent → Process → Synthetic, so every handler call falls through to the right implementation.

**3 simulators** (`simulators/{aws,gcp,azure}/`) implement enough cloud API surface for the backends to work. Each is tested against the real SDK, CLI, and Terraform provider for that cloud.

## Completed Phases (1-56)

| Phase | What | Key Artifacts |
|---|---|---|
| 1-10 | Foundation: simulators (AWS/GCP/Azure), backends, agent, frontend | 3 simulators, 8 backends, Docker REST API frontend |
| 11-20 | WASM sandbox, process execution, archive ops, build contexts | `sandbox/` module, pre-start staging, bind mounts |
| 21-30 | E2E tests, driver interfaces, GitLab runner support | 217 GitHub + 154 GitLab E2E tests, DriverSet |
| 31 | Enhanced WASM sandbox: 12 new builtins, pwd fix | 46 sandbox tests |
| 32 | Driver interface completion + code splitting (<400 lines/file) | 6 new driver methods, 0 handler bypasses |
| 33 | Service container support: health checks, NetworkingConfig | 6 health check tests |
| 34 | Docker build endpoint: Dockerfile parser (FROM/COPY/ENV/CMD/...) | 15 parser tests |
| 35 | bleephub: official GitHub Actions runner integration | Azure DevOps-derived internal API, Docker integration test |
| 36 | bleephub: users, auth, OAuth device flow, GraphQL engine | 19 unit tests |
| 37 | bleephub: in-memory git hosting (go-git, smart HTTP) | 33 unit tests |
| 38 | bleephub: organizations, teams, RBAC | 51 unit tests |
| 39 | bleephub: issues, labels, milestones, comments, reactions | 79 unit tests |
| 40 | bleephub: pull requests, reviews, merge | 107 unit tests |
| 41 | bleephub: API conformance, REST pagination, OpenAPI, `gh` CLI test | 148 unit tests + gh CLI integration |
| 42 | bleephub: `uses:` actions, multi-job engine, matrix, artifacts/cache | 190 unit tests |
| 43 | Cloud resource tracking: unified tagging, resource registry, CloudScanner | 11 new tests |
| 44 | Crash-only software: auto-save, atomic writes, startup recovery | 18 new tests |
| 45 | Podman pod API + PodContext/PodRegistry, implicit grouping | 21 pod tests |
| 46 | Cloud multi-container specs: deferred start, ECS/CR/ACA multi-container | 8 new tests |
| 47 | sockerless CLI + context management (`~/.sockerless/`) | 6 new tests |
| 48 | Private management API: healthz, status, container summary | 4 new tests |
| 49 | Server lifecycle + metrics: latency percentiles, PID management | 4 new tests |
| 50 | Resource management, health checker, live config reload | 3 new tests |
| 51 | CI service containers: bleephub services parsing, health wait, E2E | 9 new tests |
| 52 | Upstream test expansion: gitlab-ci-local 36, GitHub 31, GitLab 22 | 23 new E2E test cases |
| 53 | Production Docker API: TLS, Docker auth, registry creds, tmpfs | 16 new tests |
| 54 | Production Docker API: log filter/follow, ExtraHosts, DNS, restart | 18 new tests |
| 55 | Production Docker API: EventBus, disconnect, update, volume filters | 15 new tests |
| 56 | Docker API polish: list limit/sort, ancestor/network/health/before/since filters, export, commit, push stub, flushingCopy | 14 new tests |

## Project Stats

- **56 phases**, 461 tasks completed
- **15 Go modules** across backends, simulators, sandbox, agent, API, frontend, bleephub, tests
- **21 Go-implemented builtins** in WASM sandbox
- **10 driver interface methods** across 4 driver types (ExecDriver 1, FilesystemDriver 4, StreamDriver 4, ProcessLifecycleDriver 8)
- **6 external test consumers**: `act` (31 workflows), `gitlab-runner` (22 pipelines), `gitlab-ci-local` (36 tests), upstream act test suite, official `actions/runner`, `gh` CLI
- **Core tests**: 177 PASS | **Frontend tests**: 4 PASS (TLS)
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform
- **8 backends** sharing a common driver architecture
