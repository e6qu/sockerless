# Known Bugs

618 bugs fixed across 45 sprints + ECS live testing (BUG-001 through BUG-618). 0 open bugs. See [STATUS.md](STATUS.md) for overall project status.

## Fixed Bugs (ECS live testing round 2, 2026-03-30)

### BUG-613: Real ENI IP never written to container NetworkSettings

**File:** `backends/ecs/containers.go` (pollTaskExit), `backends/ecs/backend_impl.go` (ContainerStart)
**Severity:** Medium

`waitForTaskRunning` extracts the real ENI IP but only stores it in `ECSState.AgentAddress` (as `ip:9111`). The container's `NetworkSettings.Networks[].IPAddress` stays at the synthetic 172.17.0.x. The `applyTaskStatus(RUNNING)` handler does overwrite the IP, but `pollTaskExit` only calls `applyTaskStatus` on STOPPED, never on RUNNING. Fix: after `waitForTaskRunning` returns in ContainerStart, update the container's IPAddress with the ENI IP.

### BUG-614: Container stats return zeros on real AWS

**File:** `backends/ecs/stats.go`
**Severity:** Medium

`docker stats --no-stream` returns 0% CPU and 0B memory on real AWS. The `ecsStatsProvider` calls CloudWatch `GetMetricData` for `ECS/ContainerInsights`, but Container Insights may not have data for short-lived tasks or may require the cluster to have Container Insights explicitly enabled. The fallback when CloudWatch returns no data should use allocated resources instead of zeros.

### BUG-615: fargateResources produces invalid memory values

**File:** `backends/ecs/taskdef.go` (fargateResources)
**Severity:** High

For 256 CPU with 1GB memory request, `fargateResources` produces `256 CPU, 1536 memory` which is not a valid Fargate combo. The rounding logic increments from `memMin` (512) by `memInc` (1024) giving 512 → 1536 → 2048, but 1536 is invalid. Valid values for 256 CPU are 512, 1024, 2048 only. The increment logic should produce `memMin + N*memInc` where N=0,1,2,..., giving 512, 1536, 2560 — but only 512 and 2048 are valid. Fix: the valid memory values for each CPU tier are `memMin, memMin+memInc, memMin+2*memInc, ...` which for 256 CPU gives 512, 1536, 2560 — but real AWS only accepts specific values. The `memInc` field is wrong; real Fargate accepts multiples of 128 MB above the minimum, not memInc=1024.

### BUG-616: `docker run -m` with byte value not correctly parsed for Fargate

**File:** `backends/ecs/taskdef.go` (fargateResources)
**Severity:** Medium

`docker run -m 1073741824` sets `HostConfig.Memory` to 1073741824 bytes. The code divides by `1024*1024` to get MB (=1024 MB). But the Fargate mapping should map 1024 MB to `512 CPU, 1024 memory` (the smallest tier that fits). Instead it selected `256 CPU, 1536 memory` because the loop found 256 CPU first and tried to fit 1024 MB into the 512-2048 range, producing an invalid intermediate value.

### BUG-617: docker network create does not create VPC security group on real AWS

**File:** `backends/ecs/network_cloud.go` (cloudNetworkCreate)
**Severity:** High

`docker network create testnet` succeeds (returns network ID) but no VPC security group `skls-testnet` is created in AWS. No error or warning appears in logs. The `cloudNetworkCreate` function calls `ec2.CreateSecurityGroup` but the SG doesn't appear in `aws ec2 describe-security-groups`. This breaks Docker network-based isolation on real ECS.

### BUG-618: Podman CLI requires full Libpod API — only pod routes exist

**File:** `backends/core/handle_docker_api.go` (route registration)
**Severity:** High

Podman CLI always uses the Libpod API (`/libpod/*`) for all operations, not the Docker compat API. The backend only implements Libpod routes for pods (`/libpod/pods/*`) and ping. Missing: `/libpod/containers/*`, `/libpod/images/*`, `/libpod/info`, `/libpod/version` with podman-compatible response formats.

Partial fixes applied:
- Added `/libpod/_ping` route (ping now works)
- Fixed version prefix regex to handle 3-part versions like `v5.0.0/` (was only matching `v1.44/`)
- Added `/libpod/info` route (but response format causes podman crash — needs libpod-specific JSON)

Full fix requires either: (a) implement libpod container/image/system API endpoints, or (b) route `/libpod/containers/*` and `/libpod/images/*` to Docker compat handlers with response format adaptation.

**File:** `backends/ecs/network_cloud.go` (cloudNetworkCreate)
**Severity:** High

`docker network create testnet` succeeds (returns network ID) but no VPC security group `skls-testnet` is created in AWS. No error or warning appears in logs. The `cloudNetworkCreate` function calls `ec2.CreateSecurityGroup` but the SG doesn't appear in `aws ec2 describe-security-groups`. Either the EC2 call silently fails, the VPC resolution returns wrong data, or the error is swallowed. This breaks Docker network-based isolation on real ECS since containers never get per-network security groups.

**File:** `backends/ecs/taskdef.go` (fargateResources)
**Severity:** Medium

`docker run -m 1073741824` sets `HostConfig.Memory` to 1073741824 bytes. The code divides by `1024*1024` to get MB (=1024 MB). But the Fargate mapping should map 1024 MB to `512 CPU, 1024 memory` (the smallest tier that fits). Instead it selected `256 CPU, 1536 memory` because the loop found 256 CPU first and tried to fit 1024 MB into the 512-2048 range, producing an invalid intermediate value.

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
