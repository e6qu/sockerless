# Sockerless — What We Built

## The Idea

Sockerless presents an HTTP REST API identical to Docker's. CI runners (GitHub Actions via `act`, GitLab Runner, `gitlab-ci-local`) talk to it as if it were Docker, but instead of running containers locally, Sockerless farms work to cloud backends — ECS, Lambda, Cloud Run, Cloud Functions, Azure Container Apps, Azure Functions — or passes through to a local Docker daemon.

For development and testing, cloud simulators stand in for real AWS/GCP/Azure APIs, providing actual execution of tasks the same way a real cloud would. The simulators are validated against official cloud SDKs, CLIs, and Terraform providers.

## Architecture

```
CI Runner (act, gitlab-runner, gitlab-ci-local)
    │
    ▼
Frontend (Docker REST API)
    │
    ▼
Backend (ecs | lambda | cloudrun | gcf | aca | azf | docker)
    │
    ▼
Cloud Simulator (AWS | GCP | Azure)
Agent (inside container or reverse-connected)
```

**7 backends** share a common core (`backends/core/`) with driver interfaces:
- **ExecDriver** — runs commands inside containers via forward or reverse agent connections
- **FilesystemDriver** — manages container filesystem (staging dirs, agent bridge, archive ops)
- **StreamDriver** — attach/logs streaming (pipes, WebSocket relay, log buffer)
- **NetworkDriver** — IP allocation, network create/connect/disconnect (synthetic + Linux netns)

Handlers dispatch through drivers. Cloud backends delegate image management to per-cloud shared modules (`aws-common`, `gcp-common`, `azure-common`).

**3 simulators** (`simulators/{aws,gcp,azure}/`) implement enough cloud API surface for the backends to work. Each is tested against the real SDK, CLI, and Terraform provider for that cloud.

## Completed Phases (1-90)

| Phase | What |
|---|---|
| 1-10 | Foundation: 3 simulators, 8 backends, agent, Docker REST API frontend |
| 11-34 | E2E tests (217 GitHub + 154 GitLab), driver interfaces, Docker build |
| 35-42 | bleephub: GitHub API + runner + multi-job engine (190 unit tests) |
| 43-52 | CLI, crash safety, pods, service containers, upstream expansion |
| 53-56 | Production Docker API: TLS, auth, logs, DNS, restart, events, filters, export, commit |
| 57-59 | Production GitHub Actions: multi-job, matrix, secrets, expressions, concurrency |
| 60-61 | Production GitLab CI: coordinator, DAG engine, expressions, extends, include |
| 62-63 | Docker API hardening + Compose E2E: HEALTHCHECK, volumes, mounts, prune, directives |
| 64-65 | bleephub: Webhooks (HMAC-SHA256) + GitHub Apps (JWT, installation tokens) |
| 66 | OTel tracing: OTLP HTTP, otelhttp middleware, context propagation |
| 67 | Network Isolation: IPAllocator, SyntheticNetworkDriver, Linux NetnsManager |
| 69 | ARM64/Multi-Arch: goreleaser 15 builds, docker.yml 7 images |
| 70-72 | Simulator Fidelity + SDK/CLI Verification + Full-Stack E2E (real process execution) |
| 73-75 | UI: Bun/Vite/React 19 monorepo, 10 backend SPAs, 3 simulator SPAs, SPAHandler |
| 76-77 | bleephub dashboard with management endpoints and LogViewer |
| 79 | Admin Dashboard: standalone server + SPA, health polling, context discovery |
| 80 | Documentation review + tutorial verification |
| 81 | Admin: ProcessManager, cleanup scanner, ProviderInfo |
| 82 | Admin Projects: orchestrated sim+backend+frontend bundles, port allocator, 4 UI pages |
| 83 | Type-Safe API: goverter mappers, api.Backend interface, OpenAPI spec subset |
| 84 | Self-dispatch: `self api.Backend` on BaseServer, typed method overrides on all 6 cloud backends |
| 85 | Complete api.Backend: 21 new typed methods, httpProxy eliminated |
| 86 | In-process backend wiring + dead code cleanup (~1400 lines deleted) |
| 90 | Remove memory backend, spec-driven state machine tests, cloud operation mappings |

## Bug Fix Sprints (BUG-001 → BUG-583)

583 bugs fixed across 45 sprints. Per-sprint details in `_tasks/done/BUG-SPRINT-*.md`.

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

- **84 phases** (1-67, 69-77, 79-86), 753 tasks completed
- **45 bug sprints**, 583 bugs fixed (BUG-001→583), 0 open
- **16 Go modules** across backends, simulators, agent, API, frontend, bleephub, CLI, admin, tests
- **Core tests**: 302 PASS | **Frontend**: 7 | **UI (Vitest)**: 92 | **Admin**: 88 | **bleephub**: 304 | **ProcessRunner**: 15
- **Cloud SDK**: AWS 42, GCP 43, Azure 38 | **Cloud CLI**: AWS 26, GCP 21, Azure 19
- **E2E**: 371 GitHub+GitLab workflows | **Sim-backend**: 75 | **Terraform**: 75 | **Upstream**: 252
- **3 cloud simulators** validated against SDKs, CLIs, and Terraform
- **8 backends** sharing a common driver architecture

## Sprint 45 Summary (BUG-575 → BUG-583)

Fixed 8 bugs across Docker backend and frontend (BUG-580 confirmed as false positive). Docker backend `handleImageInspect` now maps `GraphDriver` from Docker SDK (BUG-575). `handleSystemDf` ContainerSummary EndpointSettings now includes IPv6Gateway, GlobalIPv6Address, GlobalIPv6PrefixLen, DriverOpts, and IPAMConfig (BUG-576), plus HostConfig with NetworkMode (BUG-577). System DF and all 3 volume handlers (create/list/inspect) now map UsageData from Docker SDK (BUG-578/579). `handleImagePush` now checks `X-Registry-Auth` header first, falls back to query param `auth` (BUG-581). BuildCache `LastUsedAt` returns empty string instead of `"0001-01-01T00:00:00Z"` for unused entries (BUG-582). Frontend `handleContainerCommit` now forwards all `changes` query params instead of only the first (BUG-583). Updated FEATURE_MATRIX.md with `docker import` row.

## Phase 83 — Type-Safe API Schema Infrastructure (in progress)

### Phase A: Align api.* Field Names with Docker SDK — DONE
- Renamed 7 fields in `api/types.go` to match Docker SDK naming:
  - `CpuShares` → `CPUShares`, `CpuQuota` → `CPUQuota`, `CpuPeriod` → `CPUPeriod`
  - `NanoCpus` → `NanoCPUs`, `Dns` → `DNS`, `DnsSearch` → `DNSSearch`, `DnsOptions` → `DNSOptions`
- JSON tags preserved (wire compatibility maintained)
- Updated all usage sites: `backends/docker/containers.go` (both create and inspect directions), `backends/core/handle_extended.go`
- Also renamed in `ContainerUpdateRequest` struct
- All 3 affected modules compile (`api`, `backends/core`, `backends/docker`)
- Core tests: 302 PASS

### Phase B1: Add goverter dependency — DONE
- Added `github.com/jmattheis/goverter v1.9.4` to `backends/docker/go.mod`
- Created `backends/docker/generate.go` with `//go:generate` directive

### Phase B2: Define goverter converter interface — DONE
- Created `backends/docker/converter.go` with goverter interface (17 methods)
- 30+ extend functions for type alias conversions (NetworkMode→string, Duration→int64, nat.PortSet→map, etc.)
- Generated `converter_gen.go` (275 lines) with field-by-field type-safe mapping
- `converter_init.go` with `!goverter` build tag bootstraps the converter instance
- `converter_manual.go` has composite converters for complex types (ContainerJSON, HostConfig, NetworkSettings, etc.)

### Phase B3: Replace manual mapping with generated converter — DONE
- Replaced all 51+ manual mapping sites with converter calls
- **604 lines deleted, 39 inserted** across 5 handler files
- Deleted: `mapContainerFromDocker`, `mapExposedPorts`, `mapPortBindings`, `mapMountsFromSummary`, `mapMountsFromDf`, `mapNetworkIPAMAndContainers`
- All modules compile, core 302 tests pass, go vet clean

### Phase C1: Implement api.Backend on Docker backend — DONE
- Created `backends/docker/backend_impl.go` (~580 lines)
- All 44 interface methods delegate to `s.docker.*` + goverter converters
- Helper types: `hijackedRWC` (wraps Docker HijackedResponse as io.ReadWriteCloser), `nopRWC` (empty io.ReadWriteCloser)
- Renamed `Info()` → `getInfo()` in `client.go` to avoid interface conflict
- Added `Name` field to `api.ContainerCreateRequest`

### Phase C2: Implement api.Backend on core BaseServer — DONE
- Created `backends/core/backend_impl.go` (~2080 lines)
- All 44 methods extracted from HTTP handlers into typed methods
- Helper types: `pipeRWC` (io.ReadWriteCloser from pipes), `pipeConn` (net.Conn adapter for driver Attach/Exec)
- Added `matchImageFilters()` for typed image filtering (reference, dangling, label, before, since)
- 302 core tests pass

### Phase C3: Change frontend to use api.Backend — DONE
- `Server.backend` changed from `*BackendClient` to `api.Backend`
- Added `Server.httpProxy *BackendClient` for operations not in interface (pods, build, archive, resize, push, save, search, commit, export, changes, update)
- Rewrote 8 handler files: ~30 handlers use typed `s.backend.Method()` calls, ~20 use `s.httpProxy`
- Created `BackendHTTPAdapter` implementing `api.Backend` via HTTP for backward compat (~720 lines)
- Added `bufferedConn` type to prevent data loss from HTTP upgrade connections (exec/attach)
- Added `parseFilters()` helper for Docker API JSON filter string parsing
- 7 frontend tests pass

### Phase C4: Update startup composition — DONE
- `NewServer(logger, backend api.Backend, backendAddr string)` accepts any in-process backend
- `cmd/main.go` uses `BackendHTTPAdapter` wrapping `BackendClient` for HTTP-based backends
- In-process wiring ready for Phase 68 (Multi-Tenant Backend Pools)

### Phase D1: Docker v1.44 spec subset — DONE
- Created `api/docker-v1.44-subset.yaml` — 73 type definitions
- Maps every `api.*` Go type to its Docker spec equivalent
- Lists all field names, types, required status, Docker spec name
- Supports `embeds` for embedded types and `extensions` for Sockerless-only fields

### Phase D2: Field coverage validation test — DONE
- Created `api/coverage_test.go` with `gopkg.in/yaml.v3`
- Parses YAML spec, compares against Go struct fields via reflection
- Bidirectional: fails if spec has field Go doesn't, or Go has field spec doesn't
- 73 subtests (one per type), all PASS
- Added `gopkg.in/yaml.v3 v3.0.1` to `api/go.mod`

## Phase 84 — Self-Dispatch for Cloud Backends (6 tasks)

Eliminated the dual code path problem where cloud backends had HTTP handlers with cloud-specific logic, but their inherited `api.Backend` methods bypassed that logic. Added a `self api.Backend` field to BaseServer for virtual dispatch — HTTP handlers call `s.self.Method()`, and cloud backends override the typed methods via Go embedding.

### Phase A: Self-dispatch on BaseServer — DONE
- Added `self api.Backend` field to BaseServer, `SetSelf(b)` method, `s.self = s` in constructor
- Removed `RouteOverrides` struct and all override logic from `registerRoutes()`
- Rewrote 16 HTTP handlers as thin wrappers delegating to `s.self.*`: ContainerCreate/Start/Stop/Kill/Remove/Restart/Logs/Attach, ContainerPrune/Pause/Unpause, ExecStart, ImagePull/Load, VolumeRemove/VolumePrune
- Added `CloudLogParamsFromOpts()` to core for cloud backends using typed log options
- Fixed 16 test files that create BaseServer directly (added `s.self = s`)

### Phase B: AWS backends (ECS + Lambda) — DONE
- Created `backends/ecs/backend_impl.go` with 14 typed method overrides
- Created `backends/lambda/backend_impl.go` with 12 typed method overrides
- Deleted old HTTP handler files (extended.go, images.go) and handler functions from containers.go/logs.go
- Refactored `startMultiContainerTask` to return error instead of writing to ResponseWriter
- Deferred ECS task definition registration from Create to Start (simplifies pod handling)

### Phase C: GCP backends (CloudRun + GCF) — DONE
- Created `backends/cloudrun/backend_impl.go` with 14 typed method overrides
- Created `backends/cloudrun-functions/backend_impl.go` with 12 typed method overrides
- Same cleanup pattern: deleted handler files, kept helpers

### Phase D: Azure backends (ACA + AZF) — DONE
- Created `backends/aca/backend_impl.go` with 14 typed method overrides
- Created `backends/azure-functions/backend_impl.go` with 12 typed method overrides
- Same cleanup pattern

### Phase E: Streaming methods audit — DONE
- Verified all streaming methods (ContainerLogs, ContainerAttach, ExecStart, ImagePull, ImageLoad) work correctly through self-dispatch
- Cloud backends use `io.Pipe()` for streaming — goroutine writes to PipeWriter, typed method returns PipeReader
- Core handler adds Docker mux framing on top of raw stream data

### Phase F: Verification — DONE
- All 11 modules build clean (core, memory, docker, ecs, lambda, cloudrun, gcf, aca, azf, frontend, api)
- Core: 302 PASS, Frontend: 7 PASS, API: 73 subtests PASS

---

## Phase 85 — Complete api.Backend Coverage (8 tasks)

**Goal:** Add all remaining typed methods to `api.Backend` (21 new methods), implement them on BaseServer and Docker backend, convert all frontend handlers to typed calls, and eliminate the `httpProxy` HTTP fallback entirely.

### Phase A-C: Types + Interface (3 tasks) — DONE
- Added 7 pod types to `api/types.go`: PodCreateRequest, PodCreateResponse, PodInspectResponse, PodContainerInfo, PodListEntry, PodActionResponse, PodListOptions
- Added ContainerPathStat, ContainerArchiveResponse, ImageBuildOptions, ImageSearchResult, ContainerCommitRequest
- Added 21 new methods to `api.Backend` interface: 8 pod + 8 container/exec + 5 image

### Phase D: BaseServer pod methods — DONE
- Created `backends/core/backend_impl_pods.go` with 8 pod method implementations
- Rewrote `handle_pods.go` as thin HTTP wrappers calling `s.self.PodMethod()`
- Moved pod type definitions from core to api package

### Phase E: BaseServer container/exec/image methods — DONE
- Created `backends/core/backend_impl_ext.go` with 13 method implementations
- Converted 7 handler files to thin wrappers: handle_extended.go, handle_containers_archive.go, handle_containers_export.go, handle_images.go, build.go, handle_commit.go
- Uses io.Pipe() pattern for streaming methods (ImageBuild, ImagePush, ImageSave, ContainerExport, ContainerGetArchive)

### Phase F: Docker backend + BackendHTTPAdapter — DONE
- Added 21 method implementations to `backends/docker/backend_impl.go` (Docker SDK delegation)
- Added 21 method implementations to `frontends/docker/backend_adapter.go` (HTTP proxy adapter)
- Pod methods return not-supported errors on Docker backend

### Phase G: Frontend typed handlers — DONE
- Converted all 22 httpProxy handlers to typed `s.backend.Method()` calls
- Files updated: pods.go, containers_stream.go, containers.go, exec.go, images.go
- Removed `httpProxy *BackendClient` field from frontend `Server` struct
- Zero HTTP proxy fallback paths remaining in frontend

### Phase H: Verification — DONE
- Core: 302 PASS, Frontend: 7 PASS, API: PASS
- All 8 backends compile clean
- Added buildargs validation to build handler (test fix)

## Phase 86 — In-Process Backend Wiring + Dead Code Cleanup (4 tasks)

**Goal:** Eliminate the HTTP round-trip between frontend and backend by wiring the in-process `BaseServer` directly, and delete all dead adapter/client/helper code.

### Phase A: Delete dead code — DONE
- Removed `proxyPassthrough()` and `proxyErrorResponse()` from `frontends/docker/helpers.go` (-20 lines)
- Removed `parseEnv()` from `backends/core/build.go` (-16 lines)

### Phase B: Remove backendAddr + wire in-process — DONE
- Removed unused `backendAddr string` parameter from `NewServer()` signature
- Updated 3 test call sites in `server_test.go`
- Replaced `BackendHTTPAdapter(BackendClient)` in `cmd/main.go` with direct `core.NewBaseServer()` wiring
- Frontend now calls backend methods in-process (no HTTP round-trip)

### Phase C: Delete BackendHTTPAdapter + BackendClient — DONE
- Deleted `frontends/docker/backend_adapter.go` (~1100 lines)
- Deleted `frontends/docker/backend_client.go` (~260 lines)

### Phase D: Verification — DONE
- Core: 302 PASS, Frontend: 7 PASS, API: PASS
- All 9 backends compile clean
- **Net: ~1400 lines deleted, 0 new files**

## Phase 90 — Remove Memory Backend + Spec Verification (3 tasks)

**Goal:** Remove the memory backend (thin wrapper around BaseServer with WASM sandbox), add spec-driven state machine tests, and document cloud operation mappings.

### Task 1: Remove backends/memory/ — DONE
- Deleted `backends/memory/` directory (~180 lines: server.go, adapter.go, cmd/main.go, Dockerfile, Makefile, ui_embed/noembed.go)
- Deleted `ui/packages/backend-memory/` (SPA package)
- Deleted 3 memory-specific Dockerfiles (e2e, upstream-act, upstream-gcl)
- Deleted `smoke-tests/act/Dockerfile` (memory-specific, referenced deleted frontends/docker)
- Removed from `go.work`, `.goreleaser.yml`, `.github/workflows/ci.yml`, `.github/workflows/docker.yml`
- Cleaned Makefile: removed ~20 memory targets, updated defaults from memory→ecs, removed from MODULES_UI
- Updated `smoke-tests/act/run.sh`: removed memory case, changed default to ecs
- **Backend count: 8 → 7**

### Task 2: Spec-driven state machine tests (P90-C) — DONE
- Created `backends/core/spec_verify_test.go` (~280 lines)
- `TestSpecStateTransitions`: parses `api/openapi.yaml` for `x-sockerless-state-transition`, tests 15 state transitions (8 operations × from-states)
- `TestSpecStateTransition_ContainerCreate`: dedicated test for create (no from-state)
- `TestSpecForceRemove`: tests force-remove from all 4 states
- `TestSpecOperationCoverage`: verifies test↔spec bidirectional coverage
- `TestSpecEventTypes`: validates all event types match Docker conventions
- **5 new spec verification tests, all PASS**

### Task 3: Cloud operation mappings (P90-D) — DONE
- Added `x-sockerless-cloud-operations` to 4 lifecycle operations in `api/openapi.yaml`
- Documents how ContainerCreate/Start/Stop/Remove map to each cloud provider's API
- Added extension to header documentation

## Unified Image Management (PR #100)

Consolidated all image management into per-cloud shared modules:

- **`core.ImageManager`** + **`core.AuthProvider`** interface in `backends/core/image_manager.go`
- **3 shared cloud modules**: `backends/aws-common/` (ECR), `backends/gcp-common/` (Artifact Registry), `backends/azure-common/` (ACR)
- All 6 cloud backends delegate 12 image methods to `s.images.*` one-liners
- Deleted ~2000 lines of duplicated code: 4 `registry.go` files, `aca/backend_impl_images.go`, 6 per-backend `image_auth.go` files
- `core.ParseImageRef()` exported for use by AuthProvider implementations
- `core.SetOCIAuth()` exported for cloud auth header injection
