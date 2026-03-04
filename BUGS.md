# Known Bugs

## Fixed (BUG-001 → BUG-251)

251 bugs fixed across 23 sprints. See `WHAT_WE_DID.md` for sprint summaries and `_tasks/done/BUG-SPRINT-*.md` for per-sprint details.

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

## Open Bugs

### Medium Severity

| Bug | File | Issue |
|-----|------|-------|
| OB-018 | `docker/extended.go:324-390` | Docker system df drops BuildCache |
| OB-056 | `lambda/images.go`, `gcf/images.go`, `azf/images.go` | FaaS image pull uses synthetic config (no registry fetch) — containers lose default CMD/ENV |

### Low Severity

| Bug | File | Issue |
|-----|------|-------|
| OB-060 | `core/handle_images.go`, `core/handle_extended.go`, `core/handle_volumes.go` | Missing Docker-compatible events for image tag/remove, volume create/destroy |
| OB-063 | `core/handle_extended.go:325-347` | Container update ignores resource fields — only handles RestartPolicy |
| OB-064 | `core/handle_images.go:181-209` | Image load always stores as "loaded:latest" (overwrites, no manifest extraction) |
| OB-066 | `lambda/logs.go`, `gcf/logs.go`, `azf/logs.go` | FaaS logs endpoint ignores buffered LogBuffers data (only queries cloud logging) |
| OB-068 | `api/types.go` | API types missing fields: NetworkSettings top-level (Gateway, IPAddress, Bridge, etc.), HostConfig (DNS, Memory, CpuShares, PidMode, etc.), HealthcheckConfig.StartInterval, EndpointSettings IPv6 fields, Image.GraphDriver, ExecInstance.DetachKeys |
| OB-103 | `ecs/containers.go:522-525` | ECS kill exitCh — background polling goroutine continues after force-stop (self-terminates eventually) |
| OB-104 | `ecs/containers.go:551-576`, `cloudrun/containers.go:579`, `aca/containers.go:565` | Cloud force remove + background poller race — StopContainer called twice (may double-close channel) |
| OB-105 | `aca/containers.go:748-765`, `cloudrun/containers.go:738` | ACA/CloudRun fire-and-forget LRO pollers — stop/delete jobs not actually waited on |
| OB-108 | `lambda/containers.go:248-296` | Lambda start goroutine calls StopContainer on already-removed container (benign but noisy) |
| OB-113 | `api/types.go:589-595` | DiskUsageResponse missing BuildCache field and BuildCache type |
| OB-116 | `frontends/docker/images.go:177-195` | Frontend handleImageBuild missing query params: labels, target, platform, pull, etc. |
| OB-117 | `frontends/docker/images.go:210-224` | Frontend handleContainerCommit missing pause and changes params |

## False Positives

Bugs investigated and confirmed as non-issues. Tracked here to avoid re-collecting.

| ID | File | Reported Issue | Why False Positive |
|----|------|----------------|-------------------|
| FP-001 | `api/types.go:554-558` | Event Type/Action/Actor use uppercase JSON tags | Docker API uses PascalCase for these fields — current tags are correct |
| FP-002 | `api/types.go` | EndpointSettings missing IPAMConfig type (OB-057) | EndpointIPAMConfig already exists since Sprint 20 — the type was added in BUG-175 |
