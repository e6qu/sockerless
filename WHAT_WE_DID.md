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

## Bug Fix Sprints (BUG-001 → BUG-574)

574 bugs fixed across 44 sprints. Per-sprint details in `_tasks/done/BUG-SPRINT-*.md`.

| Sprint | Bugs | Focus |
|--------|------|-------|
| 1-6 | 001→046 | Admin UI: races, concurrency, error states, XSS, HTTP status codes |
| SimCmd | 047→051 | FaaS simulator command protocol → `SOCKERLESS_CMD` env var |
| 7-26 | 052→319 | Core lifecycle, cloud parity, Docker field mapping, API types, resource leaks |
| 27-32 | 320→408 | WaitCh close gaps, cloud events, pod lifecycle, TmpfsDirs cleanup, frontend params |
| 33-37 | 409→475 | Resize, stats, default networks, cloud logs, container inspect, image metadata |
| 38-40 | 476→514 | Stop/restart timeout, prune SpaceReclaimed, image save/build/search, Docker backend routes |
| 41 | 515→527 | ENV merge, Cmd/Entrypoint, stats, volume 200, events since/until, cloud errors, Dockerfile parser |
| 42 | 528→540 | Cloud/Docker/core bugs |
| 43 | 541→553 | Container/stats/event, cloud error & Dockerfile parser bugs |
| 44 | 554→574 | Docker inspect/list fields, cloud restart, KQL datetime, frontend routes |

0 open bugs remain — see `BUGS.md`.

## Sprint 44 Summary (BUG-554 → BUG-574)

Fixed 21 bugs across Docker backend, cloud backends, and frontend. Docker: logs Content-Type for TTY (554), NetworkSettings top-level scalars (555), EndpointSettings IPv6/IPAM/DriverOpts in inspect+list (556/557), ContainerSummary HostConfig (558), image push auth header (559), commit multi-value changes (560), resize 204 status (561), exec inspect missing fields (562), IPAM AuxiliaryAddresses (563), image healthcheck StartInterval (564), Ports null→empty (565), exec start stdin guard (574). Cloud: ECS restart stale TaskDef (566), CloudRun restart double-delete (567), KQL datetime quotes (568, 3 locations), ACA execution name guard (569). Frontend: container list before/since (570), restart signal (571), POST /_ping (572), POST /build/prune (573).

## Sprint 41 Summary (BUG-515 → BUG-527)

Fixed 13 bugs: ENV merge by key instead of all-or-nothing (BUG-515), clear image Cmd when Entrypoint overridden (BUG-516), stats `id`/`name` fields (BUG-517), stats `precpu_stats` tracks previous reading (BUG-518), volume create 200 for existing (BUG-519), events `since`/`until` with history replay (BUG-520), ECS/CloudRun/ACA cloud error mapping (BUG-521/522/523), image save 404 on missing (BUG-524), `parseLabels` quote-aware tokenizer (BUG-525), `parseEnv` multi-value support (BUG-526), `NanoCpus` field in HostConfig (BUG-527).

## Sprint 33 Summary (BUG-409 → BUG-422)

14 bugs fixed in core backend parity: added `POST /containers/{id}/resize` and `POST /exec/{id}/resize` no-op endpoints (BUG-409), added `precpu_stats` to stats response (BUG-410), stats memory limit uses container's `HostConfig.Memory` when set (BUG-411), `InitDefaultNetwork` now creates `host` and `none` networks alongside `bridge` (BUG-412), added commit/update/load event emissions (BUG-413/414/415), container and volume prune now calculate `SpaceReclaimed` from filesystem (BUG-416/417), deterministic `ImageID` for unknown images using sha256 of image name (BUG-418), `FormatStatus` for paused state returns "Up X (Paused)" (BUG-419), health check exec instances deleted after completion (BUG-420), `handleInfo` now counts `ContainersPaused` separately (BUG-421), `handleContainerLogs` respects `stdout`/`stderr` query params (BUG-422).

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

## Sprint 34 Summary (BUG-423 → BUG-436)

14 bugs fixed in cloud backend logs parity and two core issues. Created shared `CloudLogParams` helper in `backends/core/log_cloud.go` with `ParseCloudLogParams`, `FormatLine`, `ApplyTail`, `FilterBufferedOutput`, `WriteMuxLine`, and cloud-specific filter helpers (CloudWatch millis, Cloud Logging timestamp, KQL datetime). All 6 cloud backends now support `since`/`until` timestamp filtering (BUG-423/424), `stdout`/`stderr` suppression (BUG-426), and `details` label prepending (BUG-427). CloudRun, GCF, ACA, AZF now support `tail` via client-side slicing (BUG-425). Lambda, GCF, AZF gained follow-mode polling (BUG-428/429/430). ECS/CloudRun/ACA follow-mode queries no longer apply `since`/`until` (BUG-433). ACA follow polling changed from 2s to 1s for consistency (BUG-434). FaaS LogBuffers output now filtered through params (BUG-435). Core `ImageSummary.Containers` now counts containers per image in both `handleImageList` (BUG-431) and `handleSystemDf` (BUG-436). Health check loop now uses `StartInterval` during start period (BUG-432).

## Sprint 35 Summary (BUG-437 → BUG-449)

13 bugs fixed across container inspect, NetworkSettings, system info, exec lifecycle, frontend proxy, image metadata, and volume usage. Container inspect now populates `LogPath`, `ResolvConfPath`, `HostnamePath`, `HostsPath` (BUG-437–440). `NetworkSettings.SandboxID` and `SandboxKey` set from container ID (BUG-441/442). `NetworkSettings.Bridge` set to `docker0` for bridge network containers (BUG-443). `handleInfo` now returns `KernelVersion: "5.15.0-sockerless"` (BUG-444). `ExecInstance.CanRemove` set to true after exec completes or errors (BUG-445). Frontend `handleSystemDf` forwards `type` query param to backend (BUG-446). `handleImageLoad` now sets `RootFS.Layers` with a synthetic layer hash (BUG-447). Both `handleImagePull` and `handleImageLoad` now populate `GraphDriver` with overlay2 metadata (BUG-448). `handleSystemDf` volumes now include `UsageData` with `RefCount` and `Size` (BUG-449).

## Sprint 36 Summary (BUG-450 → BUG-462)

13 bugs fixed in system df, commit/build, and container field gaps. System df ImageSummary now sets `VirtualSize` and `Labels` (BUG-450/451). Container list populates `SizeRw` from `DirSize` when `size=true` (BUG-452). Committed images get synthetic `RootFS.Layers` and `GraphDriver` overlay2 metadata (BUG-453/454). Built images get `GraphDriver` (BUG-455). System df ContainerSummary now includes `ImageID`, `Command`, `Status`, `Labels`, `Ports`, `Mounts`, `NetworkSettings`, `HostConfig`, and `SizeRootFs` (BUG-456–461). `buildContainerFromConfig` sets `Container.Image` to sha256 image ID instead of reference name (BUG-462).

## Sprint 37 Summary (BUG-463 → BUG-475)

13 bugs fixed across image endpoints, stats, container inspect, update request, and frontend forwarding. `handleImageHistory` now returns per-layer entries from `RootFS.Layers` instead of a single hardcoded entry (BUG-463). `handleImageSave` manifest uses actual `RootFS.Layers` instead of empty array (BUG-464). `buildStatsEntry` populates `networks` field from container `NetworkSettings` (BUG-465). API `Container` struct gets `SizeRw`/`SizeRootFs` pointer fields (BUG-466). Core `handleContainerInspect` respects `size` query parameter (BUG-467), forwarded by frontend (BUG-468). Frontend `handleVersion` uses `info.KernelVersion` instead of empty string (BUG-469). `ContainerUpdateRequest` gets `PidsLimit` (BUG-470) and `OomKillDisable` (BUG-471) fields. Frontend `handleImagePush` forwards `X-Registry-Auth` header as query param (BUG-472), core accepts it (BUG-473). Image `Metadata.LastTagTime` set on pull/load/build/commit/tag operations (BUG-474). `handleImageSearch` respects `limit` query parameter (BUG-475).

## Sprint 38 Summary (BUG-476 → BUG-488)

13 bugs fixed across core and all 6 cloud backends. Core `handleContainerStop` and `handleContainerRestart` now accept the `t` (timeout) query parameter for API parity (BUG-476/477). All 6 cloud backends (`handleContainerPrune`) now sum image sizes for `SpaceReclaimed` instead of hardcoding 0 (BUG-478→483). The 3 container-service backends (ECS, CloudRun, ACA) `handleVolumePrune` now sum volume directory sizes for `SpaceReclaimed` (BUG-484→486). Core `handleContainerWait` now handles `condition=removed` — returns exit code 0 if the container is already gone, and polls briefly for actual deletion after stop (BUG-487). Core `handleImageSave` now writes image config JSON entries (architecture, os, created, config, rootfs) alongside manifest.json so `docker load` can parse the full image metadata (BUG-488).

## Sprint 42 Summary (BUG-528 → BUG-540)

Fixed 13 bugs across cloud backends, Docker backend, and core. All 3 cloud container backends (ECS, CloudRun, ACA) now use key-based ENV merge instead of all-or-nothing replacement (BUG-528), and clear inherited Cmd when Entrypoint is overridden (BUG-529). Added `mapAWSError`/`mapGCPError`/`mapAzureError` to ECS/CloudRun/ACA `errors.go` — raw cloud SDK errors are now mapped to proper Docker API error types (BUG-530/531/532). Docker backend `handleContainerCreate` now maps 26 additional HostConfig fields: DNS, memory/CPU resources, PidMode, IpcMode, UTSMode, VolumesFrom, GroupAdd, ReadonlyRootfs, OomKillDisable, PidsLimit, Sysctls, Runtime, Links, PublishAllPorts, CgroupnsMode, ConsoleSize (BUG-533). Same 26 fields added to `mapContainerFromDocker` inspect output (BUG-534). Docker `handleNetworkConnect` returns 204 instead of 200 (BUG-535). Core `handleContainerCommit` now accepts `changes` query param with Dockerfile instructions (CMD, ENTRYPOINT, ENV, WORKDIR, USER, LABEL, EXPOSE) and `pause` param (BUG-536/537). GCF `http.Post` replaced with 10-minute timeout client (BUG-538). Docker `handleContainerCommit` passes `changes` and `pause` to Docker SDK (BUG-539). Docker `handleImagePull` passes `Platform` field (BUG-540). Added `NanoCpus` to `api.HostConfig`. Created `FEATURE_MATRIX.md` — comprehensive Docker API compatibility matrix across all 9 backends.

## Sprint 43 Summary (BUG-541 → BUG-553)

Fixed 13 bugs across FaaS backends (Lambda, GCF, AZF), Docker backend, and core. All 3 FaaS backends now use key-based ENV merge via exported `core.MergeEnvByKey` instead of all-or-nothing replacement (BUG-541), and clear inherited Cmd when Entrypoint is overridden (BUG-542). Docker backend `handleImageTag` now returns 200 OK instead of 201 Created (BUG-543). Docker backend Healthcheck mapping now includes `StartInterval` in both create (BUG-544) and inspect (BUG-545). Docker `handleContainerCreate` now maps `ArgsEscaped`, `NetworkDisabled`, `OnBuild` (BUG-546) and `MacAddress` (BUG-552) to Docker SDK. Docker `mapContainerFromDocker` inspect now maps the same fields back (BUG-547, BUG-553). Docker Mount inspect now maps `VolumeOptions` and `TmpfsOptions` (BUG-548) — create path already had them. Core `Store` now has an atomic `NextPID()` counter replacing hardcoded `Pid=42` in container start/restart/pod-start and `Pid=43` in exec start (BUG-549, BUG-550). Core `handleImagePull` now generates deterministic image sizes from FNV hash of image reference (10-100MB range) instead of hardcoded 7654321 (BUG-551).

## Sprint 40 Summary (BUG-502 → BUG-514)

Fixed 13 bugs: frontend attach TTY content-type `raw-stream` (BUG-502), exec create empty `Cmd` validation (BUG-503), container top `ps_args` param (BUG-504), container stop `signal` param with exit code (BUG-505), frontend container create `platform` forwarding (BUG-506), frontend container remove `link` forwarding (BUG-507), Docker backend image push route (BUG-508), Docker backend image save routes (BUG-509), Docker backend image search route (BUG-510), Docker backend image build route (BUG-511), Docker backend archive routes (BUG-512), image search result sorting by relevance (BUG-513), frontend container start `detachKeys` forwarding (BUG-514).

## Sprint 39 Summary (BUG-489 → BUG-501)

Fixed 13 bugs: container `expose` filter (BUG-489), image `before`/`since` list filters (BUG-490/491), frontend image load `quiet` param forwarding (BUG-492), core image load `quiet` suppression (BUG-493), image push `auth` query param (BUG-494), container resize `h`/`w` params (BUG-495), exec resize `h`/`w` params (BUG-496), container top synthetic PID from `c.State.Pid` (BUG-497), frontend image create `fromSrc` repo/tag forwarding (BUG-498), image tag empty repo validation (BUG-499), volume list `dangling` filter (BUG-500), exec start TTY content-type `raw-stream` (BUG-501).

## Project Stats

- **80 phases** (1-67, 69-77, 79-82), 725 tasks completed
- **43 bug sprints**, 553 bugs fixed (BUG-001→553), 0 open
- **18 Go modules** across backends, simulators, sandbox, agent, API, frontend, bleephub, gitlabhub, CLI, admin, tests
- **Core tests**: 302 PASS | **Frontend**: 7 | **UI (Vitest)**: 92 | **Admin**: 88 | **bleephub**: 304 | **gitlabhub**: 136 | **ProcessRunner**: 15
- **Cloud SDK**: AWS 42, GCP 43, Azure 38 | **Cloud CLI**: AWS 26, GCP 21, Azure 19
- **E2E**: 371 GitHub+GitLab workflows | **Sim-backend**: 75 | **Terraform**: 75 | **Upstream**: 252
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform
- **8 backends** sharing a common driver architecture
