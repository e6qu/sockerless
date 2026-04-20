# Cloud Resource Mapping

Authoritative mapping between Docker / Podman concepts and the cloud resources that back them in each Sockerless backend. The corollary: **state derives from cloud actuals**. After a backend restart, every list / inspect / stop / exec call must reproduce the same answer by querying the cloud APIs of its configured environment — no in-memory map, on-disk JSON, S3 object, or DynamoDB row may be consulted as the source of truth.

This document is the source of truth for Phase 89 (stateless-backend audit, BUG-723..726).

> **Companion specs:**
> - [BACKEND_STATE.md](BACKEND_STATE.md) — the stateless principle, identity model, tagging conventions
> - [SIMULATOR_RECOVERY.md](SIMULATOR_RECOVERY.md) — recovery on restart, PID re-attachment, simulator-side tag handling
> - [BACKENDS.md](BACKENDS.md) — per-backend implementation overview
>
> Per-backend `docker_api_mapping.md` files (under `backends/<name>/docs/`) describe the call-by-call translation; this file describes the durable resource mapping.

---

## Universal rules

1. **Cloud resources are tagged at creation** with `sockerless-managed=true` plus identity tags so they can be enumerated and reattributed after restart.
2. **Every list / inspect call queries the cloud first.** In-memory caches are allowed but must be invalidatable, must be rebuilt on miss from cloud actuals, and must never be the source of truth.
3. **Persistent on-disk state is forbidden.** No `~/.sockerless/state/*.json`, no S3 buckets, no DynamoDB. The only file paths backends touch on disk are: configuration (read-only), credentials (read-only), and CLI run-state (PID files etc.).
4. **State buckets / lock tables for terraform are infrastructure, not sockerless state** — they hold Terraform's state for the operator-managed infra and have nothing to do with backend operation.
5. **A "container" in the docker API is whatever the cloud calls a single container of work** — task, function invocation, app revision, job execution. A "pod" in the libpod API is a *group* of containers, which in clouds without first-class pods is a multi-container task / multi-container app.

---

## Mapping per cloud

### AWS ECS (backend `ecs`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | ECS task (Fargate) | `task ARN` (cloud), `containerID` (Docker) | `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>`, `sockerless-instance=<backend-instance-id>` |
| Pod (libpod) | ECS task with multi-container task definition | `task ARN`, `pod name` | + `sockerless-pod=<name>` |
| Image | ECR repository / image | `<account>.dkr.ecr.<region>.amazonaws.com/<repo>:<tag>` | (registry-managed) |
| Network (user-defined) | EC2 security group + Cloud Map private DNS namespace | `sg-…` + `ns-…` | `sockerless:network=<name>`, `sockerless:network-id=<id>` |
| Volume (named) | EFS access point or empty volume in the task definition | (depends on backend EFS config) | (currently per-task-def; durable named volumes Phase-89-pending) |
| Exec instance | ECS `ExecuteCommand` session | (transient SSM session) | (transient — no recovery needed) |

**State derivation (implemented in Phase 89):**

- `docker ps -a` → `ListTasks` RUNNING+STOPPED + `DescribeTasks(Include=TAGS)` filtered to `sockerless-managed=true`, projected via `taskToContainer`.
- `docker pod ps` → same task query grouped by `sockerless-pod` tag; `ecsCloudState.ListPods`.
- `docker network ls` → `DescribeSecurityGroups(tag:sockerless:network-id=<id>)` + `ListNamespaces(DNS_PRIVATE) → ListTagsForResource(tag:sockerless:network-id=<id>)`.
- `docker images` → `DescribeRepositories` + `DescribeImages` → `ImageSummary` with ECR RepoTags/RepoDigests; `ecsCloudState.ListImages`.
- `docker exec` → `ecsCloudState.resolveTaskARN(containerID)` via tag filter, then `ExecuteCommand`.
- `docker stop/kill/rm/restart/wait/logs/ExecCreate` → all go through `Server.resolveTaskState(ctx, containerID)` cache+cloud-fallback helper.

**In-memory state as a cache (post-Phase-89):**

- `s.ECS *StateStore[ECSState]` — transient cache; every cloud-identity callsite uses `resolveTaskState` which repopulates on miss.
- `s.NetworkState *StateStore[NetworkState]` — transient cache; populated on create, recovered via `resolveNetworkState` on miss.
- `s.VolumeState *StateStore[VolumeState]` — transient cache (volume state is simpler; follows same pattern).

### AWS Lambda (backend `lambda`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Lambda function | `function ARN`, `containerID` | function tags: `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | Multi-container pod is **not supported** by Lambda — one function = one container. Pods would require a coordinator (e.g. Step Functions); not in scope. | — | — |
| Image | ECR repository / image | `<account>.dkr.ecr.<region>.amazonaws.com/<repo>:<tag>` | (registry-managed) |
| Network | **Native cross-container DNS is not addressable per-execution.** Lambda VPC config only routes egress; peer-Lambda discovery requires Service Discovery + a separate fronting service. Treat docker networks as bookkeeping only. | (no cloud anchor) | (Phase 89 follow-up: file as known limitation) |
| Volume | Lambda layers (read-only) or `/tmp` (per-invocation, ephemeral). Bind mounts and named volumes outside `/tmp` are not supported. | — | — |
| Exec instance | **Implemented via the agent overlay**: `cloudExecStart` dials the reverse-agent WebSocket (registered by `sockerless-lambda-bootstrap` during `Invoke`) and tunnels the command. | (transient agent session) | — |

**State derivation (implemented in Phase 89):**

- `docker ps -a` → `ListFunctions` + `ListTags` per function ARN (filter `sockerless-managed=true`), project to `api.Container`.
- `docker images` → `lambdaCloudState.ListImages` paginates ECR `DescribeRepositories` + `DescribeImages` (same ECR that ECS uses).
- `docker exec` → `resolveLambdaState` for FunctionName → dial reverse-agent WebSocket → tunnel through overlay.
- `docker stop/kill/rm/wait/logs` → all go through `Server.resolveLambdaState(ctx, containerID)` cache+cloud-fallback helper.

**In-memory state as a cache (post-Phase-89):**

- `s.Lambda *StateStore[LambdaState]` — transient cache; `resolveLambdaState` recovers `FunctionARN` + `FunctionName` on miss via `ListFunctions` + tag filter.

### GCP Cloud Run (backend `cloudrun`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | **Current:** Cloud Run **Job** + execution (`run.googleapis.com/v2`). **Post-Phase 87:** Cloud Run **Service** with internal ingress + VPC connector. | job name `sockerless-<containerID[:12]>` + execution id | label `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | **Current:** not supported (1 Job = 1 container). **Post-Phase 87:** Cloud Run Service with multi-container revision (sidecars). | revision ref + sidecar container names | + label `sockerless-pod=<name>` |
| Image | Artifact Registry / GCR | `<region>-docker.pkg.dev/<project>/<repo>/<image>:<tag>` | (registry-managed) |
| Network | Cloud DNS private managed zone (1 zone per docker network, sanitized from name). **Post-Phase 87** also needs VPC connector + internal-ingress Service IP for cross-container routing to actually work (currently the A-records point at placeholder `0.0.0.0` per BUG-715). | managed-zone name | label `sockerless:network=<name>`, `sockerless:network-id=<id>` (Phase 89 follow-up) |
| Volume | Cloud Storage Fuse mount (per-revision config) — currently bookkeeping only on the Jobs path. | bucket/prefix | — |
| Exec instance | **Not supported natively** by Cloud Run Services / Jobs. Must go through the agent overlay (same pattern as Lambda) — Phase 87 deliverable. | — | — |

**State derivation:**

- `docker ps -a` → **Current:** `Jobs.ListJobs` + `Executions.ListExecutions` per job, filter by label `sockerless-managed=true`. **Post-Phase 87:** `Services.ListServices`.
- `docker stop` → **Current:** `Jobs.CancelExecution` on the active execution. **Post-Phase 87:** `Services.DeleteService` (or revision rollback).
- `docker network ls` → `ManagedZones.List` filter by label `sockerless:network=*`.
- `docker images` → Artifact Registry `Images.List` filtered by repo path.
- `docker logs` → Cloud Logging `LogAdmin.Entries(filter='resource.type="cloud_run_revision" labels.execution_name="<exec>"')`.

**In-memory state as a cache (post-Phase-89):**

- `s.CloudRun *StateStore[CloudRunState]` — transient cache; `resolveCloudRunState` recovers `JobName` (via `ListJobs` + label filter on `sockerless_container_id`) + `ExecutionName` (via `ListExecutions`, filter to non-terminal) on miss.
- `s.NetworkState *StateStore[NetworkState]` — transient cache; `resolveNetworkState` recovers `ManagedZoneName` via `ManagedZones.Get(<sanitized>)`.
- `docker images` cloud-derived via `cloudRunCloudState.ListImages` using the shared `core.OCIListImages` against `<region>-docker.pkg.dev`.

### Azure Container Apps (backend `aca`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | **Current:** ACA **Job** + execution (`armcontainerapps.JobsClient`). **Post-Phase 88:** ACA **App** with internal ingress (`armcontainerapps.ContainerAppsClient`). | job name `sockerless-<containerID[:12]>` + execution id | tag `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | **Current:** not supported. **Post-Phase 88:** ACA App with multi-container template (sidecars). | app name + sidecar container names | + tag `sockerless-pod=<name>` |
| Image | ACR | `<acrName>.azurecr.io/<repo>:<tag>` | (registry-managed) |
| Network | Azure Private DNS Zone (per-network) + per-network NSG. Cross-container DNS via A-records currently broken on Jobs (BUG-716 — placeholder IPs); fixed when Phase 88 moves to Apps with internal ingress. | zone name + NSG id | tag `sockerless:network=<name>`, `sockerless:network-id=<id>` (Phase 89 follow-up) |
| Volume | Azure Files share via ACA volumes (per-Job/App config) | mount config | — |
| Exec instance | ACA exec console (`Jobs.NewListSecretsPager` is the data-plane analog) — different proto from SSM. Phase 88 deliverable. | — | — |

**State derivation:**

- `docker ps -a` → **Current:** `JobsClient.NewListByResourceGroupPager(rg)` + `JobsExecutionsClient.NewListPager(rg, jobName)` for active executions, filter by tag `sockerless-managed=true`. **Post-Phase 88:** `ContainerAppsClient.NewListByResourceGroupPager(rg)`.
- `docker stop` → **Current:** `JobsExecutionsClient.BeginStop(rg, jobName, execName)`. **Post-Phase 88:** `ContainerAppsClient.BeginStop(rg, appName)`.
- `docker network ls` → `PrivateZonesClient.NewListByResourceGroupPager(rg)` filter by tag `sockerless:network=*`.
- `docker images` → ACR `RegistryClient.NewListImportImagesPager` for the configured ACR.
- `docker logs` → Log Analytics workspace queries on `ContainerAppConsoleLogs_CL` filtered by container app + execution name.

**In-memory state as a cache (post-Phase-89):**

- `s.ACA *StateStore[ACAState]` — transient cache; `resolveACAState` recovers `JobName` (via `Jobs.NewListByResourceGroupPager` + tag filter) + `ExecutionName` (via `Executions.NewListPager`, filter to `Running`/`Processing`) on miss.
- `s.NetworkState *StateStore[NetworkState]` — transient cache; `resolveNetworkState` recovers `DNSZoneName` via `PrivateDNSZones.Get(skls-<net>.local)` and `NSGName` via `NSG.Get(nsg-<env>-<net>)`.
- `docker images` cloud-derived via `acaCloudState.ListImages` using the shared `core.OCIListImages` against `<ACRName>.azurecr.io`.

### GCP Cloud Run Functions (backend `cloudrun-functions` / `gcf`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Cloud Function (gen 2) — backed by `cloudfunctions.v2.FunctionService` | function name `sockerless-<containerID[:12]>`, function name + revision | label `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | **Not supported.** Cloud Functions are 1-to-1 with a container; there is no first-class group abstraction. Multi-container pods would require a coordinator (e.g. Workflows + Pub/Sub) and are out of scope. | — | — |
| Image | Artifact Registry (the function's deployed container image) | `<region>-docker.pkg.dev/<project>/<repo>/<image>:<tag>` | (registry-managed) |
| Network | **Not supported natively.** Cloud Functions can connect to a VPC for egress via a connector, but they don't expose addressable inbound IPs to peer functions. Cross-container DNS via a docker-network abstraction is not implementable on Cloud Functions; backend treats `docker network create` / `connect` as a no-op for cloud purposes (returns success but the network is bookkeeping only). | (no cloud anchor) | — |
| Volume | **Not supported.** Cloud Functions have read-only filesystems plus `/tmp`. Bind mounts and named volumes are rejected at create time. | — | — |
| Exec instance | **Not supported natively.** Like Lambda, exec must go through the agent overlay (function bootstrap dials back to sockerless via reverse-WebSocket). Implementation parallels `sockerless-lambda-bootstrap`; pending. | — | — |

**State derivation:**

- `docker ps -a` → `Functions.ListFunctions(parent="projects/<project>/locations/<region>")`, filter by label `sockerless-managed=true`, project to `api.Container`. Recovery already implemented in `backends/cloudrun-functions/recovery.go`.
- `docker stop` → `Functions.DeleteFunction(name)` (Cloud Functions have no in-place stop; deletion is the analog).
- `docker images` → `gcfCloudState.ListImages` via the shared `core.OCIListImages` against `<region>-docker.pkg.dev` with token from `ARAuthProvider`.
- `docker logs` → Cloud Logging `LogAdmin.Entries(filter='resource.type="cloud_function" labels.function_name="<name>"')`.

**In-memory state as a cache (post-Phase-89):**

- `s.GCF *StateStore[GCFState]` — transient cache for `FunctionName`; backend's `ContainerStop/Kill/Remove` paths call `CloudState` directly for lookups since GCF has only one cloud-identity field.

### Azure Functions (backend `azure-functions` / `azf`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Function App (Linux container deployment) — `armappservice.WebAppsClient` | function app name `sockerless-<containerID[:12]>` | tag `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | Multi-container Function App is **not supported** (Function Apps are 1-container). Pod deletion path does delete the underlying app, but pods are local-bookkeeping only. | — | — |
| Image | ACR | `<acrName>.azurecr.io/<repo>:<tag>` | (registry-managed) |
| Network | **Not supported natively.** Function Apps support VNet integration for outbound traffic but not addressable inbound IPs for peer apps. `docker network create` / `connect` is bookkeeping-only. | — | — |
| Volume | **Not supported** for arbitrary bind mounts. App settings + Azure Files share via App Service mounts can be configured but aren't auto-translated from `--volume`. | — | — |
| Exec instance | **Not supported natively.** Kudu console + SSH are the App Service equivalents but use a different protocol from SSM. Agent overlay would be needed; pending. | — | — |

**State derivation:**

- `docker ps -a` → `WebApps.NewListByResourceGroupPager(resourceGroup)`, filter by tag `sockerless-managed=true`, project to `api.Container`.
- `docker stop` → `WebApps.Stop(name)` (function app stays defined but doesn't run).
- `docker rm` → `WebApps.Delete(name)`.
- `docker images` → `azfCloudState.ListImages` via the shared `core.OCIListImages` against `config.Registry` (the ACR hostname) with token from `ACRAuthProvider`.
- `docker logs` → App Service container logs via `WebApps.GetContainerLogsZip` or `LogAnalytics` queries on the workspace linked to the App.

**In-memory state as a cache (post-Phase-89):**

- `s.AZF *StateStore[AZFState]` — transient cache for `FunctionAppName`.

### Local Docker (backend `docker`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Docker container on the local daemon | container ID, name | (Docker labels — `sockerless-managed=true` for filtering when a single daemon hosts both sockerless and non-sockerless containers) |
| Pod | **Podman pod** when the local daemon is podman; not natively supported by docker. Implemented via the local pod registry that delegates to `podman pod` commands. | pod ID | — |
| Image | Local image cache | image ID, ref | — |
| Network | Docker user-defined network | network ID, name | label `sockerless-managed=true` |
| Volume | Docker named volume | volume name | label `sockerless-managed=true` |
| Exec instance | Docker exec (native) | exec ID | (transient) |

**State derivation:**

- The local Docker / Podman daemon IS the source of truth. The backend forwards every docker API call to the daemon; no additional state-of-truth mapping is required. Sockerless still tags resources it creates so that `docker ps --filter label=sockerless-managed=true` cleanly partitions sockerless-owned objects from anything else on the same daemon.

---

## State boundaries

These are the only places sockerless backends are allowed to keep state:

1. **Configuration** (read-only at startup): `~/.sockerless/contexts/*/config.json`, env vars.
2. **In-memory caches**: anything queried from cloud actuals, scoped to the backend lifetime, invalidated on miss.
3. **CLI run-state** (the management binary `cmd/sockerless`, not the backend itself): `~/.sockerless/run/<context>/backend.pid`.
4. **Per-process transient state**: HTTP-request-scoped, exec-session-scoped, etc. — torn down with the request.

Forbidden:

- `~/.sockerless/state/images.json` (BUG-723 — Store.Images persistence). **Removed** in Phase 89; all 6 cloud backends now derive `docker images` from their respective cloud registries.
- Backend-side databases, KV stores, message queues for state.
- Tags written by sockerless that store secrets or state-snapshots beyond identity (`sockerless-managed`, `sockerless-container-id`, `sockerless-name`, `sockerless-pod`, `sockerless:network`, `sockerless:network-id`, `sockerless-instance` — these are identity/discovery only).

---

## Recovery contract

After a backend restart with no in-memory state and no on-disk JSON:

- `docker ps -a` returns the same containers as before.
- `docker network ls` returns the same user-defined networks as before.
- `docker images` returns the same images as before (queried from the cloud registry).
- `docker stop <id>` works on any previously-created container.
- `docker exec <id>` works on any previously-running container (when the backend supports exec).
- `docker pod ps` returns the same pods as before (for backends that map pods to multi-container task defs).

A backend that fails any of these contracts is in violation of Phase 89.

---

## Phase 89 status

| Bug | Status | Notes |
|---|---|---|
| **BUG-723** | fixed | `Store.Images` disk persistence removed. All 6 cloud backends implement `CloudImageLister.ListImages`: ECS + Lambda via ECR `DescribeRepositories`+`DescribeImages`; Cloud Run + GCF via shared `core.OCIListImages` against `<region>-docker.pkg.dev` with `ARAuthProvider` token; ACA + AZF via `core.OCIListImages` against the configured ACR with `ACRAuthProvider` token. `BaseServer.ImageList` merges cache + cloud, deduped by ID. |
| **BUG-724** | partial | `core.CloudPodLister` interface lands; `BaseServer.PodList` merges cache + cloud. ECS `ListPods` groups tasks by `sockerless-pod` tag. Cloud Run + ACA pod listing blocked on Phase 87/88 — multi-container pods need the Jobs→Services/Apps rewrite first. GCF + AZF don't support pods. |
| **BUG-725** | fixed | `resolve*State` cache+cloud-fallback helpers landed across 4 backends (ECS, Lambda, Cloud Run, ACA). Every cloud-state-dependent callsite migrated (Stop, Kill, Remove, Restart, Wait, Logs, ExecCreate, cloudExecStart, etc.). Unit tests cover cache-hit + cache-miss-no-cloud paths. |
| **BUG-726** | fixed | `resolveNetworkState` cache+cloud-fallback helpers landed in ECS, Cloud Run, and ACA. Cloud Map namespaces tagged with `sockerless:network-id` at create time. `cloudServiceRegister` in cloudrun + aca use the fallback. Lambda + GCF + AZF don't have user-defined cloud networks. |

Remaining Phase 89 work:

- Per-backend restart-resilience integration tests against the simulators (ECS, Cloud Run, ACA, Lambda). Each would: spin up sim + backend, create container, kill backend, restart backend, assert every docker call still works. Unit tests already prove the helpers; integration tests prove the SDK interaction. Non-blocking — Phase 89 is functionally complete.

Dependencies:

- BUG-724 full completion requires Phase 87 (cloudrun Jobs → Services with sidecars) + Phase 88 (aca Jobs → Apps with sidecars) so that multi-container pods have a cloud anchor.
