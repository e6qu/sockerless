# Known Bugs

## Fixed (BUG-001 → BUG-344)

319 bugs fixed across 28 sprints. See `WHAT_WE_DID.md` for sprint summaries and `_tasks/done/BUG-SPRINT-*.md` for per-sprint details.

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
