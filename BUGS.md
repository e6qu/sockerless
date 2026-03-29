# Known Bugs

612 bugs fixed across 45 sprints + ECS live testing (BUG-001 through BUG-612). 0 open bugs. See [STATUS.md](STATUS.md) for overall project status.

## Fixed Bugs (synthetic behavior elimination, 2026-03-29)

| ID | Severity | Summary |
|----|----------|---------|
| BUG-595 | Medium | `SecurityGroupID` → `SecurityGroupIDs []string` for multiple Docker networks |
| BUG-596 | Medium | Real image size from manifest layer sizes instead of `fnv32(ref)` random |
| BUG-597 | Low | Real layer digests in pull progress instead of hardcoded "abc123" |
| BUG-598 | Medium | Real config digest as image ID instead of `sha256(ref_string)` |
| BUG-599 | Low | Real `diff_ids` for RootFS.Layers instead of single random hash |
| BUG-600 | Low | Real build history from OCI config blob instead of fake entries |
| BUG-601 | High | `StatsProvider` interface + ECS CloudWatch Container Insights metrics |
| BUG-602 | Medium | `ContainerTop` via agent exec `ps` instead of hardcoded single process |
| BUG-603 | Medium | Real ENI IP always overwrites synthetic; MAC derived from real IP |
| BUG-604 | Medium | `fargateResources()` maps HostConfig to valid Fargate CPU/memory combos |
| BUG-605 | Low | `hostKernelVersion()` via `uname -r` instead of hardcoded string |
| BUG-606 | Low | Real manifest digest for RepoDigests instead of `sha256(ref)` |
| BUG-607 | Medium | OCI push uses real layer content from `LayerContent` store when available |
| BUG-608 | Medium | Deferred — `docker build` RUN requires cloud build service integration |
| BUG-609 | Medium | `docker load` preserves layer tarballs in `LayerContent` store |
| BUG-610 | Low | `ContainerChanges` returns NotImplemented when no agent (not empty list) |
| BUG-611 | Low | `ContainerExport` returns NotImplemented when no agent (not empty tar) |
| BUG-612 | Medium | `FetchImageMetadata` logs warning on fetch failure instead of silent nil |

## Fixed Bugs (ECS live manual testing, 2026-03-29)

| ID | Severity | Summary |
|----|----------|---------|
| BUG-584 | High | Security group from `cloudNetworkConnect` not used in `runECSTask` — network isolation broken |
| BUG-585 | Medium | `ContainerLogs` log stream name uses empty task ID for never-started containers |
| BUG-586 | Medium | Bind mount volumes have no EFS backing — silently empty on Fargate |
| BUG-587 | Medium | `ContainerRestart` reuses deregistered task definition (stale `TaskDefARN`) |
| BUG-588 | Low | `ContainerStart` marked running before `RunTask` succeeded (false-positive window) |
| BUG-589 | Low | `region` variable in ECS terraform module not wired to output |
| BUG-591 | High | `FetchImageConfig` fails on Docker Hub — X-Registry-Auth passed as basicAuth causes 400 from token endpoint; image config merge only updates primary key, leaving aliases with stale synthetic config (`/bin/sh`). Nginx and any image relying on CMD/ENTRYPOINT exits immediately |
| BUG-592 | Medium | DX: Starting ECS backend requires 10+ env var exports — terragrunt output should provide a single `source`-able env file or a `sockerless connect` command |
| BUG-593 | Medium | DX: `docker run` for short-lived containers shows no inline output — CloudWatch log ingestion latency means stdout isn't available until seconds after the task exits; `docker logs <id>` works after a delay |
| BUG-594 | Low | Orphaned in-memory containers from prior server sessions (e.g. `ecs-exec-212439`) persist across restarts showing "Up 25 days" — recovery should reconcile against actual ECS cluster state and clean up stale entries |

Per-sprint details in `_tasks/done/BUG-SPRINT-*.md`.

## False Positives

Bugs investigated and confirmed as non-issues.

| ID | Reason |
|----|--------|
| FP-001 | Docker API uses PascalCase for Event fields — current tags are correct |
| FP-002 | EndpointIPAMConfig already exists (added in BUG-175) |
| FP-003 | Changing Container.State/Config/HostConfig to pointers would break too much code |
| FP-004 | Docker emits `kill` then `die` — current order is correct |
| FP-005 | No real filesystem change tracking (known limitation) |
| FP-006 | Stats network stats always zero (known limitation) |
| FP-007 | Missing Ulimits/DeviceRequests/Devices — lower priority |
| FP-008 | FaaS stop doesn't cancel invocations — by design |
| FP-009 | FaaS validation order is correct |
| FP-010 | Docker ContainerUpdate direct pass-through is correct |
| FP-011 | Docker ExecInspect ProcessConfig doesn't include env/workingDir |
| FP-012 | Already fixed in Sprint 43 (BUG-541) |
| FP-013 | Already fixed in Sprint 43 (BUG-542) |
| FP-014 | ConsoleSize matches Docker SDK type |
| FP-015 | Cloud backends don't support pause semantics |
| FP-016 | Cloud backends use registries, not image load |
| FP-017 | BUG-590: exitCh double-close between ContainerKill/pollTaskExit — all paths use `LoadAndDelete`, safe |
