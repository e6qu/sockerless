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

**State derivation:**

- `docker ps -a` → `ListTasks(cluster, RUNNING)` + `ListTasks(cluster, STOPPED)` + `DescribeTasks(arns, Include=TAGS)`, filter to `sockerless-managed=true`, project to `api.Container` via `taskToContainer` (already exists).
- `docker pod ps` → same as above, group by `sockerless-pod` tag, emit one `PodListEntry` per group.
- `docker network ls` → `DescribeSecurityGroups(Filters=[tag:sockerless:network=*])` + `ListNamespaces(filter NamespaceType=DNS_PRIVATE)` filter to `sockerless-managed`. Each (SG, namespace) pair backed by name/id tag = one network.
- `docker images` → `DescribeRepositories` (filter to `sockerless-managed` if we tag repos that way) + `DescribeImages` for each, map to `api.Image`.
- `docker exec` → resolve `task ARN` from container-id tag (BUG-722's `resolveTaskARN`); call `ExecuteCommand`.

**Currently-violating in-memory state to remove (BUG-725):**

- `s.ECS *StateStore[ECSState]` (TaskARN, ClusterARN, SecurityGroupIDs, ServiceID per container) — must become a cache; lookups fall back to cloud.
- `s.NetworkState *StateStore[NetworkState]` (SecurityGroupID, NamespaceID per docker network) — same.
- `s.VolumeState *StateStore[VolumeState]` — same.

### AWS Lambda (backend `lambda`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Lambda function | `function ARN`, `containerID` | function tags: `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | Multi-container pod is **not supported** by Lambda — one function = one container. Pods would require a coordinator (e.g. Step Functions); not in scope. | — | — |
| Image | ECR repository / image | `<account>.dkr.ecr.<region>.amazonaws.com/<repo>:<tag>` | (registry-managed) |
| Network | **Native cross-container DNS is not addressable per-execution.** Lambda VPC config only routes egress; peer-Lambda discovery requires Service Discovery + a separate fronting service. Treat docker networks as bookkeeping only. | (no cloud anchor) | (Phase 89 follow-up: file as known limitation) |
| Volume | Lambda layers (read-only) or `/tmp` (per-invocation, ephemeral). Bind mounts and named volumes outside `/tmp` are not supported. | — | — |
| Exec instance | **Implemented via the agent overlay**: `cloudExecStart` dials the reverse-agent WebSocket (registered by `sockerless-lambda-bootstrap` during `Invoke`) and tunnels the command. | (transient agent session) | — |

**State derivation:**

- `docker ps -a` → `ListFunctions` + `ListTags` per function ARN (filter `sockerless-managed=true`), project to `api.Container`.
- `docker images` → ECR `DescribeImages` (same as ECS).
- `docker exec` → look up function tag → invoke agent → exec via overlay.

**Currently-violating in-memory state to remove (BUG-725):**

- `s.Lambda *StateStore[LambdaState]` (FunctionARN, log group, agent token per container) — cache only.

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

**Currently-violating in-memory state (BUG-725 cross-cloud sibling):**

- `s.CloudRun *StateStore[CloudRunState]` (JobName, ExecutionName, etc. per container) — must become a cache.
- `s.NetworkState *StateStore[NetworkState]` (ManagedZoneName per docker network) — must become a cache; lookups fall back to `ManagedZones.Get(<sanitized>)`.

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

**Currently-violating in-memory state (BUG-725 / BUG-726 cross-cloud sibling):**

- `s.ACA *StateStore[ACAState]` (JobName, AppName, ExecutionName per container) — must become a cache.
- `s.NetworkState *StateStore[NetworkState]` (DNSZoneName, NSGName per docker network) — must become a cache; lookups fall back to `PrivateZonesClient.Get` + tag-filter SG list.

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
- `docker images` → Artifact Registry `Images.List` filtered by repo path.
- `docker logs` → Cloud Logging `LogAdmin.Entries(filter='resource.type="cloud_function" labels.function_name="<name>"')`.

**Currently-violating in-memory state (BUG-725 cross-cloud sibling):**

- `s.GCF *StateStore[GCFState]` (FunctionName per container) — must become a cache; lookups fall back to Cloud Functions API + label filter.

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
- `docker images` → ACR `RegistryClient.NewListImportImagesPager` for the configured ACR.
- `docker logs` → App Service container logs via `WebApps.GetContainerLogsZip` or `LogAnalytics` queries on the workspace linked to the App.

**Currently-violating in-memory state (BUG-725 cross-cloud sibling):**

- `s.AZF *StateStore[AZFState]` (FunctionAppName per container) — must become a cache; lookups fall back to `WebApps.NewListByResourceGroupPager` + tag filter.

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

- `~/.sockerless/state/images.json` (BUG-723 — Store.Images persistence). **Removed** in Phase 89 step 1. Per-backend cloud-derived `docker images` is the in-progress step 2.
- Backend-side databases, KV stores, message queues for state.
- Tags written by sockerless that store secrets or state-snapshots beyond identity (`sockerless-managed`, `sockerless-container-id`, `sockerless-name`, `sockerless-pod`, `sockerless:network`, `sockerless-instance` — these are identity/discovery only).

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

## Phase 89 work breakdown

This doc grounds the following bugs:

- **BUG-723** Remove `Store.Images` disk persistence; query the cloud registry on `docker images` / `docker pull` / `docker push`.
- **BUG-724** Implement `PodList` / `PodInspect` per backend by deriving from cloud actuals (multi-container task / app), not from `Store.Pods`.
- **BUG-725** Replace ECS `s.ECS` and `s.NetworkState` and `s.VolumeState` with cache-on-demand wrappers; `resolveTaskARN` (BUG-722) becomes the canonical pattern, generalized.
- **BUG-726** Same as 725 for cloudrun / aca / lambda / gcf / azf where applicable.

Implementation order recommended:

1. Land this doc (this commit).
2. **Audit pass**: in each backend's main file, mark every `s.<StateStore>.Get(id)` callsite with a comment indicating whether it's a cache or a state-of-truth use. State-of-truth uses become "look up from cloud, write to cache, return."
3. ECS first (BUG-725 reference implementation): generalize `resolveTaskARN` so every backend method that needs `TaskARN` falls back to cloud lookup.
4. Cloudrun + ACA + Lambda + GCF + AZF: same pattern.
5. `Store.Images` removal: replace `Store.Images.List()` with a `cloudImageList(ctx)` method per backend that queries ECR / Artifact Registry / ACR. Delete `PersistImages` / `RestoreImages` / `images.json` plumbing.
6. Pod derivation: `PodList` queries cloud, groups by `sockerless-pod` tag.
7. Tests: each backend gets a "stateless restart" integration test that creates resources, restarts the backend (drops in-memory state), and asserts every relevant docker call still works.
