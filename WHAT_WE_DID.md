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

## Bug Fix Sprints (BUG-001 → BUG-408)

382 bugs fixed across 32 sprints. Per-sprint details in `_tasks/done/BUG-SPRINT-*.md`.

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
| 28 | BUG-337→344 | ECS ClusterARN, restart task/job leaks, pod kill signal, image tag dedup, frontend stats one-shot |
| 29 | BUG-345→358 | Cloud restart stop event + RestartCount, remove/prune destroy event + pod cleanup, core pod stop/kill events |
| 30 | BUG-359→377 | Cloud StopHealthCheck gaps, create event, signalToExitCode, force-remove events, Network.Disconnect, core prune events, pod lifecycle, FormatStatus uptime, Event.Scope, restart event |
| 31 | BUG-378→394 | Container & pod parity: pod ProcessLifecycle/HealthCheck, pod remove cleanup, pod wait condition, container filter gaps (exited/publish/volume/is-task/size) |
| 32 | BUG-395→408 | Cloud kill event ordering, TmpfsDirs cleanup (remove+prune), frontend query param forwarding (size/signal/platform/digests/noOverwrite/force), pod list filters, image push/save/search |

0 open bugs remain — see `BUGS.md`.

## Sprint 32 Summary (BUG-395 → BUG-408)

14 bugs fixed. Three categories: (1) Cloud backend correctness — all 6 cloud backends had `handleContainerKill` closing WaitChs before emitting "kill"+"die" events, so event watchers missed events (BUG-395); `handleContainerRemove` (BUG-396) and `handleContainerPrune` (BUG-397) were missing `TmpfsDirs.LoadAndDelete` + `os.RemoveAll` cleanup, leaking tmpfs temp directories. (2) Frontend query parameter forwarding — 7 endpoints were silently dropping parameters: `size` in container list (BUG-398), `signal` in container stop (BUG-402), `platform` in image pull (BUG-403), `shared-size`/`digests` in image list (BUG-404), `noOverwriteDirNonDir` in put archive (BUG-405), `force` in network remove (BUG-406). (3) New endpoints — `handleImagePush` was a hardcoded stub returning fake push output; now proxies to core backend which returns actual image data (BUG-399). Core pod list now supports `filters` query parameter with name/id/label/status matching (BUG-400), forwarded from frontend (BUG-401). Added `GET /images/get` for `docker save` (BUG-407) returning tar with manifest.json. Added `GET /images/search` (BUG-408) replacing the NotImplemented stub with local image store search.

## Sprint 27 Summary (BUG-320 → BUG-336)

17 bugs fixed. The dominant pattern was `WaitChs.Delete` calls that deleted the channel from the map without closing it first, leaving any goroutine blocked on `<-ch` waiting forever. This affected `handleContainerRemove` and `handleContainerPrune` across all six cloud backends (ECS, CloudRun, ACA, Lambda, GCF, AZF) plus two core paths (`handleContainerRemove` and `store.RevertToCreated`) — 14 fixes in total. Two additional correctness bugs were fixed: ACA `handleContainerRestart` called `MarkCleanedUp` without an empty-check guard and deleted container state prematurely before the re-create sequence completed (BUG-334); and Docker `handleContainerCommit` constructed an image reference as `"repo:"` when the tag parameter was empty, producing an invalid ref that would be rejected by image stores (BUG-335). Finally, the frontend `handleContainerLogs` was not forwarding the `details` query parameter to the backend, silently dropping it for any client that set it (BUG-336).

## Sprint 28 Summary (BUG-337 → BUG-344)

8 bugs fixed. Three ECS inconsistencies: `handleContainerRemove` and `handleContainerRestart` used hardcoded `s.config.Cluster` instead of the `ClusterARN` fallback pattern already used by stop/kill (BUG-337, BUG-338), and `handleContainerRestart` didn't deregister the old task definition, leaking it in ECS (BUG-339). Two container-backend restart resource leaks: CloudRun and ACA `handleContainerRestart` called `MarkCleanedUp` without first calling `deleteJob()`, leaving the old Cloud Run job or ACA job resource alive in the cloud (BUG-340, BUG-341). Core `handlePodKill` hardcoded exit code 137 regardless of the `signal` query parameter — now uses `signalToExitCode()` matching `handleContainerKill` (BUG-342). `handleImageTag` appended to `RepoTags` without checking for duplicates, causing the same tag to appear multiple times (BUG-343). Frontend `handleContainerStats` didn't forward the `one-shot` query parameter to the backend (BUG-344).

## Sprint 29 Summary (BUG-345 → BUG-358)

14 bugs fixed. All 6 cloud backend restart handlers emitted "die" but not "stop" event (BUG-345→350) and didn't increment `RestartCount` (BUG-351) — core does both. All 6 cloud backend `handleContainerRemove` missing "destroy" event (BUG-352) and pod cleanup via `Pods.RemoveContainer` (BUG-356). All 6 cloud backend `handleContainerPrune` missing "destroy" event (BUG-353) and pod cleanup (BUG-357). Core `handlePodStop` missing "die" and "stop" events per container (BUG-354). Core `handlePodKill` missing "kill" and "die" events per container (BUG-355). Core `handlePodRemove` with `force=false` didn't clean up exited containers — LogBuffers, WaitChs, StagingDirs, TmpfsDirs, and ExecIDs leaked (BUG-358).

## Sprint 31 Summary (BUG-378 → BUG-394)

17 bugs fixed. Core `handlePodStart` was missing `ProcessLifecycle.Start()` and `StartHealthCheck()` calls — containers were marked running but no process spawned (BUG-378→379). Core `handlePodRemove` non-force path was missing `StopHealthCheck`, `ProcessLifecycle.Cleanup`, `Network.Disconnect` loop, and "destroy" events (BUG-380→383). Core `handleContainerWait` ignored the `condition` query parameter — now supports `not-running`, `next-exit`, and `removed` (BUG-384). Core `MatchContainerFilters` was missing `exited` (exit code), `publish` (port), `volume` (mount), and `is-task` filters (BUG-385, 391→393). All 6 cloud `handleContainerCreate` were missing `?pod=` query parameter validation and `Pods.AddContainer()` (BUG-386). All 6 cloud `handleContainerRestart` emitted "restart" event unconditionally — now uses `httptest.ResponseRecorder` to only emit on success (BUG-387). Frontend `handlePodKill` and `handlePodStop` weren't forwarding `signal` and `t` query parameters to backend (BUG-388→389). Core `handleContainerLogs` wasn't handling the `details` query parameter (BUG-390). Core `handleContainerList` wasn't handling the `size` query parameter (BUG-394).

## Sprint 30 Summary (BUG-359 → BUG-377)

18 bugs fixed. All 6 cloud backends were missing `StopHealthCheck(id)` in stop/kill/remove handlers (BUG-359→361), missing "create" event in `handleContainerCreate` (BUG-362), had a simplified `signalToExitCode` that only handled SIGKILL — now maps 8 signals with 24 aliases (BUG-363), missing "kill"+"die" events in force-remove path (BUG-364), and missing `Network.Disconnect` loop in remove/prune (BUG-365→366). Core `handleImagePrune` was missing `BuildContexts` cleanup and untag/delete events (BUG-367→368). Core `handleVolumePrune` and `handleNetworkPrune` were missing destroy events (BUG-369→370). Core `handlePodKill` was missing `ProcessLifecycle.Cleanup` (BUG-371). Core `handlePodRemove` force path was missing "destroy" events per container (BUG-372). Core `handlePodStart` didn't reset `FinishedAt`/`ExitCode` (BUG-373). `FormatStatus` was hardcoded — now computes actual uptime (BUG-374). `Event.Scope` was never set to "local" (BUG-375). `handleContainerRestart` was missing "restart" event in core and all 6 cloud backends (BUG-376).

## Project Stats

- **80 phases** (1-67, 69-77, 79-82), 725 tasks completed
- **30 bug sprints**, 351 bugs fixed (BUG-001→377), 0 open
- **18 Go modules** across backends, simulators, sandbox, agent, API, frontend, bleephub, gitlabhub, CLI, admin, tests
- **Core tests**: 302 PASS | **Frontend**: 7 | **UI (Vitest)**: 92 | **Admin**: 88 | **bleephub**: 304 | **gitlabhub**: 136 | **ProcessRunner**: 15
- **Cloud SDK**: AWS 42, GCP 43, Azure 38 | **Cloud CLI**: AWS 26, GCP 21, Azure 19
- **E2E**: 371 GitHub+GitLab workflows | **Sim-backend**: 75 | **Terraform**: 75 | **Upstream**: 252
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform
- **8 backends** sharing a common driver architecture
