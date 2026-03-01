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

## Completed Phases (1-56) — Summary

| Phase | What | Key Artifacts |
|---|---|---|
| 1-10 | Foundation: simulators (AWS/GCP/Azure), backends, agent, frontend | 3 simulators, 8 backends, Docker REST API frontend |
| 11-34 | WASM sandbox, E2E tests, driver interfaces, Docker build | 217 GitHub + 154 GitLab E2E, 46 sandbox tests |
| 35-42 | bleephub: GitHub API + runner + multi-job engine | 190 unit tests, users, auth, git, orgs, issues, PRs, `gh` CLI |
| 43-52 | CLI, crash safety, pods, service containers, upstream expansion | sockerless CLI, PodContext, resource registry |
| 53-56 | Production Docker API: TLS, auth, logs, DNS, restart, events, filters, export, commit | 16+18+15+14 new tests |

## Phases 57-66 — CI Runners, API Hardening, bleephub Features, OTel

| Phase | What | Key Results |
|---|---|---|
| 57 | Production GitHub Actions multi-job engine | 221 bleephub unit + 6 integration |
| 58 | CI/CD pipeline, Dockerfiles, goreleaser, GHCR images | Core CI + release workflows |
| 59 | Secrets, expressions, matrix fail-fast, concurrency | 259 bleephub unit + 9 integration |
| 60 | gitlabhub: GitLab Runner coordinator + DAG engine | 62 unit + 9 integration |
| 61 | Advanced GitLab CI: expressions, extends, include, matrix | 129 unit + 17 integration |
| 62 | Docker API hardening: HEALTHCHECK, volumes, mounts, prune | 230 core PASS |
| 63 | Compose E2E: health race fix, SHELL/VOLUME directives | 249 core PASS |
| 64 | bleephub webhooks: HMAC-SHA256, async delivery, CI trigger | 270 bleephub PASS |
| 65 | GitHub Apps: JWT auth, installation tokens, manifest flow | 293 bleephub PASS |
| 66 | OTel tracing: OTLP HTTP, otelhttp middleware, spans | 241 core + 298 bleephub + 129 gitlabhub + 7 frontend |

## Phase 67 — Network Isolation (Linux)

IPAllocator, SyntheticNetworkDriver (8 methods), Linux NetnsManager (build-tagged), LinuxNetworkDriver wrapper, handler refactoring to driver pattern.

**Tests**: 255 core PASS (+14)

## Phase 69 — ARM64 / Multi-Arch Completion

Goreleaser 8→15 builds (added 6 cloud backends + gitlabhub), gitlabhub Dockerfile.release, docker.yml 6→7 images, CI build-check 8→15 binaries + ARM64 cross-compile job.

## Phase 70 — Simulator Fidelity

Brought all three cloud simulators to production quality: real process execution via shared ProcessRunner engine, structured log queries (CloudWatch/Cloud Logging/Log Analytics), correct status enums, SDK/CLI/Terraform compatibility. 24 tasks, 15 shared ProcessRunner tests.

**Tests**: SDK: AWS 8→21, GCP 8→23, Azure 7→16 | Shared ProcessRunner: 15 PASS

## Phase 71 — SDK/CLI Verification & Documentation

Closed three gaps: FaaS real execution (Lambda/GCF/AZF), CLI execution+log verification tests, README quick-starts. Built arithmetic evaluator (recursive-descent parser) with 27 new tests (21 SDK + 6 CLI) across all clouds.

**Tests**: SDK: AWS 42, GCP 43, Azure 38 | CLI: AWS 26, GCP 21, Azure 19

## Phase 72 — Full-Stack E2E Tests

Real arithmetic execution through full Docker API stack (Frontend → Backend → Simulator). Forward-agent backends (ECS/CloudRun/ACA): 18 arithmetic tests + fast-exit fix. FaaS backends (Lambda/GCF/AZF): enabled real execution via invoke-with-command, 18 arithmetic tests. Central multi-backend tests in `tests/` module (4 tests). Fixed ECS test name collisions.

**Tests**: sim-test-all: 75 PASS, test-e2e: 65 PASS

## Phase 68 — Multi-Tenant Backend Pools (In Progress)

### P68-001: Pool Configuration ✅
Added `PoolConfig` and `PoolsConfig` types to `backends/core/` for defining named backend pools with concurrency limits and queue sizes. `ValidatePoolsConfig()` checks 8 rules (non-empty pools, unique names, valid backend types, non-negative limits, default pool exists). `LoadPoolsConfig()` loads from `SOCKERLESS_POOLS_CONFIG` env var → `$SOCKERLESS_HOME/pools.json` → default single-pool config. Includes `GetPool()` and `PoolNames()` convenience methods. 18 tests.

## Phase 73 — UI Foundation + Shared Core + Memory Backend Dashboard

Established the web UI monorepo and delivered the first working embedded SPA. Key deliverables:

- **Monorepo scaffold**: Bun workspaces + Turborepo + TypeScript 5.8, `ui/` root with `packages/core/` and `packages/backend-memory/`
- **Shared core package** (`@sockerless/ui-core`): API client + types mirroring Go structs, 7 TanStack Query hooks (health/status/containers/metrics/resources/check/info), 7 Tailwind-styled components (AppShell, StatusBadge, MetricsCard, RefreshButton, ErrorBoundary, Spinner, DataTable)
- **Memory backend SPA**: React 19 + Vite 6.4 + React Router 7 + Tailwind CSS 4. Four dashboard pages: Overview (status + health checks + system info), Containers (DataTable with sorting/filtering), Resources (registry entries), Metrics (goroutines, heap, request latencies P50/P95/P99)
- **Go embed system**: `SPAHandler` with index.html fallback for client-side routing, `RegisterUI(fs.FS)` on BaseServer (zero impact on backends without UI), build-tagged `ui_embed.go`/`ui_noembed.go` in memory backend
- **Build integration**: Makefile targets (`ui-build`, `ui-test`, `build-memory-with-ui`, `build-memory-noui`), CI `ui` job with `oven-sh/setup-bun`, `-tags noui` for build-check jobs
- **Verified end-to-end**: 307 redirect `/` → `/ui/`, SPA HTML served at `/ui/`, fallback routing for `/ui/containers`, API still works alongside UI

**Tests**: 12 Vitest PASS (6 API client + 3 hooks + 3 DataTable) + 5 Go SPAHandler PASS

## Phase 74 — All Backend Dashboards + Docker Frontend

Rolled out UI dashboards to all 7 remaining backends + Docker frontend (9 new SPAs total). Key deliverables:

- **Shared BackendApp component**: Extracted 4 dashboard pages (Overview, Containers, Resources, Metrics) from `backend-memory` into `@sockerless/ui-core/pages`, created `BackendApp` component that assembles BrowserRouter + AppShell + Routes. Each new SPA is a thin wrapper (~30 lines of unique code)
- **BackendInfoCard**: New component showing backend-type badge, instance ID, and context from `/internal/v1/status`
- **6 cloud backend SPAs**: ECS, Lambda, CloudRun, GCF, ACA, AZF — each with `ui_embed.go`/`ui_noembed.go` + `registerUI()` in `server.go`
- **Docker backend SPA**: Added management endpoints (`/internal/v1/healthz`, `/status`, `/metrics`, `/containers/summary`, `/check`, `/resources`) to `backends/docker/` (non-BaseServer), SPA with build-tagged embed
- **Docker frontend SPA**: Custom SPA for MgmtServer showing docker_requests, goroutines, heap, configuration. Custom API client for non-standard paths (`/healthz`, `/status`, `/metrics`)
- **Build integration**: Makefile expanded with 18 new targets (`build-*-with-ui`, `build-*-noui`), `ui-build` copies dist/ for all 9 backends, CI build-check uses `-tags noui` for all 9 modules with embed

**Tests**: 16 Vitest PASS (+4: 2 BackendApp + 2 BackendInfoCard) + 5 Go SPAHandler PASS

## Phase 75 — Simulator Dashboards (AWS, GCP, Azure)

Added web dashboards to all 3 cloud simulators. Unlike backend dashboards (which share `BackendApp` + management endpoints), simulator dashboards are cloud-specific — each shows its own resource types via custom pages and `/sim/v1/` JSON summary endpoints.

- **SimulatorApp component**: New shared component in `@sockerless/ui-core` — like BackendApp but accepts custom `navItems` + `children` routes. New `useSimHealth()` and `useSimSummary()` hooks
- **SPAHandler in simulator shared libs**: Copied `spaHandler()` + `RegisterUI()` to each simulator's `shared/server.go` (avoids cross-module dependency on backend-core)
- **AWS SPA** (6 pages): Overview, ECS Tasks, Lambda Functions, ECR Repos, S3 Buckets, CloudWatch Log Groups
- **GCP SPA** (6 pages): Overview, Cloud Run Jobs, Cloud Functions, Artifact Registry, GCS Buckets, Cloud Logging
- **Azure SPA** (6 pages): Overview, Container Apps Jobs, Azure Functions, ACR Registries, Storage Accounts, Monitor Logs
- **Dashboard endpoints**: Each simulator gets `dashboard.go` with 6 JSON endpoints under `/sim/v1/` — avoids parsing complex cloud-native response formats in the browser
- **Store promotions**: GCP/Azure local stores promoted to package-level globals for dashboard access (AWS stores were already global)
- **Build integration**: Makefile `ui-build` copies 3 new dist/ dirs, `MODULES_SIM_UI` lint list with `GOWORK=off`, CI `build-check` uses `noui` for simulators

**Tests**: 18 Vitest PASS (+2 SimulatorApp) + 13 UI packages build + 3 simulators compile with noui

## Project Stats

- **75 phases** (1-67, 69-75), 677 tasks completed
- **16 Go modules** across backends, simulators, sandbox, agent, API, frontend, bleephub, gitlabhub, tests
- **21 Go-implemented builtins** in WASM sandbox
- **18 driver interface methods** across 5 driver types
- **7 external test consumers**: `act`, `gitlab-runner`, `gitlab-ci-local`, upstream act, `actions/runner`, `gh` CLI, gitlabhub gitlab-runner
- **Core tests**: 255 PASS (+5 SPAHandler) | **Frontend tests**: 7 PASS | **UI tests**: 18 PASS (Vitest) | **bleephub tests**: 298 PASS | **gitlabhub tests**: 129 PASS | **Shared ProcessRunner**: 15 PASS
- **Cloud SDK tests**: AWS 42, GCP 43, Azure 38 | **Cloud CLI tests**: AWS 26, GCP 21, Azure 19
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform — now with real process execution for all services (container + FaaS)
- **8 backends** sharing a common driver architecture
