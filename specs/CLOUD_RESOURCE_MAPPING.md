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
6. **Each cloud service has exactly one supported generation** — whichever is current. No backend keeps fallback paths to older generations (e.g. no GCF v1 alongside v2; no Azure Functions Consumption-v3 runtime alongside Flex Consumption). If the operator points sockerless at an older generation, `Config.Validate()` fails fast with an "upgrade to the supported generation" error; there is no silent downgrade.
7. **No fakes, no fallbacks, no placeholders.** Workarounds, silent substitutions, placeholder fields, synthetic-metadata backstops — all are bugs and land in [BUGS.md](../BUGS.md) under "Open" until a real fix ships.

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
| Exec instance | Reverse-agent overlay (`sockerless-lambda-bootstrap` dials back during `Invoke`); see [Exec](#exec). | (transient agent session) | — |

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
| Exec instance | Reverse-agent overlay (no native exec on Cloud Run Jobs/Services); see [Exec](#exec). | (transient agent session) | — |

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
| Exec instance | ACA console exec API (`Microsoft.App/jobs/{job}/executions/{exec}/exec` via `aca/exec_cloud.go`), with the reverse-agent preferred when present; see [Exec](#exec). | (transient management-API or agent session) | — |

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
| Exec instance | Reverse-agent overlay (no native exec on Cloud Functions); see [Exec](#exec). | (transient agent session) | — |

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
| Exec instance | Reverse-agent overlay (Kudu console / SSH not implemented); see [Exec](#exec). | (transient agent session) | — |

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

## Per-invocation container state

For long-running containers (ECS tasks, Cloud Run Jobs, ACA Jobs, Cloud Run Services, ACA ContainerApps) the cloud resource IS the container — `docker inspect` / `docker wait` / `docker ps` read directly from `DescribeTasks` / `Execution.status` / `Revision.status`. For **FaaS backends** the cloud function is long-lived but *invocations* are ephemeral, so `docker wait` needs a per-invocation signal, not a function-level one. Each backend has exactly one cloud-native signal for invocation completion + exit code; see [Phase 95 in PLAN.md](../PLAN.md) for the implementation plan.

| Backend | Container-lifecycle resource | Completion signal | Exit-code source |
|---------|------------------------------|-------------------|------------------|
| `ecs` | `Task` | `Task.LastStatus=STOPPED` | `Task.Containers[].ExitCode` (already wired via BUG-738) |
| `lambda` | Function Invocation | `lambda:Invoke` response OR CloudWatch Logs `END RequestId <id>` | `Invoke.FunctionError` ("Unhandled"/"Handled") → 1; 2xx + no error → 0; `REPORT … Status: timeout` → 124 |
| `cloudrun` (Jobs) | `Execution` | `Execution.conditions[Type=Completed].status=True`; `completionTime` set | `failedCount > 0` → 1; `succeededCount > 0` → 0 |
| `cloudrun` (Services, UseService=on) | `Revision` | `Revision.conditions` or request completion from service URL | HTTP 2xx → 0; 4xx/5xx → 1; 408 → 124 |
| `cloudrun-functions` (gcf) | HTTP invocation to `ServiceConfig.Uri` | HTTP response status | 2xx → 0; 4xx/5xx → 1; 408 → 124 |
| `aca` (Jobs) | `JobExecution` | `JobExecution.properties.status in {Succeeded, Failed, Stopped}`; `endTime` set | `status=Succeeded` → 0; `Failed`/`Stopped` → 1/137 |
| `aca` (ContainerApps, UseApp=on) | `Revision` + container app logs | Request completion from container-app ingress | HTTP status mapping same as GCF/AZF |
| `azure-functions` (azf) | HTTP invocation to Function App default host | HTTP response status | Same HTTP mapping as GCF |
| `docker` | Local container | Daemon events | Daemon-reported |

Rules:
1. Backends never fabricate an exit code. If the signal isn't yet available (invocation still running / execution not yet `Succeeded`), `docker wait` blocks on the wait channel; it does not return 0 prematurely.
2. Backends never conflate *function state* with *invocation state*. An `ACTIVE` Lambda / GCF / AZF function with no in-flight invocation still maps to `State.Status=exited` *for a specific container* once that container's invocation is known to have finished — the cloud function resource itself remains `Active` and reusable.
3. Invocation results that can't be recovered from the cloud (e.g. in-memory `InvocationResults` map lost on backend restart) fall back to the conservative "container is running if its function still exists" view. That's the same invariant `resolveTaskState` already applies for restart-safe state recovery.

## Volume provisioning per backend

`docker volume create`, `docker volume rm`, `docker volume ls`, `docker volume inspect`, `docker volume prune`, and `-v volname:/mnt` / bind-mount flags currently return `NotImplemented` on every cloud backend (BUG-731): the earlier in-memory metadata store silently accepted volumes but never bound them to any cloud-side storage. The table below is the design for real provisioning, phase by phase.

| Backend | Cloud resource | Lifecycle mapping | IAM / API actions needed | Simulator work |
|---------|----------------|--------------------|--------------------------|----------------|
| `ecs` | **EFS** file system + per-AZ mount targets + per-volume access point. Access point maps the volume name to a subdirectory owned by a fixed UID/GID so tasks can't trample each other. | `VolumeCreate` → ensure one EFS per backend (reuse by tag `sockerless-managed=true`), then `CreateAccessPoint` per volume, store volume-name → access-point-id in tags. `VolumeRemove` → `DeleteAccessPoint` (EFS stays, holding other volumes). Bind / named mounts → inject `EFSVolumeConfiguration{FileSystemId, AccessPointId, TransitEncryption=ENABLED}` into the task-def's `Volumes` array + `MountPoints` in the container def. | `elasticfilesystem:CreateFileSystem`, `DescribeFileSystems`, `CreateMountTarget`, `DescribeMountTargets`, `CreateAccessPoint`, `DescribeAccessPoints`, `DeleteAccessPoint`, `TagResource`, `PutFileSystemPolicy`. Task execution role needs `elasticfilesystem:ClientMount/ClientWrite/ClientRootAccess`. | `simulators/aws/efs.go` — real EFS-like slice. Store file systems + mount targets + access points; back access points with per-volume subdirectories on a host-side Docker volume so the per-task Docker container can mount the same path and see the same files. |
| `lambda` | None (permanent). Lambda containers only have a 512 MB–10 GB ephemeral `/tmp`; there is no per-invocation cross-container storage docker volumes can usefully bind. | `VolumeCreate`/etc. stay `NotImplemented`. `ContainerCreate` with `-v volname:/x` should reject with a clear error pointing at S3 / DynamoDB / EFS-via-Lambda-VPC for durable state. | — | — |
| `cloudrun` | **GCS bucket** per volume (simplest first pass), mounted via Cloud Run Service's native `Volume{Gcs{Bucket}}` in the revision template. Optional upgrade to **Filestore** later for POSIX semantics if `O_APPEND` / file locking is needed. | `VolumeCreate` → `storage.Buckets.Insert` with naming `sockerless-volume-<id>`, label `sockerless-managed=true`. `VolumeRemove` → `DeleteBucket` (requires empty; force=true uses `DeleteObjects` first). Bind / named mount → inject `RevisionTemplate.Volumes[].Gcs{Bucket}` + `Container.VolumeMounts` in the service spec. | `storage.buckets.create/delete/list`, `storage.objects.*` for prune/delete. Cloud Run service account needs `roles/storage.objectAdmin` on buckets it mounts. | `simulators/gcp/storage.go` already has GCS slice; extend with `Volume{Gcs}` honouring on the Cloud Run simulator path so the backing Docker container gets a real bind mount against the sim's bucket directory. |
| `aca` | **Azure Files share** in a sockerless-owned storage account, linked into the managed environment as an `ManagedEnvironments/storages` resource, then referenced from `ContainerApp.Properties.Template.Volumes[]` + `Container.VolumeMounts`. | `VolumeCreate` → (ensure storage account exists) + `FileShares.Create` + `ManagedEnvironmentsStorages.CreateOrUpdate` so the env knows about the share. `VolumeRemove` → `FileShares.Delete` + `ManagedEnvironmentsStorages.Delete`. Bind / named mount → inject `ContainerAppProperties.Template.Volumes` + `Container.VolumeMounts` into the app spec. | `Microsoft.Storage/storageAccounts/read,write,listKeys`, `Microsoft.Storage/storageAccounts/fileServices/shares/read,write,delete`, `Microsoft.App/managedEnvironments/storages/read,write,delete`. | `simulators/azure/storage.go` gains `fileServices/shares` sub-resource CRUD (the storage slice today is blob-only). `simulators/azure/containerappsenv.go` gains `storages` sub-resource. The sim's ACA container bind-mounts a host-side directory per share so containers see real files. |
| `cloudrun-functions` (gcf) | GCP Cloud Functions (targeting the current v2 API only; v1 not supported). v2 is Cloud Run Services under the hood. | Shared helper with `cloudrun` — same GCS-bucket-mount lifecycle. | Same as cloudrun. | Shares `simulators/gcp/` GCS extensions. |
| `azure-functions` (azf) | Azure Functions on the current Flex Consumption / Premium plan with BYOS Azure Files mounts. | Provision Azure Files share (shared helper with ACA), then attach to the Function App via `sites/<fn>/config/azurestorageaccounts`. | Same Azure Files permissions as ACA + `Microsoft.Web/sites/config/write`. | Shares `simulators/azure/` Azure Files extensions. |
| `docker` | Real Docker volumes via the local daemon. | Already implemented — passthrough to `docker volume *` on the host daemon. | — | — |

Each cloud's volume work is filed as its own phase in [PLAN.md](../PLAN.md) so real provisioning lands as discrete, reviewable units.

---

## Docker / Podman API coverage matrix

Full list of every `api.Backend` method sockerless implements, per-backend status, and simulator coverage. Status legend:

- **✓** — fully implemented against real cloud resources (or the docker daemon for the docker backend).
- **⚠** — partially implemented; see notes row below the table.
- **✗** — returns `api.NotImplementedError` with a clear message pointing at the cloud's native primitive. No silent acceptance.
- **—** — not applicable: the cloud has no equivalent primitive and the API surface doesn't meaningfully extend to it.
- **S✗** — **Simulator gap**: backend is implemented but the simulator doesn't yet emulate the underlying cloud slice, so the call only works against real cloud. Tracked as separate bugs / phases.

### Container lifecycle + runtime ops

| Method | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|--------|:------:|:---:|:------:|:--------:|:---:|:---:|:---:|
| ContainerCreate | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerStart | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerStop | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerKill | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerRestart | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerRemove | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerWait | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerInspect | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerList | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerLogs | ✓ | ✓ (CloudWatch) | ✓ (CloudWatch) | ✓ (Cloud Logging) | ✓ | ✓ (Log Analytics) | ✓ |
| ContainerStats | ✓ | ⚠ CloudWatch — zero until metrics arrive | ⚠ CloudWatch | ⚠ Cloud Monitoring | ⚠ | ⚠ Log Analytics | ⚠ |
| ContainerTop | ✓ | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ | ⚠ agent only | ⚠ |
| ContainerRename | ✓ | ⚠ local-name-only | ⚠ local-name-only | ⚠ local-name-only | ⚠ | ⚠ local-name-only | ⚠ |
| ContainerUpdate | ✓ | ⚠ limited — CPU/mem only via task-def rev | ⚠ | ⚠ via new revision | ⚠ | ⚠ via new revision | ⚠ |
| ContainerResize | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| ContainerPause | ✓ | ✗ Fargate no-pause | ✗ | ✗ Cloud Run no-pause | ✗ | ✗ ACA no-pause | ✗ |
| ContainerUnpause | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| ContainerCommit | ✓ | ✗ Fargate no-snapshot | ✗ | ✗ | ✗ | ✗ | ✗ |
| ContainerExport | ✓ | ✗ Fargate no-fs | ✗ | ✗ | ✗ | ✗ | ✗ |
| ContainerChanges | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| ContainerStatPath | ✓ | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ | ⚠ agent only | ⚠ |
| ContainerGetArchive | ✓ | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ | ⚠ agent only | ⚠ |
| ContainerPutArchive | ✓ | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ | ⚠ agent only | ⚠ |
| ContainerPrune | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerAttach | ✓ | ✓ (CloudWatch stream) | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only / ACA console | ⚠ agent only |

Notes:

- **ContainerStats ⚠** — cloud providers only surface aggregated per-task metrics with ~60s lag; no block-I/O or network-byte counters equivalent to docker's cgroup stats. Sockerless reports CPU-ns + mem-bytes + PIDs=0 (BUG-733) when nothing's there yet. A future phase may add cloud-native stats endpoints to each simulator for parity with docker's streaming stats.
- **ContainerTop / Stat / GetArchive / PutArchive / Attach ⚠ agent only** — possible only when the sockerless agent is bundled into the container image (Lambda's agent-as-handler pattern; CR/ACA/GCF/AZF use the same overlay). Without a registered reverse-agent session, every backend returns a `NotImplementedError` that names the missing prerequisite (`SOCKERLESS_CALLBACK_URL`) — never a silently-empty stream. ACA additionally falls back to the cloud-native console exec API for ExecStart/Attach when no agent is present. See [Exec](#exec) below for the full resolution table.
- **ContainerRename ⚠** — cloud resources (ECS task, Cloud Run Job, ACA app) have immutable names derived from the container ID; the docker API's "rename" updates local metadata only (`sockerless-name` tag does stay updated via re-tag). `docker inspect` shows the new name but the cloud resource name doesn't change.
- **ContainerUpdate ⚠** — resource-limit updates go through a new task-def revision / service revision / app revision. Docker's live `update --cpus --memory` semantics can't apply to already-running cloud tasks; the next start picks up the new limits.
- **ContainerResize ✗** — TTY resize events (`SIGWINCH`) don't propagate through Cloud Run / Fargate / ACA to the container. Future phase may add a sim-side pipe for local testing.

### Exec

| Method | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|--------|:------:|:---:|:------:|:--------:|:---:|:---:|:---:|
| ExecCreate | ✓ | ✓ (SSM) | ✓ (agent overlay) | ✓ (agent overlay) | ✓ (agent overlay) | ✓ ACA console / agent | ✓ (agent overlay) |
| ExecStart | ✓ | ✓ (SSM AgentMessage) | ✓ agent | ✓ agent | ✓ agent | ✓ ACA console / agent | ✓ agent |
| ExecInspect | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ExecResize | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |

Notes:

- **Resolution policy** (applies to ExecStart and ContainerAttach across every cloud backend): each call resolves the container, then dispatches as follows:
  1. If a reverse-agent session is registered for the container → `BaseServer.{ExecStart,ContainerAttach}` runs through `Drivers.{Exec,Stream}` (= `core.ReverseAgent{Exec,Stream}Driver`), which bridges over the WebSocket.
  2. Else, if the backend has a cloud-native exec surface (only ACA today via `cloudExecStart` against the ACA management API; ECS via SSM) → use that.
  3. Else → return `NotImplementedError` naming the missing prerequisite (`SOCKERLESS_CALLBACK_URL` for the agent path) — never a silently-empty stream or exit-126.
- **ECS**: real `ExecuteCommand` via SSM Session Manager. Requires task IAM role grants for `ssmmessages:*` (BUG-720) + `EnableExecuteCommand: true` at RunTask (BUG-719) + full SSM AgentMessage decoder in the backend (BUG-717).
- **Lambda**: agent-as-handler. `sockerless-lambda-bootstrap` dials back to `/v1/lambda/reverse`; exec tunnels through.
- **Cloud Run / GCF / AZF**: no native exec surface. Reverse-agent overlay is the only path; backends now route through `BaseServer.ExecStart` after verifying the session exists.
- **ACA**: ACA has a native console exec API (`Microsoft.App/jobs/{job}/executions/{exec}/exec`) wired via `aca/exec_cloud.go::cloudExecStart`. The backend prefers the reverse-agent when present and falls back to cloudExecStart otherwise.

### Images

| Method | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|--------|:------:|:---:|:------:|:--------:|:---:|:---:|:---:|
| ImagePull | ✓ | ✓ (ECR pull-through) | ✓ (ECR pull-through) | ✓ (AR) | ✓ | ✓ (ACR cache) | ✓ |
| ImagePush | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ImageList | ✓ | ✓ ECR | ✓ ECR | ✓ AR | ✓ | ✓ ACR | ✓ |
| ImageInspect | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ImageRemove | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ImageTag | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ImageHistory | ✓ | ✓ (manifest) | ✓ | ✓ | ✓ | ✓ | ✓ |
| ImageBuild | ✓ | ✓ CodeBuild / Cloud Build / ACR tasks? | ⚠ | ⚠ Cloud Build | ⚠ | ⚠ ACR build | ⚠ |
| ImagePush | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ImageLoad | ✓ | ⚠ tarball → ECR push | ⚠ | ⚠ tarball → AR push | ⚠ | ⚠ tarball → ACR push | ⚠ |
| ImageSave | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| ImageSearch | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| ImagePrune | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

Notes:

- **ImageBuild**: the backend's ImageManager ships to the cloud's native build service — AWS CodeBuild / GCP Cloud Build / Azure ACR Tasks. The simulator serves each build API (GCP Cloud Build is implemented; AWS CodeBuild and ACR Tasks have slices). Still **⚠** per-backend because not every build option (cache-from, secrets mount, multi-arch) round-trips faithfully yet.
- **ImageSave ✗** — cloud registries don't serve a full tar. Would require downloading manifest + all blobs and retaring. Possible but nobody's asked for it; marked NotImplemented clean.
- **ImageSearch ✗** — cloud registries don't expose full-text search over public images. Docker Hub search via HTTPS still works but isn't what the docker search endpoint expects. Marked NotImplemented.

### Networks

| Method | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|--------|:------:|:---:|:------:|:--------:|:---:|:---:|:---:|
| NetworkCreate | ✓ | ✓ SG + Cloud Map | — | ✓ Cloud DNS zone | — | ✓ Private DNS + NSG | — |
| NetworkRemove | ✓ | ✓ | — | ✓ | — | ✓ | — |
| NetworkInspect | ✓ | ✓ | — | ✓ | — | ✓ | — |
| NetworkList | ✓ | ✓ | — | ✓ | — | ✓ | — |
| NetworkConnect | ✓ | ✓ Cloud Map A-record | — | ✓ CNAME when UseService | — | ✓ CNAME when UseApp | — |
| NetworkDisconnect | ✓ | ✓ | — | ✓ | — | ✓ | — |
| NetworkPrune | ✓ | ✓ | — | ✓ | — | ✓ | — |

Notes:

- **Lambda / GCF / AZF (— columns)** — these three are invocation-scoped runtimes; there's no VPC-like peer-to-peer primitive they address with. "Networks" in the docker API would map to... nothing meaningful. The backends accept network names as local bookkeeping (so `-v net=foo` doesn't error) but nothing cloud-side ties to them.

### Volumes

| Method | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|--------|:------:|:---:|:------:|:--------:|:---:|:---:|:---:|
| VolumeCreate | ✓ | ✗ (Phase 91) | ✗ (no equivalent) | ✗ (Phase 92) | ✗ (Phase 94) | ✗ (Phase 93) | ✗ (Phase 94) |
| VolumeInspect | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| VolumeList | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| VolumeRemove | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |
| VolumePrune | ✓ | ✗ | ✗ | ✗ | ✗ | ✗ | ✗ |

See "Volume provisioning per backend" section above. Phases 91-94 land real provisioning.

### Pods (libpod)

| Method | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|--------|:------:|:---:|:------:|:--------:|:---:|:---:|:---:|
| PodCreate | ✓ | ✓ multi-container task-def | ✗ | ✓ multi-container Service revision | ✗ | ✓ multi-container App | ✗ |
| PodStart | ✓ | ✓ | ✗ | ✓ | ✗ | ✓ | ✗ |
| PodStop | ✓ | ✓ | ✗ | ✓ | ✗ | ✓ | ✗ |
| PodKill | ✓ | ✓ | ✗ | ✓ | ✗ | ✓ | ✗ |
| PodRemove | ✓ | ✓ | ✗ | ✓ | ✗ | ✓ | ✗ |
| PodList | ✓ | ✓ (group by `sockerless-pod` tag) | ✗ | ✓ (grouping across Jobs + Services) | ✗ | ✓ (grouping across Jobs + Apps) | ✗ |
| PodInspect | ✓ | ✓ | ✗ | ✓ | ✗ | ✓ | ✗ |
| PodExists | ✓ | ✓ | ✗ | ✓ | ✗ | ✓ | ✗ |

Notes:

- **Lambda / GCF / AZF ✗** — function-as-a-service platforms have no multi-container-per-invocation primitive. Pods would need an external coordinator (Step Functions / Cloud Workflows / Durable Functions) which is out of scope.

### System + misc

| Method | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|--------|:------:|:---:|:------:|:--------:|:---:|:---:|:---:|
| Info | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| SystemDf | ✓ | ⚠ containers only; registry size N/A | ⚠ | ⚠ | ⚠ | ⚠ | ⚠ |
| SystemEvents | ✓ | ⚠ local events only | ⚠ | ⚠ | ⚠ | ⚠ | ⚠ |
| AuthLogin | ✓ | ✓ (ECR token) | ✓ | ✓ (GAR token) | ✓ | ✓ (ACR token) | ✓ |

Notes:

- **SystemDf ⚠** — `docker system df` shows disk usage by images / containers / volumes / build-cache. Sockerless reports container counts correctly; cloud registries don't cleanly expose aggregate size-on-disk per image without fetching every manifest. Marked partial.
- **SystemEvents ⚠** — sockerless emits its own events (container create / start / stop / die / destroy / network create / etc.) on all backends. What's not emitted: cloud-side events originating outside sockerless (a manual `aws ecs stop-task`, `gcloud run services update` etc.). Future phase could poll each cloud's audit log and re-emit. Not currently prioritised.

---

## Simulator coverage gaps (S✗)

Below is the current "implementing it against real cloud would work, but there's no sim-side emulation so local/CI testing falls back to real cloud":

| Gap | Backend(s) | Blocks |
|-----|------------|--------|
| Azure Files share slice (`fileServices/shares`) in `simulators/azure/storage.go` | aca, azf | Phase 93, Phase 94 real volumes |
| Managed-environment `storages` sub-resource in `simulators/azure/containerappsenv.go` | aca | Phase 93 real volumes |
| GCS bucket-mount honouring in `simulators/gcp/cloudrun.go` spec-builder path | cloudrun, gcf | Phase 92 real volumes |
| EFS `AccessPoint` CRUD in `simulators/aws/` (new `efs.go`) | ecs | Phase 91 real volumes |
| ACA console exec proto in `simulators/azure/containerapps.go` | aca | ExecCreate/Start/Inspect |
| Lambda reverse-agent (already wired) — no sim gap, works end-to-end | lambda | — |
| Cloud Run exec: no upstream API exists; Lambda-style agent overlay would need to be ported | cloudrun, gcf | Exec family |
| AWS Session Manager agent-side ack validation | ecs | BUG-729 |

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
