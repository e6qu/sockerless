# Known Bugs

## Fixed (BUG-001 → BUG-138)

138 bugs fixed across 18 sprints. See `WHAT_WE_DID.md` for sprint summaries and `_tasks/done/BUG-SPRINT-*.md` for per-sprint details.

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

## Open Bugs

| Bug | Sev | File | Issue |
|-----|-----|------|-------|
| OB-001 | High | `core/restart_policy.go:73-91` | restart_policy Start() failure leaves inconsistent state (WaitCh + Running not reverted) |
| OB-002 | High | `docker/containers.go:127-141` | Docker container create drops IPAMConfig (static IP silently fails) |
| OB-003 | High | `docker/extended.go:134-153` | Docker network connect drops IPAMConfig (static IP on connect silently fails) |
| OB-004 | Med | `core/handle_containers.go:72-91` | Container create leaks on pod validation failure |
| OB-005 | Med | `core/handle_pods.go:180-219` | Pod remove without force doesn't check running containers |
| OB-006 | Med | `core/handle_pods.go:135-178` | Pod start/stop/kill don't cascade to containers |
| OB-010 | Med | `core/handle_containers.go`, `core/handle_extended.go` | Container remove/prune missing pod registry cleanup |
| OB-013 | Med | `core/handle_images.go:226-231` | Image tag doesn't update old alias store entries |
| OB-014 | Med | `core/handle_containers_archive.go:251-264` | tmpfs temp dirs never cleaned up |
| OB-015 | Med | `docker/containers.go:347-355` | Docker attach drops logs/detachKeys params |
| OB-016 | Med | `docker/containers.go:303-313` | Docker logs drops details param |
| OB-018 | Med | `docker/extended.go:324-390` | Docker system df drops BuildCache |
| OB-020 | Med | `ecs/extended.go` | ECS restart leaks old task ARN in ResourceRegistry |
| OB-022 | Med | All 6 cloud backends | Cloud remove force-path uses StopContainer (not ForceStop) |
| OB-023 | Low | `core/handle_extended.go:114-133` | Stats streams forever after container exits |
| OB-024 | Low | `core/event_bus.go:82-83` | emitEvent calls time.Now() twice (Time/TimeNano inconsistent) |
| OB-025 | Low | `core/handle_extended.go:324-329` | Container update treats malformed JSON as empty body |
| OB-026 | Low | `docker/networks.go:91` | Docker network inspect drops verbose/scope params |
| OB-027 | Low | `docker/containers.go:248-256` | Docker container start drops checkpoint options |
| OB-028 | Low | `docker/images.go:40-96` | Docker image inspect missing Metadata.LastTagTime |
| OB-029 | Low | `docker/images.go:100` | Docker image load hardcodes quiet=false |
| OB-030 | Low | ECS/CloudRun/ACA | Cloud background goroutines call StopContainer on deleted containers |
| OB-031 | Med | `docker/containers.go:109-122` | Docker mount mapping drops VolumeOptions and TmpfsOptions |
