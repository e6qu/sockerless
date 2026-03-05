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

## Completed Phases (1-82)

| Phase | What |
|---|---|
| 1-10 | Foundation: 3 simulators, 8 backends, agent, Docker REST API frontend |
| 11-34 | WASM sandbox, E2E tests (217 GitHub + 154 GitLab), driver interfaces, Docker build |
| 35-42 | bleephub: GitHub API + runner + multi-job engine (190 unit tests) |
| 43-52 | CLI, crash safety, pods, service containers, upstream expansion |
| 53-56 | Production Docker API: TLS, auth, logs, DNS, restart, events, filters, export, commit |
| 57-59 | Production GitHub Actions: multi-job, matrix, secrets, expressions, concurrency |
| 60-61 | Production GitLab CI: gitlabhub coordinator, DAG engine, expressions, extends, include |
| 62-63 | Docker API hardening + Compose E2E: HEALTHCHECK, volumes, mounts, prune, directives |
| 64-65 | bleephub: Webhooks (HMAC-SHA256) + GitHub Apps (JWT, installation tokens) |
| 66 | OTel tracing: OTLP HTTP, otelhttp middleware, context propagation |
| 67 | Network Isolation: IPAllocator, SyntheticNetworkDriver, Linux NetnsManager |
| 69 | ARM64/Multi-Arch: goreleaser 15 builds, docker.yml 7 images |
| 70-72 | Simulator Fidelity + SDK/CLI Verification + Full-Stack E2E (real process execution) |
| 73-75 | UI: Bun/Vite/React 19 monorepo, 10 backend SPAs, 3 simulator SPAs, SPAHandler |
| 76-77 | bleephub + gitlabhub dashboards with management endpoints and LogViewer |
| 79 | Admin Dashboard: standalone server + SPA, health polling, context discovery |
| 80 | Documentation review + tutorial verification |
| 81 | Admin: ProcessManager, cleanup scanner, ProviderInfo |
| 82 | Admin Projects: orchestrated sim+backend+frontend bundles, port allocator, 4 UI pages |

## Bug Fix Sprints (BUG-001 → BUG-336)

311 bugs fixed across 27 sprints. Per-sprint details in `_tasks/done/BUG-SPRINT-*.md`.

| Sprint | Bugs | Focus |
|--------|------|-------|
| 1-6 | BUG-001→046 | Admin UI: races, concurrency, error states, XSS, HTTP status codes |
| SimCmd | BUG-047→051 | FaaS simulator command protocol → `SOCKERLESS_CMD` env var |
| 7 | BUG-052→062 | Core: tar corruption, error swallowing, cloud resource leaks |
| 9 | BUG-063→068 | API types (`*bool`), cloud state revert, cloud resource cleanup |
| 10 | BUG-069→074 | Image store aliases, FaaS kill/prune lifecycle, Docker Mounts |
| 11 | BUG-075→082 | Lambda restart, Docker inspect/list field mapping (5 bugs) |
| 12 | BUG-083→090 | Docker create mapping (21 fields), FaaS pause, core events |
| 13 | BUG-091→098 | Docker NetworkingConfig, LogBuffers leak, 5 Docker field gaps |
| 14 | BUG-099→106 | FaaS stop state, ECS restart, Docker params, volume prune |
| 15 | BUG-107→114 | Pod cleanup, CloudRun/ACA Args, Docker auth/filters |
| 16 | BUG-115→122 | Tar traversal, prune cleanup, cloud AgentRegistry, Docker events/df |
| 17 | BUG-123→130 | Start revert, kill signals, exec ordering, image dedup, Docker df/auth |
| 18 | BUG-131→138 | Core restart (health/events/stale), ImageID, image aliases, AgentRegistry leak, FaaS restart, Docker list params |
| 19 | BUG-139→157 | Core lifecycle (stop/restart/start/exec), cloud AgentRegistry leaks, Docker exec detach, frontend attach |
| 20 | BUG-158→176 | Core kill/stop events, cloud restart parity, AgentRegistry leak, API types |
| 21 | BUG-177→201 | Resource leaks, cloud parity, Docker field mapping, lifecycle safety |
| 22 | BUG-202→226 | Core lifecycle safety, Docker API parity, API type gaps, frontend conformance |
| 23 | BUG-227→251 | Forward agent fix (CloudRun/ACA), Docker parity, lifecycle safety |
| 24 | BUG-252→269 | Final 18: BuildCache, FaaS image config, events, image load, LRO waits, API types |
| 25 | BUG-270→294 | Core lifecycle, API serialization, cloud parity, Docker field mapping |
| 26 | BUG-295→319 | WaitCh leaks, HTTP status codes, symlink traversal, cloud events, API types |
| 27 | BUG-320→336 | WaitChs.Delete close gaps (all 8 backends), ACA restart guard, Docker commit ref, frontend logs query param |

0 open bugs remain — see `BUGS.md`.

## Sprint 27 Summary (BUG-320 → BUG-336)

17 bugs fixed. The dominant pattern was `WaitChs.Delete` calls that deleted the channel from the map without closing it first, leaving any goroutine blocked on `<-ch` waiting forever. This affected `handleContainerRemove` and `handleContainerPrune` across all six cloud backends (ECS, CloudRun, ACA, Lambda, GCF, AZF) plus two core paths (`handleContainerRemove` and `store.RevertToCreated`) — 14 fixes in total. Two additional correctness bugs were fixed: ACA `handleContainerRestart` called `MarkCleanedUp` without an empty-check guard and deleted container state prematurely before the re-create sequence completed (BUG-334); and Docker `handleContainerCommit` constructed an image reference as `"repo:"` when the tag parameter was empty, producing an invalid ref that would be rejected by image stores (BUG-335). Finally, the frontend `handleContainerLogs` was not forwarding the `details` query parameter to the backend, silently dropping it for any client that set it (BUG-336).

## Sprint 28 Summary (BUG-337 → BUG-344)

8 bugs fixed. Three ECS inconsistencies: `handleContainerRemove` and `handleContainerRestart` used hardcoded `s.config.Cluster` instead of the `ClusterARN` fallback pattern already used by stop/kill (BUG-337, BUG-338), and `handleContainerRestart` didn't deregister the old task definition, leaking it in ECS (BUG-339). Two container-backend restart resource leaks: CloudRun and ACA `handleContainerRestart` called `MarkCleanedUp` without first calling `deleteJob()`, leaving the old Cloud Run job or ACA job resource alive in the cloud (BUG-340, BUG-341). Core `handlePodKill` hardcoded exit code 137 regardless of the `signal` query parameter — now uses `signalToExitCode()` matching `handleContainerKill` (BUG-342). `handleImageTag` appended to `RepoTags` without checking for duplicates, causing the same tag to appear multiple times (BUG-343). Frontend `handleContainerStats` didn't forward the `one-shot` query parameter to the backend (BUG-344).

## Project Stats

- **80 phases** (1-67, 69-77, 79-82), 725 tasks completed
- **28 bug sprints**, 319 bugs fixed (BUG-001→344), 0 open
- **18 Go modules** across backends, simulators, sandbox, agent, API, frontend, bleephub, gitlabhub, CLI, admin, tests
- **Core tests**: 302 PASS | **Frontend**: 7 | **UI (Vitest)**: 92 | **Admin**: 88 | **bleephub**: 304 | **gitlabhub**: 136 | **ProcessRunner**: 15
- **Cloud SDK**: AWS 42, GCP 43, Azure 38 | **Cloud CLI**: AWS 26, GCP 21, Azure 19
- **E2E**: 371 GitHub+GitLab workflows | **Sim-backend**: 75 | **Terraform**: 75 | **Upstream**: 252
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform
- **8 backends** sharing a common driver architecture
