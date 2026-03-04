# Known Bugs

## Fixed (BUG-001 → BUG-226)

226 bugs fixed across 22 sprints. See `WHAT_WE_DID.md` for sprint summaries and `_tasks/done/BUG-SPRINT-*.md` for per-sprint details.

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

## Open Bugs

### High Severity

| Bug | File | Issue |
|-----|------|-------|
| OB-073 | `cloudrun/containers.go:628` | CloudRun waitForExecutionRunning returns GCP resource path (not IP) as agent address — forward agent always fails |
| OB-074 | `aca/containers.go:624` | ACA waitForExecutionRunning returns execution name (not IP) as agent address — forward agent always fails |

### Medium Severity

| Bug | File | Issue |
|-----|------|-------|
| OB-013 | `core/handle_images.go:226-231` | Image tag doesn't update old alias store entries |
| OB-014 | `core/handle_containers_archive.go:251-264` | tmpfs temp dirs never cleaned up |
| OB-015 | `docker/containers.go:347-355` | Docker attach drops logs/detachKeys params |
| OB-016 | `docker/containers.go:303-313` | Docker logs drops details param |
| OB-018 | `docker/extended.go:324-390` | Docker system df drops BuildCache |
| OB-045 | `docker/containers.go:356-393`, `docker/exec.go:58-95` | Docker attach/exec ignores stdin — unidirectional copy only (interactive sessions broken) |
| OB-047 | `docker/server.go:46-106` | Docker backend missing routes: container update, changes, export, commit, resize, exec resize |
| OB-050 | `docker/extended.go:330-412` | Docker system df missing NetworkSettings in container summaries |
| OB-055 | `lambda/containers.go:390-401`, `gcf/containers.go`, `azf/containers.go` | FaaS kill + background invocation goroutine race on WaitCh and container state |
| OB-056 | `lambda/images.go`, `gcf/images.go`, `azf/images.go` | FaaS image pull uses synthetic config (no registry fetch) — containers lose default CMD/ENV |
| OB-077 | `core/build.go:519-523` | BuildContexts entry and staging dir leaked if image is built but never used as container |
| OB-081 | `core/health.go:84-148` | Health check execs not tracked in Store.Execs or container.ExecIDs — invisible to exec API |
| OB-086 | `lambda/containers.go:191-216`, `cloudrun-functions/containers.go:176-219` | FaaS create stores container after Registry.Register — orphan cloud resource on interrupt |
| OB-092 | `frontends/docker/images.go:12-42` | Frontend handleImageCreate ignores fromSrc param — docker import silently broken |
| OB-093 | `frontends/docker/containers_stream.go:108-110` | Frontend handleContainerResize always returns 200 without forwarding h/w — TTY resize dropped |
| OB-094 | `frontends/docker/exec.go:122-124` | Frontend handleExecResize always returns 200 without forwarding — exec TTY resize dropped |
| OB-095 | `frontends/docker/backend_client.go:184-248` | Frontend dialUpgrade ignores request context — backend connections not cancelled on disconnect |

### Low Severity

| Bug | File | Issue |
|-----|------|-------|
| OB-026 | `docker/networks.go:91` | Docker network inspect drops verbose/scope params |
| OB-027 | `docker/containers.go:248-256` | Docker container start drops checkpoint options |
| OB-028 | `docker/images.go:40-96` | Docker image inspect missing Metadata.LastTagTime |
| OB-060 | `core/handle_images.go`, `core/handle_extended.go`, `core/handle_volumes.go` | Missing Docker-compatible events for image tag/remove, volume create/destroy |
| OB-063 | `core/handle_extended.go:325-347` | Container update ignores resource fields — only handles RestartPolicy |
| OB-064 | `core/handle_images.go:181-209` | Image load always stores as "loaded:latest" (overwrites, no manifest extraction) |
| OB-065 | All 6 cloud backends | Prune handlers skip AgentRegistry.Remove (stale entries for stopped containers) |
| OB-066 | `lambda/logs.go`, `gcf/logs.go`, `azf/logs.go` | FaaS logs endpoint ignores buffered LogBuffers data (only queries cloud logging) |
| OB-068 | `api/types.go` | API types missing fields: NetworkSettings top-level (Gateway, IPAddress, Bridge, etc.), HostConfig (DNS, Memory, CpuShares, PidMode, etc.), HealthcheckConfig.StartInterval, EndpointSettings IPv6 fields, Image.GraphDriver, ExecInstance.DetachKeys |
| OB-098 | `core/handle_containers_archive.go:109-143` | Symlinks and hardlinks silently dropped during tar extraction |
| OB-100 | `core/handle_containers_query.go:82-105` | Malformed Created timestamp causes incorrect before/since filter results |
| OB-103 | `ecs/containers.go:522-525` | ECS kill exitCh — background polling goroutine continues after force-stop (self-terminates eventually) |
| OB-104 | `ecs/containers.go:551-576`, `cloudrun/containers.go:579`, `aca/containers.go:565` | Cloud force remove + background poller race — StopContainer called twice (may double-close channel) |
| OB-105 | `aca/containers.go:748-765`, `cloudrun/containers.go:738` | ACA/CloudRun fire-and-forget LRO pollers — stop/delete jobs not actually waited on |
| OB-106 | `lambda/recovery.go:14-43` | Lambda ScanOrphanedResources no pagination — misses functions beyond first 50 |
| OB-107 | `azure-functions/recovery.go:54-66` | AZF CleanupResource always no-op — AZF state not persisted, lookup always fails at startup |
| OB-108 | `lambda/containers.go:248-296` | Lambda start goroutine calls StopContainer on already-removed container (benign but noisy) |
| OB-113 | `api/types.go:589-595` | DiskUsageResponse missing BuildCache field and BuildCache type |
| OB-116 | `frontends/docker/images.go:177-195` | Frontend handleImageBuild missing query params: labels, target, platform, pull, etc. |
| OB-117 | `frontends/docker/images.go:210-224` | Frontend handleContainerCommit missing pause and changes params |
| OB-118 | `frontends/docker/networks.go:52-61` | Frontend handleNetworkInspect drops verbose/scope params — never forwarded to backend |

## False Positives

Bugs investigated and confirmed as non-issues. Tracked here to avoid re-collecting.

| ID | File | Reported Issue | Why False Positive |
|----|------|----------------|-------------------|
| FP-001 | `api/types.go:554-558` | Event Type/Action/Actor use uppercase JSON tags | Docker API uses PascalCase for these fields — current tags are correct |
| FP-002 | `api/types.go` | EndpointSettings missing IPAMConfig type (OB-057) | EndpointIPAMConfig already exists since Sprint 20 — the type was added in BUG-175 |
