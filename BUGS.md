# Known Bugs

## Fixed (BUG-001 → BUG-553)

553 bugs fixed across 43 sprints. See `WHAT_WE_DID.md` for sprint summaries and `_tasks/done/BUG-SPRINT-*.md` for per-sprint details.

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
| 36 | BUG-450→462 | System df field gaps, container list SizeRw, commit/build GraphDriver, Container.Image sha256 ID |
| 37 | BUG-463→475 | Image history/save, stats networks, container size fields, update PidsLimit/OomKillDisable, image push auth, LastTagTime, image search limit |
| 38 | BUG-476→488 | Stop/restart timeout params, container/volume prune SpaceReclaimed (6+3 cloud backends), wait "removed" condition, image save config JSON |
| 39 | BUG-489→501 | Container expose filter, image before/since filters, image load quiet, image push auth, resize h/w, top PID, image tag validation, volume dangling filter, exec TTY content-type |
| 40 | BUG-502→514 | Frontend attach TTY content-type, exec empty Cmd, container top ps_args, stop signal, frontend query params, Docker backend missing image/archive routes, image search sort |
| 41 | BUG-515→527 | ENV merge, Cmd/Entrypoint, stats name/id/precpu, volume 200, events since/until, cloud errors, image save 404, Dockerfile parser, NanoCpus |
| 42 | BUG-528→540 | ECS/CloudRun/ACA ENV merge + Cmd/Entrypoint, staticcheck QF1008, errcheck lint |
| 43 | BUG-541→553 | FaaS ENV merge + Cmd/Entrypoint, Docker image tag 200, Healthcheck StartInterval, ContainerConfig fields, Mount inspect, incrementing PIDs, deterministic image sizes |

## Sprint 43 Detail (BUG-541 → BUG-553)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-541 | Lambda/GCF/AZF | `handleContainerCreate` ENV merge is all-or-nothing — now uses `MergeEnvByKey` (same fix as BUG-515/528) |
| BUG-542 | Lambda/GCF/AZF | `handleContainerCreate` inherits image Cmd when Entrypoint overridden — now clears Cmd when only Entrypoint is overridden (same fix as BUG-516/529) |
| BUG-543 | Docker | `handleImageTag` returns 201 Created — Docker API returns 200 OK for image tag |
| BUG-544 | Docker | `handleContainerCreate` Healthcheck mapping missing `StartInterval` — now forwarded to Docker SDK |
| BUG-545 | Docker | `mapContainerFromDocker` Healthcheck inspect mapping missing `StartInterval` — now mapped in reverse direction |
| BUG-546 | Docker | `handleContainerCreate` ContainerConfig missing `ArgsEscaped`, `NetworkDisabled`, `OnBuild` — now forwarded |
| BUG-547 | Docker | `mapContainerFromDocker` inspect Config missing `ArgsEscaped`, `NetworkDisabled`, `OnBuild` — now mapped |
| BUG-548 | Docker | `mapContainerFromDocker` Mount inspect missing `VolumeOptions` and `TmpfsOptions` — now mapped (create already had them) |
| BUG-549 | Core | `handleContainerStart`/`handleContainerRestart`/restart_policy/pods hardcode `Pid=42` — now uses `Store.NextPID()` incrementing counter |
| BUG-550 | Core | `handleExecStart` hardcodes `Pid=43` — now uses `Store.NextPID()` |
| BUG-551 | Core | `handleImagePull` hardcodes `Size: 7654321` — now uses deterministic FNV hash of image reference (10-100MB range) |
| BUG-552 | Docker | `handleContainerCreate` missing `MacAddress` in ContainerConfig mapping — now forwarded |
| BUG-553 | Docker | `mapContainerFromDocker` inspect Config missing `MacAddress` — now mapped |

## Sprint 42 Detail (BUG-528 → BUG-540)

See `_tasks/done/BUG-SPRINT-42.md` for details.

## Sprint 41 Detail (BUG-515 → BUG-527)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-515 | Core | `handleContainerCreate` ENV merge is all-or-nothing — now merges by key (image defaults, container overrides) |
| BUG-516 | Core | `handleContainerCreate` inherits image Cmd when container overrides Entrypoint — now clears Cmd when Entrypoint is overridden |
| BUG-517 | Core | `buildStatsEntry` missing `name` and `id` top-level fields — now includes container name and ID |
| BUG-518 | Core | `buildStatsEntry` `precpu_stats` always zero — now tracks previous CPU reading per container |
| BUG-519 | Core | `handleVolumeCreate` returns 201 for existing volume — now returns 200 (Docker parity) |
| BUG-520 | Core | `handleSystemEvents` missing `since`/`until` query params — now supports event replay and auto-stop |
| BUG-521 | ECS | `errors.go` empty — now has `mapAWSError` for proper 404/409/400 mapping |
| BUG-522 | CloudRun | `errors.go` empty — now has `mapGCPError` for proper 404/409/400 mapping |
| BUG-523 | ACA | `errors.go` empty — now has `mapAzureError` for proper 404/409/400 mapping |
| BUG-524 | Core | `handleImageSave` silently skips missing images — now returns 404 if any image not found |
| BUG-525 | Core | `parseLabels` breaks on quoted values with spaces — now uses quote-aware tokenizer |
| BUG-526 | Core | `parseEnv` doesn't handle multi-value `ENV k1=v1 k2=v2` — now parses all pairs |
| BUG-527 | API | `HostConfig` missing `NanoCpus` field — now accepted, stored, and returned on inspect |

## Sprint 40 Detail (BUG-502 → BUG-514)

| ID | Component | Description |
|----|-----------|-------------|
| BUG-502 | Frontend | `handleContainerAttach` hardcodes multiplexed-stream Content-Type — now uses raw-stream for TTY containers |
| BUG-503 | Core | `handleExecCreate` allows empty `Cmd` — now returns 400 "No exec command specified" |
| BUG-504 | Core | `handleContainerTop` ignores `ps_args` query param — now reads it for API parity |
| BUG-505 | Core | `handleContainerStop` ignores `signal` query param — now applies signal-based exit code |
| BUG-506 | Frontend | `handleContainerCreate` doesn't forward `platform` query param to backend |
| BUG-507 | Frontend | `handleContainerRemove` doesn't forward `link` query param to backend |
| BUG-508 | Docker | Missing `handleImagePush` route — `POST /internal/v1/images/{name}/push` not registered |
| BUG-509 | Docker | Missing `handleImageSave` routes — `GET /internal/v1/images/get` and `GET /internal/v1/images/{name}/get` not registered |
| BUG-510 | Docker | Missing `handleImageSearch` route — `GET /internal/v1/images/search` not registered |
| BUG-511 | Docker | Missing `handleImageBuild` route — `POST /internal/v1/images/build` not registered |
| BUG-512 | Docker | Missing archive routes — `PUT/HEAD/GET /internal/v1/containers/{id}/archive` not registered |
| BUG-513 | Core | `handleImageSearch` returns results in arbitrary iteration order — now sorted by relevance |
| BUG-514 | Frontend | `handleContainerStart` doesn't forward `detachKeys` query param to backend |

## Open Bugs

None — all known bugs have been fixed.

## False Positives

Bugs investigated and confirmed as non-issues. Tracked here to avoid re-collecting.

| ID | File | Reported Issue | Why False Positive |
|----|------|----------------|-------------------|
| FP-001 | `api/types.go:554-558` | Event Type/Action/Actor use uppercase JSON tags | Docker API uses PascalCase for these fields — current tags are correct |
| FP-002 | `api/types.go` | EndpointSettings missing IPAMConfig type (OB-057) | EndpointIPAMConfig already exists since Sprint 20 — the type was added in BUG-175 |
| FP-003 | `api/types.go` | Container.State/Config/HostConfig should be pointers | Changing to pointers would break vast amounts of code — Go JSON unmarshal handles both fine |
| FP-004 | Core | Container kill emits events in wrong order | Docker actually emits `kill` then `die` — current order is correct |
| FP-005 | Core | handleContainerChanges always empty | Known limitation — no real filesystem change tracking |
| FP-006 | Core | Stats network stats always zero | Known limitation |
| FP-007 | Core | Missing Ulimits/DeviceRequests/Devices in HostConfig | Lower priority — save for future sprint |
| FP-008 | Core | FaaS stop doesn't cancel invocations | By design — FaaS invocations are fire-and-forget |
| FP-009 | Core | FaaS resource leak on pod validation failure | Validation order is correct — rejection before cloud resource creation |
