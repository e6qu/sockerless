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

## Phases 57-67 — CI Runners, API Hardening, bleephub Features, OTel, Network Isolation

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
| 67 | Network Isolation: IPAllocator, SyntheticNetworkDriver, Linux NetnsManager | 255 core PASS (+14) |

## Phases 69-72 — ARM64, Simulator Fidelity, SDK Verification, Full-Stack E2E

| Phase | What | Key Results |
|---|---|---|
| 69 | ARM64/Multi-Arch: goreleaser 15 builds, docker.yml 7 images | CI build-check 15 binaries + ARM64 cross-compile |
| 70 | Simulator Fidelity: real process execution, structured logs | SDK: AWS 8→21, GCP 8→23, Azure 7→16. ProcessRunner: 15 PASS |
| 71 | SDK/CLI Verification: FaaS real execution, arithmetic evaluator | SDK: AWS 42, GCP 43, Azure 38. CLI: AWS 26, GCP 21, Azure 19 |
| 72 | Full-Stack E2E: arithmetic through Docker API stack | sim-test-all: 75 PASS, test-e2e: 65 PASS |

## Phase 68 — Multi-Tenant Backend Pools (In Progress)

P68-001 done: `PoolConfig`/`PoolsConfig` types, `ValidatePoolsConfig()` (8 rules), `LoadPoolsConfig()`, 18 tests. 9 tasks remaining.

## Phases 73-75 — UI Foundation, Backend Dashboards, Simulator Dashboards

**Phase 73**: Bun/Vite/React 19/Tailwind 4 monorepo, shared core package (API client, 7 hooks, 7 components), memory backend SPA (4 pages), Go `SPAHandler` with embed. 12 Vitest + 5 Go SPAHandler PASS.

**Phase 74**: Rolled out dashboards to all 7 remaining backends + Docker frontend (9 new SPAs). Shared `BackendApp` component, `BackendInfoCard`, Makefile with 18 new targets, CI `-tags noui`. 16 Vitest PASS.

**Phase 75**: 3 simulator SPAs (AWS/GCP/Azure) with cloud-specific resource pages, `/sim/v1/` summary endpoints, `SimulatorApp` component. Store promotions for dashboard access. 18 Vitest PASS.

## Phases 76-77 — bleephub & gitlabhub Dashboards

**Phase 76**: bleephub SPA (6 pages), 5 Go management endpoints, log capture (500 lines/job), shared `LogViewer` component with ANSI→CSS. 6 Go mgmt + 16 Vitest + 3 LogViewer PASS.

**Phase 77**: gitlabhub SPA (6 pages), 5 Go management endpoints, stage-grouped pipeline view (stages left-to-right, jobs stacked vertically). 7 Go mgmt + 16 Vitest + 5 Playwright E2E PASS.

## Phases 79-82 — Admin Dashboard, Docs, Process Management, Projects

**Phase 79**: Standalone `sockerless-admin` server + SPA (7 pages), component registry, health polling, `/api/v1/` endpoints, context discovery. 9 Go + 4 Vitest + 17 Playwright E2E PASS.

**Phase 80**: Fixed stale docs, updated test counts, verified quick-starts, fixed bleephub README. 8 tasks.

**Phase 81**: ProcessManager (Start/Stop/StopAll with ring buffer logs), cleanup scanner (orphaned PIDs, stale tmp, stopped containers, stale cloud resources), `ProviderInfo` on all 8 backends. 22 new Go + 11 Vitest PASS.

**Phase 82**: Project bundles (sim+backend+frontend), `PortAllocator`, orchestrated startup with rollback, JSON persistence, 8 API endpoints, 4 UI pages (list/create/detail/logs). 39 Go + 18 Vitest PASS.

## Bug Fix Sprints (BUG-001→051)

| Sprint | Bugs Fixed | Key Changes | Test Delta |
|---|---|---|---|
| 1 | BUG-001→020 | LoggingMiddleware, race fixes, project name validation, RingBuffer carry-over, error states on 8 UI pages, per-row pending state | 70→77 Go, 86→86 Vitest |
| 2 | BUG-003→016 (14) | opLock concurrency, graceful HTTP shutdown, LogViewer XSS fix, DataTable onRowClick, confirm guards | 77→83 Go, 86→89 Vitest |
| 3 | BUG-003→023 (21) | StopAll bypass opLock, context cancel cleanup, processErrorStatus, ANSI rewrite, URL encoding on 14 endpoints, empty states | 83→86 Go, 89→92 Vitest |
| 4 | BUG-024→033 (10) | HTTP status codes (409 Conflict), ScanStoppedContainers age, RingBuffer negative guard, auto-refresh | 86→87 Go, 92 Vitest |
| 5 | BUG-034→042 (9) | Stop/Start race (generation check), start/stop button guards, 404 route, concurrent error display | 87→88 Go, 92 Vitest |
| 6 | BUG-043→046 (4) | "stopping" state detection, error display fix, health badge mapping, provider cache invalidation | 88 Go, 92 Vitest |

## Simulator Command Protocol Cleanup (BUG-047→051)

Eliminated simulator-specific command protocol from FaaS backends (Lambda/GCF/AZF). Replaced `SimCommand`/`ImageConfig.Command` JSON fields with standard `SOCKERLESS_CMD` env var. Configurable `AgentTimeout` (default 30s, tests use 5s). 5 bugs fixed, all 75 sim-backend tests pass.

## Bug Audit: api, backends, frontends (BUG-052→062)

Audited `api/`, `backends/core/`, all 8 backend implementations, and `frontends/docker/` for correctness bugs. Found and fixed 9 real bugs:
- **High**: `extractTar` silent file corruption (BUG-052), `handlePutArchive` swallowing errors (BUG-053), network prune filter not forwarded (BUG-058)
- **Medium**: `mergeStagingDir` silent errors (BUG-054), `createTar` ignoring write errors (BUG-055), commit JSON decode error ignored (BUG-059), buildargs unmarshal error ignored (BUG-060), ECS task definition leak (BUG-062)
- **Low**: Agent drivers ignoring container-not-found (BUG-061)

Changed `createTar` signature to return `error`, updated 5 callers. Added 10 new tests. All 286 core tests pass, 0 lint issues across 19 modules.

## Project Stats

- **80 phases** (1-67, 69-77, 79-82), 725 tasks completed
- **18 Go modules** across backends, simulators, sandbox, agent, API, frontend, bleephub, gitlabhub, CLI, admin, tests
- **21 Go-implemented builtins** in WASM sandbox
- **18 driver interface methods** across 5 driver types
- **7 external test consumers**: `act`, `gitlab-runner`, `gitlab-ci-local`, upstream act, `actions/runner`, `gh` CLI, gitlabhub gitlab-runner
- **Core tests**: 286 PASS (+5 SPAHandler) | **Frontend tests**: 7 PASS | **UI tests**: 92 PASS (Vitest) | **Admin tests**: 88 PASS | **bleephub tests**: 304 PASS | **gitlabhub tests**: 136 PASS | **Shared ProcessRunner**: 15 PASS
- **Cloud SDK tests**: AWS 42, GCP 43, Azure 38 | **Cloud CLI tests**: AWS 26, GCP 21, Azure 19
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform — now with real process execution for all services (container + FaaS)
- **8 backends** sharing a common driver architecture
