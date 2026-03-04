# Known Bugs

## Fixed (BUG-001 → BUG-176)

176 bugs fixed across 20 sprints. See `WHAT_WE_DID.md` for sprint summaries and `_tasks/done/BUG-SPRINT-*.md` for per-sprint details.

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

## Open Bugs

### High Severity

| Bug | File | Issue |
|-----|------|-------|
| OB-002 | `docker/containers.go:127-141` | Docker container create drops IPAMConfig (static IP silently fails) |
| OB-003 | `docker/extended.go:134-153` | Docker network connect drops IPAMConfig (static IP on connect silently fails) |
| OB-072 | `core/handle_extended.go:178-217` | Container rename non-atomic — 3 separate lock ops create window where container is unreachable by name |
| OB-073 | `cloudrun/containers.go:628` | CloudRun waitForExecutionRunning returns GCP resource path (not IP) as agent address — forward agent always fails |
| OB-074 | `aca/containers.go:624` | ACA waitForExecutionRunning returns execution name (not IP) as agent address — forward agent always fails |

### Medium Severity

| Bug | File | Issue |
|-----|------|-------|
| OB-004 | `core/handle_containers.go:72-91` | Container create leaks on pod validation failure (also leaks ContainerNames) |
| OB-005 | `core/handle_pods.go:180-219` | Pod remove without force doesn't check running containers |
| OB-006 | `core/handle_pods.go:135-178` | Pod start/stop/kill don't cascade to containers |
| OB-010 | `core/handle_containers.go`, `core/handle_extended.go` | Container remove/prune missing pod registry cleanup |
| OB-013 | `core/handle_images.go:226-231` | Image tag doesn't update old alias store entries |
| OB-014 | `core/handle_containers_archive.go:251-264` | tmpfs temp dirs never cleaned up |
| OB-015 | `docker/containers.go:347-355` | Docker attach drops logs/detachKeys params |
| OB-016 | `docker/containers.go:303-313` | Docker logs drops details param |
| OB-018 | `docker/extended.go:324-390` | Docker system df drops BuildCache |
| OB-020 | `ecs/extended.go`, `cloudrun/extended.go`, `aca/extended.go` | Cloud restart leaks old resource in ResourceRegistry (ECS task ARN, CloudRun/ACA job name) |
| OB-022 | All 6 cloud backends | Cloud remove force-path uses StopContainer (not ForceStop) |
| OB-031 | `docker/containers.go:109-122` | Docker mount mapping drops VolumeOptions and TmpfsOptions |
| OB-040 | `core/handle_extended.go:67-96` | Core container prune ignores `filters` query param (prunes everything) |
| OB-041 | `core/handle_extended.go:220-270` | Pause/unpause doesn't suspend/resume health checks (health status drifts while paused) |
| OB-042 | `core/handle_containers.go:392-451` | Restart doesn't call resolveTmpfsMounts (containers with tmpfs lose their mounts) |
| OB-045 | `docker/containers.go:356-393`, `docker/exec.go:58-95` | Docker attach/exec ignores stdin — unidirectional copy only (interactive sessions broken) |
| OB-046 | `docker/exec.go:39-56` | Docker exec inspect drops all ProcessConfig fields (empty config returned) |
| OB-047 | `docker/server.go:46-106` | Docker backend missing routes: container update, changes, export, commit, resize, exec resize |
| OB-048 | `docker/containers.go:109-122` | Docker mount Consistency field not forwarded to Docker SDK |
| OB-049 | `docker/extended.go:294-328` | Docker system events passed raw without mapping to api.Event type |
| OB-050 | `docker/extended.go:330-412` | Docker system df missing NetworkSettings in container summaries |
| OB-051 | `docker/containers.go:201-239` | Docker container list missing SizeRootFs field mapping |
| OB-052 | `cloudrun/containers.go:182-184` | CloudRun re-start deletes old job without marking ResourceRegistry (phantom orphan entries) |
| OB-053 | `ecs/containers.go:252-258` | ECS forward-mode start failure leaks running task (task not stopped, registry not cleaned) |
| OB-054 | `ecs/eni.go:10-17` | ECS ENI IP extraction dereferences nil pointers (can panic on unexpected SDK response) |
| OB-055 | `lambda/containers.go:390-401`, `gcf/containers.go`, `azf/containers.go` | FaaS kill + background invocation goroutine race on WaitCh and container state |
| OB-056 | `lambda/images.go`, `gcf/images.go`, `azf/images.go` | FaaS image pull uses synthetic config (no registry fetch) — containers lose default CMD/ENV |
| OB-057 | `api/types.go:131-139` | EndpointSettings missing IPAMConfig type (root cause of OB-002/OB-003 — type doesn't exist) |
| OB-058 | `api/types.go:110-117` | Mount missing VolumeOptions/TmpfsOptions struct types (root cause of OB-031 — types don't exist) |
| OB-077 | `core/build.go:519-523` | BuildContexts entry and staging dir leaked if image is built but never used as container |
| OB-078 | `core/handle_images.go:262-299` | Image remove has no container-in-use check — Docker returns 409 Conflict, we delete unconditionally |
| OB-079 | `core/handle_pods.go:180-218` | Force pod remove deletes WaitChs entries without closing channels — waiters blocked forever |
| OB-080 | `core/store.go:190-194` | RevertToCreated closes wait channel; waiter reads exit code 0 for never-started container |
| OB-081 | `core/health.go:84-148` | Health check execs not tracked in Store.Execs or container.ExecIDs — invisible to exec API |
| OB-082 | `core/ipam.go:37-55` | Empty gateway in user-provided IPAM config produces malformed IP base string — all IPs invalid |
| OB-086 | `lambda/containers.go:191-216`, `cloudrun-functions/containers.go:176-219` | FaaS create stores container after Registry.Register — orphan cloud resource on interrupt |
| OB-087 | `aca/extended.go:23-37` | ACA restart doesn't clear JobName/ExecutionName — race with old job still being deleted |
| OB-088 | `docker/containers.go:386` | Docker attach Content-Type always multiplexed-stream — should be raw-stream for TTY containers |
| OB-089 | `docker/exec.go:87` | Docker exec start Content-Type always multiplexed-stream — should be raw-stream for TTY execs |
| OB-091 | `api/types.go:153-169` | ContainerSummary missing HostConfig summary field — docker ps loses NetworkMode |
| OB-092 | `frontends/docker/images.go:12-42` | Frontend handleImageCreate ignores fromSrc param — docker import silently broken |
| OB-093 | `frontends/docker/containers_stream.go:108-110` | Frontend handleContainerResize always returns 200 without forwarding h/w — TTY resize dropped |
| OB-094 | `frontends/docker/exec.go:122-124` | Frontend handleExecResize always returns 200 without forwarding — exec TTY resize dropped |
| OB-095 | `frontends/docker/backend_client.go:184-248` | Frontend dialUpgrade ignores request context — backend connections not cancelled on disconnect |

### Low Severity

| Bug | File | Issue |
|-----|------|-------|
| OB-023 | `core/handle_extended.go:114-133` | Stats streams forever after container exits |
| OB-024 | `core/event_bus.go:82-83` | emitEvent calls time.Now() twice (Time/TimeNano inconsistent) |
| OB-025 | `core/handle_extended.go:324-329` | Container update treats malformed JSON as empty body |
| OB-026 | `docker/networks.go:91` | Docker network inspect drops verbose/scope params |
| OB-027 | `docker/containers.go:248-256` | Docker container start drops checkpoint options |
| OB-028 | `docker/images.go:40-96` | Docker image inspect missing Metadata.LastTagTime |
| OB-029 | `docker/images.go:100` | Docker image load hardcodes quiet=false |
| OB-030 | ECS/CloudRun/ACA | Cloud background goroutines call StopContainer on deleted containers |
| OB-059 | `core/handle_extended.go:282-323` | Core events handler only filters by "type" — ignores event/container/image/label filters |
| OB-060 | `core/handle_images.go`, `core/handle_extended.go`, `core/handle_volumes.go` | Missing Docker-compatible events for image tag/remove, volume create/destroy |
| OB-061 | `core/handle_extended.go:359-405` | System df image count wrong — aliases not deduplicated by image ID |
| OB-062 | `core/handle_extended.go:178-218` | Container rename doesn't update network Containers map (stale name in network inspect) |
| OB-063 | `core/handle_extended.go:325-347` | Container update ignores resource fields — only handles RestartPolicy |
| OB-064 | `core/handle_images.go:181-209` | Image load always stores as "loaded:latest" (overwrites, no manifest extraction) |
| OB-065 | All 6 cloud backends | Prune handlers skip AgentRegistry.Remove (stale entries for stopped containers) |
| OB-066 | `lambda/logs.go`, `gcf/logs.go`, `azf/logs.go` | FaaS logs endpoint ignores buffered LogBuffers data (only queries cloud logging) |
| OB-067 | `docker/client.go:22` | handleInfo uses context.Background() instead of request context |
| OB-068 | `api/types.go` | API types missing fields: NetworkSettings top-level (Gateway, IPAddress, Bridge, etc.), HostConfig (DNS, Memory, CpuShares, PidMode, etc.), Event.Scope, HealthcheckConfig.StartInterval, EndpointSettings IPv6 fields, Image.GraphDriver, ExecInstance.DetachKeys |
| OB-096 | `core/handle_exec.go`, `core/store.go` | die event never emitted for synthetic/exec auto-stop paths |
| OB-097 | `core/handle_extended.go:220-251` | Pause check-then-act race — container can be paused after it has exited |
| OB-098 | `core/handle_containers_archive.go:109-143` | Symlinks and hardlinks silently dropped during tar extraction |
| OB-099 | `core/handle_pods.go:55-84` | Pod Hostname/SharedNS set outside registry mutex — concurrent read race |
| OB-100 | `core/handle_containers_query.go:82-105` | Malformed Created timestamp causes incorrect before/since filter results |
| OB-101 | `core/drivers_network.go:116-158` | Redundant network connect allocates second IP without releasing first — IP leak |
| OB-102 | `core/handle_containers.go:216` | Start event emitted with pre-update container snapshot (fragile, stale reference) |
| OB-103 | `ecs/containers.go:522-525` | ECS kill exitCh — background polling goroutine continues after force-stop (self-terminates eventually) |
| OB-104 | `ecs/containers.go:551-576`, `cloudrun/containers.go:579`, `aca/containers.go:565` | Cloud force remove + background poller race — StopContainer called twice (may double-close channel) |
| OB-105 | `aca/containers.go:748-765`, `cloudrun/containers.go:738` | ACA/CloudRun fire-and-forget LRO pollers — stop/delete jobs not actually waited on |
| OB-106 | `lambda/recovery.go:14-43` | Lambda ScanOrphanedResources no pagination — misses functions beyond first 50 |
| OB-107 | `azure-functions/recovery.go:54-66` | AZF CleanupResource always no-op — AZF state not persisted, lookup always fails at startup |
| OB-108 | `lambda/containers.go:248-296` | Lambda start goroutine calls StopContainer on already-removed container (benign but noisy) |
| OB-109 | `docker/extended.go:209,378`, `docker/images.go:48` | Docker VirtualSize hardcoded to Size in image list/inspect/df — loses shared layer info |
| OB-110 | `docker/networks.go:157` | Docker network disconnect returns 200 instead of 204 No Content |
| OB-111 | `docker/containers.go:610-612` | mapDockerError loses container ID from Docker not-found error messages |
| OB-112 | `api/types.go:398-407` | Volume missing UsageData field — docker system df shows no volume usage data |
| OB-113 | `api/types.go:589-595` | DiskUsageResponse missing BuildCache field and BuildCache type |
| OB-114 | `api/types.go:492-496` | EndpointSettings missing Links field — docker network connect --link silently dropped |
| OB-115 | `api/types.go:368-378` | NetworkCreateRequest missing CheckDuplicate field |
| OB-116 | `frontends/docker/images.go:177-195` | Frontend handleImageBuild missing query params: labels, target, platform, pull, etc. |
| OB-117 | `frontends/docker/images.go:210-224` | Frontend handleContainerCommit missing pause and changes params |
| OB-118 | `frontends/docker/networks.go:52-61` | Frontend handleNetworkInspect drops verbose/scope params — never forwarded to backend |
| OB-119 | `frontends/docker/system.go:65-115` | Frontend handleInfo missing LoggingDriver field — may break Compose log driver detection |
| OB-120 | `frontends/docker/system.go:11-18` | Frontend handlePing sends body on HEAD; missing Content-Length and Builder-Version headers |

## False Positives

Bugs investigated and confirmed as non-issues. Tracked here to avoid re-collecting.

| ID | File | Reported Issue | Why False Positive |
|----|------|----------------|-------------------|
| FP-001 | `api/types.go:554-558` | Event Type/Action/Actor use uppercase JSON tags | Docker API uses PascalCase for these fields — current tags are correct |
