# Known Bugs

## Fixed (BUG-001 → BUG-436)

410 bugs fixed across 34 sprints. See `WHAT_WE_DID.md` for sprint summaries and `_tasks/done/BUG-SPRINT-*.md` for per-sprint details.

| Sprint | Bugs | Focus |
|--------|------|-------|
| 1-6 | BUG-001→046 | Admin UI: races, concurrency, error states, XSS, HTTP status codes |
| SimCmd | BUG-047→051 | FaaS simulator command protocol cleanup |
| 7 | BUG-052→062 | Core: tar corruption, error swallowing, cloud resource leaks |
| 9 | BUG-063→068 | API types, cloud state revert, cloud resource cleanup |
| 10 | BUG-069→074 | Image store, FaaS kill/prune lifecycle, Docker Mounts |
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
| 28 | BUG-337→344 | ECS ClusterARN in remove/restart, ECS restart task def leak, CloudRun/ACA restart job leak, pod kill signal, image tag dedup, frontend stats one-shot |
| 29 | BUG-345→358 | Cloud restart "stop" event + RestartCount, cloud remove/prune "destroy" event + pod cleanup, core pod stop/kill events, pod remove non-force cleanup |
| 30 | BUG-359→377 | Cloud StopHealthCheck gaps, "create" event, signalToExitCode, force-remove events, Network.Disconnect, core prune events, pod lifecycle, FormatStatus uptime, Event.Scope, restart event |
| 31 | BUG-378→394 | Pod start process/health, pod remove non-force cleanup, container wait condition, container list filters (exited/publish/volume/is-task/size), cloud pod param, cloud restart event guard, frontend pod query params, logs details |
| 32 | BUG-395→408 | Cloud kill event order, TmpfsDirs cleanup, image push/save/search stubs, pod list filters, frontend query param forwarding |
| 33 | BUG-409→422 | Core resize endpoints, stats precpu/memlimit, default networks, event emissions, SpaceReclaimed, deterministic ImageID, paused status, health exec cleanup, paused count, logs stdout/stderr |
| 34 | BUG-423→436 | Cloud logs parity (since/until/tail/stdout/stderr/details/follow), ImageSummary.Containers count, health check StartInterval |
| 35 | BUG-437→449 | Container inspect paths, NetworkSettings fields, KernelVersion, exec CanRemove, frontend df type param, image GraphDriver/RootFS, volume UsageData |

## Sprint 35 Detail (BUG-437 → BUG-449)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-437 | Core | `buildContainerFromConfig` never sets `LogPath` — always empty in inspect |
| BUG-438 | Core | `buildContainerFromConfig` never sets `ResolvConfPath` — always empty |
| BUG-439 | Core | `buildContainerFromConfig` never sets `HostnamePath` — always empty |
| BUG-440 | Core | `buildContainerFromConfig` never sets `HostsPath` — always empty |
| BUG-441 | Core | `NetworkSettings.SandboxID` never populated — always empty |
| BUG-442 | Core | `NetworkSettings.SandboxKey` never populated — always empty |
| BUG-443 | Core | `NetworkSettings.Bridge` never populated for bridge network containers |
| BUG-444 | Core | `handleInfo` never sets `KernelVersion` — frontend forwards empty string |
| BUG-445 | Core | `ExecInstance.CanRemove` never set to true after exec completes |
| BUG-446 | Frontend | `handleSystemDf` doesn't forward `type` query parameter to backend |
| BUG-447 | Core | `Image.RootFS.Layers` empty in `handleImageLoad` — pull sets synthetic layer but load doesn't |
| BUG-448 | Core | `Image.GraphDriver` never populated — missing `Name` and `Data` in both pull and load |
| BUG-449 | Core | `handleSystemDf` volume `UsageData` never set — missing RefCount and Size |

## Sprint 34 Detail (BUG-423 → BUG-436)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-423 | All 6 cloud | `handleContainerLogs` ignores `since` query param — no timestamp-based start filtering |
| BUG-424 | All 6 cloud | `handleContainerLogs` ignores `until` query param — no timestamp-based end filtering |
| BUG-425 | CloudRun, GCF, ACA, AZF | `handleContainerLogs` ignores `tail` query param — returns all log entries |
| BUG-426 | All 6 cloud | `handleContainerLogs` ignores `stdout`/`stderr` query params — always writes stdout |
| BUG-427 | All 6 cloud | `handleContainerLogs` ignores `details` query param — doesn't prepend container labels |
| BUG-428 | Lambda | `handleContainerLogs` ignores `follow` query param — never polls for new output |
| BUG-429 | GCF | `handleContainerLogs` ignores `follow` query param — fetches once and exits |
| BUG-430 | AZF | `handleContainerLogs` ignores `follow` query param — fetches once and exits |
| BUG-431 | Core | `ImageSummary.Containers` always 0 in `handleImageList` |
| BUG-432 | Core | Health check loop ignores `StartInterval` during start period |
| BUG-433 | ECS, CloudRun, ACA | Follow-mode logs should not apply `since`/`until` to follow queries |
| BUG-434 | ACA | Follow-mode polling interval 2s vs 1s for ECS/CloudRun — inconsistent |
| BUG-435 | Lambda, GCF, AZF | `LogBuffers` output not subject to `tail`/`since`/`until` filtering |
| BUG-436 | Core | `handleSystemDf` `ImageSummary.Containers` always 0 |

## Sprint 31 Detail (BUG-378 → BUG-394)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-378 | Core | `handlePodStart` doesn't call `ProcessLifecycle.Start()` — containers marked running but no process spawned |
| BUG-379 | Core | `handlePodStart` doesn't call `StartHealthCheck()` for containers with health checks |
| BUG-380 | Core | `handlePodRemove` non-force path missing `StopHealthCheck(cid)` |
| BUG-381 | Core | `handlePodRemove` non-force path missing `ProcessLifecycle.Cleanup(cid)` |
| BUG-382 | Core | `handlePodRemove` non-force path missing "destroy" events per container |
| BUG-383 | Core | `handlePodRemove` non-force path missing `Network.Disconnect` loop |
| BUG-384 | Core | `handleContainerWait` ignores `condition` query parameter — always behaves as `not-running` |
| BUG-385 | Core | `MatchContainerFilters` missing `exited` filter (exit code filtering) |
| BUG-386 | All 6 cloud | `handleContainerCreate` missing `?pod=` query parameter validation + `Pods.AddContainer()` |
| BUG-387 | All 6 cloud | `handleContainerRestart` emits "restart" event unconditionally — should only emit on success |
| BUG-388 | Frontend | `handlePodKill` doesn't forward `signal` query parameter to backend |
| BUG-389 | Frontend | `handlePodStop` doesn't forward `t` (timeout) query parameter to backend |
| BUG-390 | Core | `handleContainerLogs` doesn't handle `details` query parameter |
| BUG-391 | Core | `MatchContainerFilters` missing `publish` filter (port filtering) |
| BUG-392 | Core | `MatchContainerFilters` missing `volume` filter (mount filtering) |
| BUG-393 | Core | `MatchContainerFilters` missing `is-task` filter |
| BUG-394 | Core | `handleContainerList` missing `size` query parameter support |

## Sprint 33 Detail (BUG-409 → BUG-422)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-409 | Core | Missing `POST /containers/{id}/resize` and `POST /exec/{id}/resize` routes — frontend forwards these but core returns 404 |
| BUG-410 | Core | `handleContainerStats` missing `precpu_stats` field — Docker clients need it to calculate CPU % |
| BUG-411 | Core | `handleContainerStats` memory limit hardcoded to 1 GiB — should use container's `HostConfig.Memory` when set |
| BUG-412 | Core | `InitDefaultNetwork` only creates `bridge` — missing `host` and `none` default networks |
| BUG-413 | Core | `handleContainerCommit` missing `container` `commit` event emission |
| BUG-414 | Core | `handleContainerUpdate` missing `container` `update` event emission |
| BUG-415 | Core | `handleImageLoad` missing `image` `load` event emission |
| BUG-416 | Core | `handleContainerPrune` SpaceReclaimed always 0 — should calculate from container rootDir |
| BUG-417 | Core | `handleVolumePrune` SpaceReclaimed always 0 — should calculate from volume dir |
| BUG-418 | Core | `handleContainerList` generates random ImageID each call for unknown images — not deterministic |
| BUG-419 | Core | `FormatStatus` for paused state returns bare "paused" instead of "Up X seconds (Paused)" |
| BUG-420 | Core | Health check exec instances leak in Store.Execs — never cleaned up |
| BUG-421 | Core | `handleInfo` missing `ContainersPaused` count — paused containers miscounted as running |
| BUG-422 | Core | `handleContainerLogs` ignores `stdout`/`stderr` query params — all output always returned as stdout |

## Sprint 32 Detail (BUG-395 → BUG-408)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-395 | All 6 cloud | `handleContainerKill` closes WaitChs BEFORE emitting "kill"+"die" events — event watchers miss events |
| BUG-396 | All 6 cloud | `handleContainerRemove` missing `TmpfsDirs.LoadAndDelete` + `os.RemoveAll` cleanup — tmpfs dirs leak |
| BUG-397 | All 6 cloud | `handleContainerPrune` missing `TmpfsDirs.LoadAndDelete` + `os.RemoveAll` cleanup |
| BUG-398 | Frontend | `handleContainerList` doesn't forward `size` query parameter to backend |
| BUG-399 | Core + Frontend | `handleImagePush` is a stub — returns hardcoded fake push output instead of proxying to backend |
| BUG-400 | Core | `handlePodList` doesn't support `filters` query parameter (name, id, label, status) |
| BUG-401 | Frontend | `handlePodList` doesn't forward `filters` query parameter to backend |
| BUG-402 | Frontend | `handleContainerStop` doesn't forward `signal` query parameter (Docker API v1.41+) |
| BUG-403 | Frontend | `handleImageCreate` (pull) doesn't forward `platform` query parameter |
| BUG-404 | Frontend | `handleImageList` doesn't forward `shared-size` and `digests` query parameters |
| BUG-405 | Frontend | `handleContainerPutArchive` doesn't forward `noOverwriteDirNonDir` query parameter |
| BUG-406 | Frontend | `handleNetworkRemove` doesn't forward `force` query parameter |
| BUG-407 | Core + Frontend | No `GET /images/get` endpoint for `docker save` (image export to tar) |
| BUG-408 | Core + Frontend | No `GET /images/search` endpoint — was returning NotImplemented |

## Sprint 30 Detail (BUG-359 → BUG-377)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-359 | All 6 cloud | `handleContainerStop` missing `StopHealthCheck(id)` |
| BUG-360 | All 6 cloud | `handleContainerKill` missing `StopHealthCheck(id)` |
| BUG-361 | All 6 cloud | `handleContainerRemove` missing `StopHealthCheck(id)` |
| BUG-362 | All 6 cloud | `handleContainerCreate` missing "create" event |
| BUG-363 | All 6 cloud | `signalToExitCode` only handles SIGKILL — core maps 8 signals (24 aliases) |
| BUG-364 | All 6 cloud | `handleContainerRemove` force path missing "kill"+"die" events |
| BUG-365 | All 6 cloud | `handleContainerRemove` missing `Network.Disconnect` loop |
| BUG-366 | All 6 cloud | `handleContainerPrune` missing `Network.Disconnect` loop + `StopHealthCheck` |
| BUG-367 | Core | `handleImagePrune` missing `BuildContexts` cleanup |
| BUG-368 | Core | `handleImagePrune` missing untag/delete events |
| BUG-369 | Core | `handleVolumePrune` missing destroy events |
| BUG-370 | Core | `handleNetworkPrune` missing destroy events |
| BUG-371 | Core | `handlePodKill` missing `ProcessLifecycle.Cleanup(cid)` |
| BUG-372 | Core | `handlePodRemove` force path missing "destroy" events per container |
| BUG-373 | Core | `handlePodStart` doesn't reset `FinishedAt`/`ExitCode` |
| BUG-374 | Core | `FormatStatus` hardcoded "Up Less than a second" — should compute actual uptime |
| BUG-375 | Core | `Event.Scope` field never set to "local" — Docker always sets it |
| BUG-376 | Core + All 6 cloud | `handleContainerRestart` missing "restart" event |

## Sprint 29 Detail (BUG-345 → BUG-358)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-345 | ECS | `handleContainerRestart` emits "die" but not "stop" event |
| BUG-346 | Lambda | `handleContainerRestart` emits "die" but not "stop" event |
| BUG-347 | CloudRun | `handleContainerRestart` emits "die" but not "stop" event |
| BUG-348 | GCF | `handleContainerRestart` emits "die" but not "stop" event |
| BUG-349 | ACA | `handleContainerRestart` emits "die" but not "stop" event |
| BUG-350 | AZF | `handleContainerRestart` emits "die" but not "stop" event |
| BUG-351 | All 6 cloud | `handleContainerRestart` doesn't increment `RestartCount` |
| BUG-352 | All 6 cloud | `handleContainerRemove` missing "destroy" event |
| BUG-353 | All 6 cloud | `handleContainerPrune` missing "destroy" event |
| BUG-354 | Core | `handlePodStop` missing "die" and "stop" events |
| BUG-355 | Core | `handlePodKill` missing "kill" and "die" events |
| BUG-356 | All 6 cloud | `handleContainerRemove` missing pod cleanup (`Pods.RemoveContainer`) |
| BUG-357 | All 6 cloud | `handleContainerPrune` missing pod cleanup (`Pods.RemoveContainer`) |
| BUG-358 | Core | `handlePodRemove` with `force=false` doesn't clean up exited containers |

## Sprint 27 Detail (BUG-320 → BUG-336)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-320 | Core | `handleContainerRemove` WaitChs.Delete doesn't close channel |
| BUG-321 | Core | `store.RevertToCreated` WaitChs.Delete doesn't close channel |
| BUG-322 | ECS | `handleContainerRemove` WaitChs.Delete doesn't close channel |
| BUG-323 | ECS | `handleContainerPrune` WaitChs.Delete doesn't close channel |
| BUG-324 | CloudRun | `handleContainerRemove` WaitChs.Delete doesn't close channel |
| BUG-325 | CloudRun | `handleContainerPrune` WaitChs.Delete doesn't close channel |
| BUG-326 | ACA | `handleContainerRemove` WaitChs.Delete doesn't close channel |
| BUG-327 | ACA | `handleContainerPrune` WaitChs.Delete doesn't close channel |
| BUG-328 | Lambda | `handleContainerRemove` WaitChs.Delete doesn't close channel |
| BUG-329 | Lambda | `handleContainerPrune` WaitChs.Delete doesn't close channel |
| BUG-330 | GCF | `handleContainerRemove` WaitChs.Delete doesn't close channel |
| BUG-331 | GCF | `handleContainerPrune` WaitChs.Delete doesn't close channel |
| BUG-332 | AZF | `handleContainerRemove` WaitChs.Delete doesn't close channel |
| BUG-333 | AZF | `handleContainerPrune` WaitChs.Delete doesn't close channel |
| BUG-334 | ACA | `handleContainerRestart` MarkCleanedUp without empty guard + premature state delete |
| BUG-335 | Docker | `handleContainerCommit` builds invalid ref `"repo:"` when tag empty |
| BUG-336 | Frontend | `handleContainerLogs` missing `details` query parameter forwarding |

## Open Bugs

None — all known bugs have been fixed.

## False Positives

Bugs investigated and confirmed as non-issues. Tracked here to avoid re-collecting.

| ID | File | Reported Issue | Why False Positive |
|----|------|----------------|-------------------|
| FP-001 | `api/types.go:554-558` | Event Type/Action/Actor use uppercase JSON tags | Docker API uses PascalCase for these fields — current tags are correct |
| FP-002 | `api/types.go` | EndpointSettings missing IPAMConfig type (OB-057) | EndpointIPAMConfig already exists since Sprint 20 — the type was added in BUG-175 |
