# Cloud Resource Mapping

Authoritative mapping between Docker / Podman concepts and the cloud resources that back them in each Sockerless backend. The corollary: **state derives from cloud actuals**. After a backend restart, every list / inspect / stop / exec call must reproduce the same answer by querying the cloud APIs of its configured environment — no in-memory map, on-disk JSON, S3 object, or DynamoDB row may be consulted as the source of truth.

This document is the source of truth for the stateless-backend invariant.

> **Companion specs:**
> - [BACKEND_STATE.md](BACKEND_STATE.md) — the stateless principle, identity model, tagging conventions
> - [SIMULATOR_RECOVERY.md](SIMULATOR_RECOVERY.md) — recovery on restart, PID re-attachment, simulator-side tag handling
> - [BACKENDS.md](BACKENDS.md) — per-backend implementation overview
>
> Per-backend `docker_api_mapping.md` files (under `backends/<name>/docs/`) describe the call-by-call translation; this file describes the durable resource mapping.

---

## Universal rules

1. **Cloud resources are tagged at creation** with the managed-marker tag plus identity tags so they can be enumerated and reattributed after restart. Tag-key spelling differs by cloud's charset rules:
   - **AWS** (ECS, Lambda) and **Azure** (ACA, AZF): `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>`. Hyphens are universally accepted.
   - **GCP** (Cloud Run, GCF): `sockerless_managed=true`, `sockerless_container_id=<id>`, `sockerless_name=<name>`. GCP labels use underscores by convention.
   The semantics are identical; only the punctuation differs. When this doc later writes "the `sockerless-managed` tag" it means whichever spelling the cloud uses.
2. **Every list / inspect call queries the cloud first.** In-memory caches are allowed but must be invalidatable, must be rebuilt on miss from cloud actuals, and must never be the source of truth.
3. **Persistent on-disk state is forbidden.** No `~/.sockerless/state/*.json`, no S3 buckets, no DynamoDB. The only file paths backends touch on disk are: configuration (read-only), credentials (read-only), and CLI run-state (PID files etc.).
4. **State buckets / lock tables for terraform are infrastructure, not sockerless state** — they hold Terraform's state for the operator-managed infra and have nothing to do with backend operation.
5. **A "container" in the docker API is whatever the cloud calls a single container of work** — task, function invocation, app revision, job execution. A "pod" in the libpod API is a *group* of containers, which in clouds without first-class pods is a multi-container task / multi-container app.
6. **Each cloud service has exactly one supported generation** — whichever is current. No backend keeps fallback paths to older generations (e.g. no GCF v1 alongside v2; no Azure Functions Consumption-v3 runtime alongside Flex Consumption). If the operator points sockerless at an older generation, `Config.Validate()` fails fast with an "upgrade to the supported generation" error; there is no silent downgrade.
7. **No fakes, no fallbacks, no placeholders.** Workarounds, silent substitutions, placeholder fields, synthetic-metadata backstops — all are bugs and land in [BUGS.md](../BUGS.md) under "Open" until a real fix ships.
8. **FaaS backends run user-supplied container images, never the native runtime.** Lambda, GCF (Cloud Run Functions gen2), and AZF (Azure Functions) deploy OCI images chosen by the operator. Sockerless never targets the platforms' "function-as-code" runtime contracts (Node/Python/Go handlers in a managed sandbox). Container deployment is what lets sockerless put its bootstrap at the entrypoint, which is the prerequisite for the reverse-agent, agent-as-handler, and overlay-rootfs (opt-in via `SOCKERLESS_OVERLAY_ROOTFS=1`) patterns. ACA and Cloud Run are native container services, so this distinction is automatic — every deployment is a container.

9. **Backend ↔ host primitive must match (CRITICAL).** When a sockerless backend is deployed *as part of a workload running on a cloud* (e.g. baked into a CI runner image), the backend must match that cloud's primitive: ECS backend in ECS, Lambda backend in Lambda, Cloud Run backend in Cloud Run, CRF in CRF, ACA in ACA, AZF in AZF. Cross-pollination ("bake the ECS backend into a Lambda image to dispatch sub-tasks via Fargate and avoid Lambda-in-Lambda recursion") is a class of architectural error tracked at top of [BUGS.md](../BUGS.md). Each cloud's own dispatch primitives are the answer for sub-task workloads on that cloud, even when 15-min caps or concurrency limits make it harder. The **runner-on-FaaS dispatch table** below gives the per-cloud primitive used for `container:` sub-tasks:

   | Backend | Primitive for `container:` sub-task | IAM (in addition to base FaaS perms) |
   |---|---|---|
   | `lambda` | `lambda.CreateFunction` (image-mode container) per sub-task → `lambda.Invoke`. Sub-task functions share the runner's workspace EFS access point via `FileSystemConfig`. After invoke + completion, `lambda.DeleteFunction`. | `lambda:CreateFunction/Invoke/Delete/Get/UpdateConfiguration/Tag/ListFunctions`, `iam:PassRole` for sub-task execution role. |
   | `cloudrun-functions` (gcf) | HTTP invoke (`https://<service-uri>/`) of a sockerless-overlay-imaged Function created via `functions.CreateFunction(stub-buildpacks-source)` + post-create `run.Services.UpdateService(image=overlay)`. See [§ GCP Cloud Run Functions](#gcp-cloud-run-functions-backend-cloudrun-functions--gcf). Function reuse pool keyed on overlay-content-hash so amortized startup matches Cloud Run Functions normal cold-start. Workspace shared via GCS bucket pre-mounted by the bootstrap. | `cloudfunctions.functions.create/get/list/update/delete`, `run.services.get/update`, `cloudbuild.builds.create`, `artifactregistry.repositories.uploadArtifacts`, `iam.serviceAccounts.actAs`. |
   | `azure-functions` (azf) | Function-app deployment + HTTP trigger invoke; sub-task workspace mounted as Azure Files share via the function app's site config. | `Microsoft.Web/sites/{create,invoke,delete}`, managed-identity `actAs`. |
   | `ecs` | `ecs.RunTask` (Fargate) per sub-task; not relevant *inside* an ECS workload because ECS tasks run a long-lived sockerless that handles repeated `RunTask` directly. | (default ECS task role.) |
   | `cloudrun` (services) | `run.Services.CreateRevision` against a per-sub-task Service; long-lived for the duration of the parent workload. | `run.services.create/get/delete`. |
   | `aca` | `containerapps.CreateOrUpdate` against a per-sub-task App. | `Microsoft.App/containerApps/write/delete`. |

---

## Mapping per cloud

### AWS ECS (backend `ecs`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | ECS task (Fargate) | `task ARN` (cloud), `containerID` (Docker) | `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>`, `sockerless-instance=<backend-instance-id>`. Per-container ops also stamp transient tags: `sockerless-network=<network-name>` (membership), `sockerless-restart-count=<n>` (used by `docker inspect.RestartCount` to recover the count after restart), `sockerless-kill-signal=<sig>` (used by exit-code mapping to recover the signal that terminated the task), `sockerless-removed=true` (registry-side cleanup marker), `sockerless-labels-b64[-<n>]=<base64-of-json>` (chunked Docker labels for >256 chars per tag value). |
| Pod (libpod) | ECS task with multi-container task definition | `task ARN`, `pod name` | + `sockerless-pod=<name>` |
| Image | ECR repository / image | `<account>.dkr.ecr.<region>.amazonaws.com/<repo>:<tag>` | (registry-managed) |
| Network (user-defined) | EC2 security group + Cloud Map private DNS namespace | `sg-…` + `ns-…` | Resource-level: `sockerless:network=<name>`, `sockerless:network-id=<id>` (colon-form is ECS-only — EC2/SD tags accept colons). |
| Volume (named) | EFS access point on a sockerless-managed EFS filesystem; injected into the task-def's `Volumes` array as `EFSVolumeConfiguration{FileSystemId, AccessPointId, TransitEncryption=ENABLED}` plus `MountPoints` in the container def. See `aws-common/volumes.go::EFSManager`. | EFS filesystem id + access-point id | EFS resource tags: `sockerless-managed=true`, `sockerless-volume-name=<name>` |
| Exec instance | ECS `ExecuteCommand` session | (transient SSM session) | (transient — no recovery needed) |

**State derivation:**

- `docker ps -a` → `ListTasks` RUNNING+STOPPED + `DescribeTasks(Include=TAGS)` filtered to `sockerless-managed=true`, projected via `taskToContainer`.
- `docker pod ps` → same task query grouped by `sockerless-pod` tag; `ecsCloudState.ListPods`.
- `docker network ls` → `DescribeSecurityGroups(tag:sockerless:network-id=<id>)` + `ListNamespaces(DNS_PRIVATE) → ListTagsForResource(tag:sockerless:network-id=<id>)`.
- `docker images` → `DescribeRepositories` + `DescribeImages` → `ImageSummary` with ECR RepoTags/RepoDigests; `ecsCloudState.ListImages`.
- `docker exec` → `ecsCloudState.resolveTaskARN(containerID)` via tag filter, then `ExecuteCommand`.
- `docker stop/kill/rm/restart/wait/logs/ExecCreate` → all go through `Server.resolveTaskState(ctx, containerID)` cache+cloud-fallback helper.

**In-memory state as a cache:**

- `s.ECS *StateStore[ECSState]` — transient cache; every cloud-identity callsite uses `resolveTaskState` which repopulates on miss.
- `s.NetworkState *StateStore[NetworkState]` — transient cache; populated on create, recovered via `resolveNetworkState` on miss.
- `s.VolumeState *StateStore[VolumeState]` — transient cache (volume state is simpler; follows same pattern).

### AWS Lambda (backend `lambda`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Lambda function | `function ARN`, `containerID` | function tags: `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | **Supported via supervisor-in-overlay** (degraded namespace isolation — see "Podman pods on FaaS backends" below). Lambda's image-mode container backs the function with a single Linux container; the overlay bakes all pod containers' rootfs and `sockerless-lambda-bootstrap` (supervisor) into one image. Pod containers share net/IPC/UTS namespaces (matches podman default — `localhost:PORT` works) but mount + PID namespaces are also shared because the Lambda execution environment doesn't grant `CAP_SYS_ADMIN`. Per-container restart is NotImpl. | function name `sockerless-pod-<podName>-...` | + tag `sockerless-pod=<name>`, env `SOCKERLESS_POD_CONTAINERS=<base64-JSON>` for round-trip |
| Image | ECR repository / image | `<account>.dkr.ecr.<region>.amazonaws.com/<repo>:<tag>` | (registry-managed) |
| Network | **Native cross-container DNS is not addressable per-execution.** Lambda VPC config only routes egress; peer-Lambda discovery requires Service Discovery + a separate fronting service. Treat docker networks as bookkeeping only. | (no cloud anchor) | (known limitation) |
| Volume | EFS access point on a sockerless-managed EFS filesystem (shared with ECS), attached at `CreateFunction` via `Function.FileSystemConfigs[]` (Lambda permits one per function, mount path constrained to `/mnt/[A-Za-z0-9_.\-]+`). Requires `SOCKERLESS_LAMBDA_SUBNETS` (Lambda-in-VPC). See `backends/lambda/backend_delegates.go::VolumeCreate` + `aws-common/volumes.go::EFSManager` + the **"Lambda bind-mount translation"** subsection of "Volume provisioning per backend" for the `-v src:dst` translation rules. `/tmp` (512 MB–10 GB ephemeral) is always present per-invocation; named volumes are durable across invocations. | EFS filesystem id + access-point id | EFS resource tags: `sockerless-managed=true`, `sockerless-volume-name=<name>` |
| Exec instance | Reverse-agent overlay (`sockerless-lambda-bootstrap` dials back during `Invoke`); see [Exec](#exec). | (transient agent session) | — |

**State derivation:**

- `docker ps -a` → `ListFunctions` + `ListTags` per function ARN (filter `sockerless-managed=true`), project to `api.Container`.
- `docker images` → `lambdaCloudState.ListImages` paginates ECR `DescribeRepositories` + `DescribeImages` (same ECR that ECS uses).
- `docker exec` → `resolveLambdaState` for FunctionName → dial reverse-agent WebSocket → tunnel through overlay.
- `docker stop/kill/rm/wait/logs` → all go through `Server.resolveLambdaState(ctx, containerID)` cache+cloud-fallback helper.

**In-memory state as a cache:**

- `s.Lambda *StateStore[LambdaState]` — transient cache; `resolveLambdaState` recovers `FunctionARN` + `FunctionName` on miss via `ListFunctions` + tag filter.

### GCP Cloud Run (backend `cloudrun`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Cloud Run **Job** + execution (default) or Cloud Run **Service** with internal ingress + VPC connector when `SOCKERLESS_GCR_USE_SERVICE=1`. | job name `sockerless-<containerID[:12]>` + execution id | label `sockerless_managed=true`, `sockerless_container_id=<id>`, `sockerless_name=<name>` (GCP underscore convention). |
| Pod | Service path: multi-container revision (sidecars). Jobs path: not supported (1 Job = 1 container). | revision ref + sidecar container names | + label `sockerless_pod=<name>` |
| Image | Artifact Registry / GCR | `<region>-docker.pkg.dev/<project>/<repo>/<image>:<tag>` | (registry-managed) |
| Network | Cloud DNS private managed zone (1 zone per docker network, sanitized from name). Cross-container routing needs `SOCKERLESS_GCR_USE_SERVICE=1` + `SOCKERLESS_GCR_VPC_CONNECTOR` — the Service path writes CNAMEs to `Service.Uri`. | managed-zone name | label `sockerless_network=<name>` on the container; the zone itself is discoverable by name `skls-<sanitized>.local` |
| Volume | Cloud Storage (GCS) bucket per volume; injected into Cloud Run Service revision template as `Volume{Gcs{Bucket}}` + `Container.VolumeMounts`. See `backends/cloudrun/backend_delegates.go::VolumeCreate`. Jobs-path volumes also supported via the same GCS bucket lifecycle. | bucket name `sockerless-volume-<id>` | label `sockerless_managed=true`, `sockerless_volume_name=<name>` |
| Exec instance | Reverse-agent overlay — bootstrap dials `SOCKERLESS_CALLBACK_URL` → `/v1/cloudrun/reverse`; see [Exec](#exec). | (transient agent session) | — |

**State derivation:**

- `docker ps -a` → `Jobs.ListJobs` + `Executions.ListExecutions` per job, filter by label `sockerless-managed=true`. With `UseService=true`: also includes `Services.ListServices`.
- `docker stop` → `Jobs.CancelExecution` on the active execution. With `UseService=true`: `Services.DeleteService` (or revision rollback).
- `docker network ls` → `ManagedZones.List` filter by label `sockerless:network=*`.
- `docker images` → Artifact Registry `Images.List` filtered by repo path.
- `docker logs` → Cloud Logging `LogAdmin.Entries(filter='resource.type="cloud_run_revision" labels.execution_name="<exec>"')`.

**In-memory state as a cache:**

- `s.CloudRun *StateStore[CloudRunState]` — transient cache; `resolveCloudRunState` recovers `JobName` (via `ListJobs` + label filter on `sockerless_container_id`) + `ExecutionName` (via `ListExecutions`, filter to non-terminal) on miss.
- `s.NetworkState *StateStore[NetworkState]` — transient cache; `resolveNetworkState` recovers `ManagedZoneName` via `ManagedZones.Get(<sanitized>)`.
- `docker images` cloud-derived via `cloudRunCloudState.ListImages` using the shared `core.OCIListImages` against `<region>-docker.pkg.dev`.

### Azure Container Apps (backend `aca`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | ACA **Job** + execution (default, `armcontainerapps.JobsClient`) or ACA **App** with internal ingress (`armcontainerapps.ContainerAppsClient`) when `SOCKERLESS_ACA_USE_APP=1`. | job name `sockerless-<containerID[:12]>` + execution id | tag `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | App path: ACA App with multi-container template (sidecars). Jobs path: not supported. | app name + sidecar container names | + tag `sockerless-pod=<name>` |
| Image | ACR | `<acrName>.azurecr.io/<repo>:<tag>` | (registry-managed) |
| Network | Azure Private DNS Zone (per-network) + per-network NSG. App path writes CNAMEs to `LatestRevisionFqdn` for cross-container DNS. | zone name + NSG id | tag `sockerless-network=<name>` on the container; zone is discoverable by name `skls-<network>.local` |
| Volume | Azure Files share in a sockerless-owned storage account, registered as a `ManagedEnvironments/storages` resource and referenced from the Job/App template's `Volumes[]` + `Container.VolumeMounts`. See `backends/aca/backend_impl.go::VolumeCreate`. | storage account + share name | tag `sockerless-managed=true`, `sockerless-volume-name=<name>` |
| Exec instance | ACA console exec API (`Microsoft.App/jobs/{job}/executions/{exec}/exec` via `aca/exec_cloud.go`), with the reverse-agent preferred when present (bootstrap dials `/v1/aca/reverse`); see [Exec](#exec). | (transient management-API or agent session) | — |

**State derivation:**

- `docker ps -a` → `JobsClient.NewListByResourceGroupPager(rg)` + `JobsExecutionsClient.NewListPager(rg, jobName)` for active executions, filter by tag `sockerless-managed=true`. With `UseApp=true`: also includes `ContainerAppsClient.NewListByResourceGroupPager(rg)`.
- `docker stop` → `JobsExecutionsClient.BeginStop(rg, jobName, execName)`. With `UseApp=true`: `ContainerAppsClient.BeginStop(rg, appName)`.
- `docker network ls` → `PrivateZonesClient.NewListByResourceGroupPager(rg)` filter by tag `sockerless:network=*`.
- `docker images` → ACR `RegistryClient.NewListImportImagesPager` for the configured ACR.
- `docker logs` → Log Analytics workspace queries on `ContainerAppConsoleLogs_CL` filtered by container app + execution name.

**In-memory state as a cache:**

- `s.ACA *StateStore[ACAState]` — transient cache; `resolveACAState` recovers `JobName` (via `Jobs.NewListByResourceGroupPager` + tag filter) + `ExecutionName` (via `Executions.NewListPager`, filter to `Running`/`Processing`) on miss.
- `s.NetworkState *StateStore[NetworkState]` — transient cache; `resolveNetworkState` recovers `DNSZoneName` via `PrivateDNSZones.Get(skls-<net>.local)` and `NSGName` via `NSG.Get(nsg-<env>-<net>)`.
- `docker images` cloud-derived via `acaCloudState.ListImages` using the shared `core.OCIListImages` against `<ACRName>.azurecr.io`.

### GCP Cloud Run Functions (backend `cloudrun-functions` / `gcf`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Cloud Function (gen 2) — backed by `cloudfunctions.v2.FunctionService`. Sockerless overlay image runs as the function's actual workload (see "Deploy sequence" below). One Function maps to *at most one* live container at a time via the `sockerless_allocation` label; when the container is removed the Function may go back into the reuse pool (see "Stateless image cache + Function reuse pool"). | function name `sockerless-<overlayHash>-<n>`, full resource path `projects/<project>/locations/<region>/functions/<name>`. The `<n>` suffix lets multiple Functions coexist for the same overlay (pool capacity) — sockerless never reuses a name within a pool. | label `sockerless_managed=true`, `sockerless_overlay_hash=<contentTag>`, `sockerless_allocation=<containerID>` (empty/absent ⇒ in pool, free), `sockerless_name=<name>` (GCP underscore convention; full container ID stored as annotation since GCP label values are 63-char limited). |
| Pod | **Supported via supervisor-in-overlay** (degraded namespace isolation — see "Podman pods on FaaS backends" below). Cloud Functions Gen2 backs each Function with a single Cloud Run container; sockerless's overlay bakes all pod containers' rootfs and a supervisor bootstrap into one image. Pod containers share net/IPC/UTS namespaces (matches podman pod default — `localhost:PORT`, `/dev/shm` work) but mount + PID namespaces are also shared because the Cloud Run sandbox blocks `unshare(CLONE_NEWNS|CLONE_NEWPID)` (no CAP_SYS_ADMIN). Compatible with CI workloads (gitlab/github runner `services:` sidecars). Per-container restart is NotImpl. | function name `sockerless-pod-<podName>-...` (overlay tagged with combined content hash) | + label `sockerless_pod=<name>`, env `SOCKERLESS_POD_CONTAINERS=<base64-JSON of [{name,image,entrypoint,cmd}]>` for round-trip |
| Image | Artifact Registry — overlay built once per content hash by Cloud Build, pushed to a sockerless-owned AR repo. | `<region>-docker.pkg.dev/<project>/sockerless-overlay/gcf:<contentTag>` where `<contentTag> = sha256(user-image, bootstrap-binary, user-cmd, user-entrypoint, user-workdir)[:16]` | (content-addressed; repo lifecycle managed by terraform). The Docker Hub remote-repo at `<region>-docker.pkg.dev/<project>/docker-hub/library/<name>` proxies user images for the overlay's `FROM`. |
| Network | **Not supported natively.** Cloud Functions can connect to a VPC for egress via a connector, but they don't expose addressable inbound IPs to peer functions. Cross-container DNS via a docker-network abstraction is not implementable on Cloud Functions; backend treats `docker network create` / `connect` as a no-op for cloud purposes (returns success but the network is bookkeeping only). | (no cloud anchor) | — |
| Volume | GCS bucket per volume (shared lifecycle helper with Cloud Run); attached via the function's runtime config (BYO mount path). See `backends/cloudrun-functions/backend_delegates.go::VolumeCreate`. `/tmp` (read/write, ephemeral) is always present per-invocation. | bucket name `sockerless-volume-<id>` | label `sockerless_managed=true`, `sockerless_volume_name=<name>` |
| Exec instance | Reverse-agent overlay (no native exec on Cloud Functions); see [Exec](#exec). | (transient agent session) | — |

**Deploy sequence (`docker run <user-image> <user-cmd>` on a free overlay):**

Cloud Run Functions Gen2's `CreateFunction` API requires a Buildpacks-compatible source archive (Go/Node/Python/Java/Ruby/PHP/.NET) — there is **no documented path** to deploy a generic OCI image directly. To deploy sockerless's overlay (`FROM <user-image>` + `sockerless-gcf-bootstrap`), the backend uses a two-stage flow:

1. **Compute `<contentTag>`** = `OverlayContentTag(spec)` — sha256 of (resolved-user-image, bootstrap-binary-path, user-entrypoint, user-cmd, user-workdir).
2. **Pool query**: `Functions.ListFunctions(filter: sockerless_managed=true AND sockerless_overlay_hash=<contentTag>)`. From the result, pick any with `sockerless_allocation=""`. **Atomic claim** via `Functions.UpdateFunction(labels.add: sockerless_allocation=<containerID>)` with the function's current `etag`. Etag mismatch ⇒ another sockerless instance won; loop. If a free function is claimed, **skip to step 6**.
3. **Image cache check**: `ArtifactRegistry.GetDockerImage(URI=<region>-docker.pkg.dev/<project>/sockerless-overlay/gcf:<contentTag>)`. 200 ⇒ overlay already exists; skip to 4. 404 ⇒ next step.
4. **Overlay build via Cloud Build**: tar a `Dockerfile` (`FROM <resolved-user-image>`, `COPY sockerless-gcf-bootstrap /opt/sockerless/...`, `ENV SOCKERLESS_USER_*=...`, `ENTRYPOINT [".../bootstrap"]`) + the bootstrap binary, upload to `gs://<build-bucket>/`, fire `cloudbuild.CreateBuild(steps: [docker build, docker push])` against the AR URI from step 3. Cloud Build deduplicates by source hash so re-fires are no-ops.
5. **Stub-source CreateFunction**: stage a no-op Go source archive at `gs://<build-bucket>/sockerless-stub-go.zip` (one-time per project; the source is identical for every sockerless deployment), `Functions.CreateFunction(parent, FunctionId=sockerless-<contentTag>-<n>, BuildConfig{Runtime:"go124", Source:storage(stub-zip), EntryPoint:"Stub"}, ServiceConfig{...env vars...}, Labels{sockerless_managed=true, sockerless_overlay_hash=<contentTag>, sockerless_allocation=<containerID>})`. Buildpacks builds a throwaway image; the function moves to ACTIVE in 30-60s.
6. **Image swap**: `Run.Services.UpdateService(name=<function.ServiceConfig.Service>, Template.Containers[0].Image=<overlay-AR-URI>)` to replace the Buildpacks-built throwaway with our overlay. Cloud Functions does not reconcile this field — the swap holds.
7. **Invoke**: HTTP POST to `Function.ServiceConfig.Uri`. The `sockerless-gcf-bootstrap` inside the overlay handles the request, exec's `SOCKERLESS_USER_*` as a subprocess, returns stdout in the response body, and copies stdout/stderr to its own (which Cloud Logging captures under `run.googleapis.com%2Fstdout` for the existing `buildCloudLogsFetcher`).

**The stub-Buildpacks-source step is not a hack** — it's the documented escape hatch for non-Buildpacks-compatible deployments and is the same pattern as `attachVolumesToFunctionService`'s post-create UpdateService for volume mounts. Cloud Functions' API surface manages function metadata (URL, IAM, trigger spec); the underlying Cloud Run Service's `Template.Containers[0].Image` is operator-controlled and persists across function updates.

**Release sequence (`docker rm <containerID>`):**

1. `Functions.ListFunctions(filter: sockerless_allocation=<containerID>)` ⇒ the claimed function.
2. Count free (allocation-empty) functions for the same overlay-hash: `ListFunctions(filter: sockerless_overlay_hash=<contentTag> AND sockerless_allocation="")`.
3. If count `>= SOCKERLESS_GCF_POOL_MAX` (default 10) ⇒ `Functions.DeleteFunction(name)` (returns to a steady-state pool size).
4. Otherwise ⇒ `Functions.UpdateFunction(labels.remove: sockerless_allocation)` to release back to the pool. Future `docker run` calls will reuse this function via the claim path in step 2 of the deploy sequence — amortized startup matches Cloud Run Functions' normal warm-pool cold-start (~1-3s, sub-100ms if `min-instances=1`).

**State derivation (every list/inspect call queries the cloud — no local caches):**

- `docker ps -a` → `Functions.ListFunctions(parent="projects/<project>/locations/<region>", filter: sockerless_managed=true AND sockerless_allocation!="")`. Free pool entries are excluded from `ps -a` (they have no associated container). Each returned function projects to one `api.Container` via the `sockerless_allocation`+`sockerless_container_id` labels.
- `docker stop` → mark container exited via `Store.LogBuffers` (the function keeps existing in the pool); subsequent `docker rm` runs the release sequence above.
- `docker images` → `gcfCloudState.ListImages` via the shared `core.OCIListImages` against `<region>-docker.pkg.dev` with token from `ARAuthProvider`.
- `docker logs` → Cloud Logging `LogAdmin.Entries(filter='resource.type="cloud_run_revision" labels.service_name="<service>" AND logName:"run.googleapis.com"')`. The `logName:` substring clause excludes Cloud Audit Logs (cf BUG-878).

**Stateless invariant — implementation:** the gcf backend has zero per-container `StateStore` entries. All container/pool state is the cloud-side label set on Functions, queried fresh on every operation. `s.GCF *StateStore[GCFState]` from earlier rounds is gone — pool semantics make a transient cache useless because allocation can change concurrently across sockerless instances.

### Azure Functions (backend `azure-functions` / `azf`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Function App (Linux container deployment) — `armappservice.WebAppsClient` | function app name `sockerless-<containerID[:12]>` | tag `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | **Supported via supervisor-in-overlay** (degraded namespace isolation — see "Podman pods on FaaS backends" below). Azure Functions on Linux container plans (Premium / Flex Consumption / App Service) back each Function App with a single Linux container; the overlay bakes all pod containers' rootfs and `sockerless-azf-bootstrap` (supervisor) into one image. Pod containers share net/IPC/UTS namespaces (matches podman default — `localhost:PORT` works) but mount + PID namespaces are also shared because Function App containers don't get `CAP_SYS_ADMIN` by default. Per-container restart is NotImpl. | function app name `sockerless-pod-<podName>-...` | + tag `sockerless-pod=<name>`, app setting `SOCKERLESS_POD_CONTAINERS=<base64-JSON>` for round-trip |
| Image | ACR | `<acrName>.azurecr.io/<repo>:<tag>` | (registry-managed) |
| Network | **Not supported natively.** Function Apps support VNet integration for outbound traffic but not addressable inbound IPs for peer apps. `docker network create` / `connect` is bookkeeping-only. | — | — |
| Volume | Azure Files share in a sockerless-owned storage account, attached to the Function App via `sites/<fn>/config/azurestorageaccounts`. See `backends/azure-functions/backend_delegates.go::VolumeCreate`. | storage account + share name | tag `sockerless-managed=true`, `sockerless-volume-name=<name>` |
| Exec instance | Reverse-agent overlay (Kudu console / SSH not implemented); see [Exec](#exec). | (transient agent session) | — |

**State derivation:**

- `docker ps -a` → `WebApps.NewListByResourceGroupPager(resourceGroup)`, filter by tag `sockerless-managed=true`, project to `api.Container`.
- `docker stop` → `WebApps.Stop(name)` (function app stays defined but doesn't run).
- `docker rm` → `WebApps.Delete(name)`.
- `docker images` → `azfCloudState.ListImages` via the shared `core.OCIListImages` against `config.Registry` (the ACR hostname) with token from `ACRAuthProvider`.
- `docker logs` → App Service container logs via `WebApps.GetContainerLogsZip` or `LogAnalytics` queries on the workspace linked to the App.

**In-memory state as a cache:**

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

For long-running containers (ECS tasks, Cloud Run Jobs, ACA Jobs, Cloud Run Services, ACA ContainerApps) the cloud resource IS the container — `docker inspect` / `docker wait` / `docker ps` read directly from `DescribeTasks` / `Execution.status` / `Revision.status`. For **FaaS backends** the cloud function is long-lived but *invocations* are ephemeral, so `docker wait` needs a per-invocation signal, not a function-level one. Each backend has exactly one cloud-native signal for invocation completion + exit code, captured by the per-backend `core.InvocationResult` tracker.

| Backend | Container-lifecycle resource | Completion signal | Exit-code source |
|---------|------------------------------|-------------------|------------------|
| `ecs` | `Task` | `Task.LastStatus=STOPPED` | `Task.Containers[].ExitCode` |
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

Real per-cloud volume provisioning: ECS + Lambda → EFS access points; Cloud Run + GCF → GCS buckets; ACA + AZF → Azure Files shares. Host-path binds remain rejected (no host filesystem in the cloud).

| Backend | Cloud resource | Lifecycle mapping | IAM / API actions needed | Simulator work |
|---------|----------------|--------------------|--------------------------|----------------|
| `ecs` | **EFS** file system + per-AZ mount targets + per-volume access point. Access point maps the volume name to a subdirectory owned by a fixed UID/GID so tasks can't trample each other. | `VolumeCreate` → ensure one EFS per backend (reuse by tag `sockerless-managed=true`), then `CreateAccessPoint` per volume, store volume-name → access-point-id in tags. `VolumeRemove` → `DeleteAccessPoint` (EFS stays, holding other volumes). Bind / named mounts → inject `EFSVolumeConfiguration{FileSystemId, AccessPointId, TransitEncryption=ENABLED}` into the task-def's `Volumes` array + `MountPoints` in the container def. | `elasticfilesystem:CreateFileSystem`, `DescribeFileSystems`, `CreateMountTarget`, `DescribeMountTargets`, `CreateAccessPoint`, `DescribeAccessPoints`, `DeleteAccessPoint`, `TagResource`, `PutFileSystemPolicy`. Task execution role needs `elasticfilesystem:ClientMount/ClientWrite/ClientRootAccess`. | `simulators/aws/efs.go` — real EFS-like slice. Store file systems + mount targets + access points; back access points with per-volume subdirectories on a host-side Docker volume so the per-task Docker container can mount the same path and see the same files. |
| `lambda` | **EFS** file system + per-AZ mount targets + access points (shared with `ecs`'s `EFSManager`). Each Lambda function gets at most one `FileSystemConfig` (Lambda enforces this) mounted at a single `/mnt/<name>` path (Lambda enforces `localMountPath` to match `/mnt/[A-Za-z0-9_.\-]+`). `/tmp` (512 MB–10 GB ephemeral) is always present per-invocation; EFS-backed named volumes are durable across invocations and across functions. | `VolumeCreate` → `EFSManager.AccessPointForVolume` (creates an AP whose `RootDirectory.Path` is unique per volume). Bind / named mounts → translated by `backends/lambda/volumes.go::fileSystemConfigsForBinds` per the rules in **"Lambda bind-mount translation"** below — multiple Docker volumes that share an access point collapse to one `FileSystemConfig`; non-`/mnt/` Docker target paths get bootstrap-time symlinks. | Same as ECS — `EFSManager` is shared. | Sim Lambda runtime needs `FileSystemConfigs` to be honoured on the simulator's container path so EFS-backed binds work end-to-end against the sim. |
| `cloudrun` | **GCS bucket** per volume (simplest first pass), mounted via Cloud Run Service's native `Volume{Gcs{Bucket}}` in the revision template. Optional upgrade to **Filestore** later for POSIX semantics if `O_APPEND` / file locking is needed. | `VolumeCreate` → `storage.Buckets.Insert` with naming `sockerless-volume-<id>`, label `sockerless-managed=true`. `VolumeRemove` → `DeleteBucket` (requires empty; force=true uses `DeleteObjects` first). Bind / named mount → inject `RevisionTemplate.Volumes[].Gcs{Bucket}` + `Container.VolumeMounts` in the service spec. | `storage.buckets.create/delete/list`, `storage.objects.*` for prune/delete. Cloud Run service account needs `roles/storage.objectAdmin` on buckets it mounts. | `simulators/gcp/storage.go` already has GCS slice; extend with `Volume{Gcs}` honouring on the Cloud Run simulator path so the backing Docker container gets a real bind mount against the sim's bucket directory. |
| `aca` | **Azure Files share** in a sockerless-owned storage account, linked into the managed environment as an `ManagedEnvironments/storages` resource, then referenced from `ContainerApp.Properties.Template.Volumes[]` + `Container.VolumeMounts`. | `VolumeCreate` → (ensure storage account exists) + `FileShares.Create` + `ManagedEnvironmentsStorages.CreateOrUpdate` so the env knows about the share. `VolumeRemove` → `FileShares.Delete` + `ManagedEnvironmentsStorages.Delete`. Bind / named mount → inject `ContainerAppProperties.Template.Volumes` + `Container.VolumeMounts` into the app spec. | `Microsoft.Storage/storageAccounts/read,write,listKeys`, `Microsoft.Storage/storageAccounts/fileServices/shares/read,write,delete`, `Microsoft.App/managedEnvironments/storages/read,write,delete`. | `simulators/azure/storage.go` gains `fileServices/shares` sub-resource CRUD (the storage slice today is blob-only). `simulators/azure/containerappsenv.go` gains `storages` sub-resource. The sim's ACA container bind-mounts a host-side directory per share so containers see real files. |
| `cloudrun-functions` (gcf) | GCP Cloud Functions (targeting the current v2 API only; v1 not supported). v2 is Cloud Run Services under the hood. | Shared helper with `cloudrun` — same GCS-bucket-mount lifecycle. | Same as cloudrun. | Shares `simulators/gcp/` GCS extensions. |
| `azure-functions` (azf) | Azure Functions on the current Flex Consumption / Premium plan with BYOS Azure Files mounts. | Provision Azure Files share (shared helper with ACA), then attach to the Function App via `sites/<fn>/config/azurestorageaccounts`. | Same Azure Files permissions as ACA + `Microsoft.Web/sites/config/write`. | Shares `simulators/azure/` Azure Files extensions. |
| `docker` | Real Docker volumes via the local daemon. | Already implemented — passthrough to `docker volume *` on the host daemon. | — | — |

Each cloud's volume work is filed as its own phase in [PLAN.md](../PLAN.md) so real provisioning lands as discrete, reviewable units.

### Lambda bind-mount translation

Lambda's volume primitive carries two hard constraints that diverge from Docker's volume model:

1. **At most one `FileSystemConfig` per function.** A Lambda function can mount one EFS access point — no more.
2. **`localMountPath` must match `/mnt/[A-Za-z0-9_.\-]+`.** Mounting at arbitrary paths (`/__w`, `/home/runner/_work`, etc.) is rejected by `lambda.CreateFunction`.

Sockerless's `backends/lambda/volumes.go::fileSystemConfigsForBinds` translates Docker `-v src:dst` into Lambda primitives subject to those constraints:

- **Single AP per function.** All EFS-backed volumes a Lambda function references must share one access point. When multiple `SharedVolume` entries (declared via `SOCKERLESS_LAMBDA_SHARED_VOLUMES`) name the same `AccessPointID`, they collapse to one `FileSystemConfig`. Multiple distinct APs in the same `CreateFunction` call are rejected at sockerless's boundary with a clear error pointing at this constraint.
- **Single mount path; bind targets are symlinks.** The collapsed `FileSystemConfig` mounts at `/mnt/sockerless-shared`. Each Docker bind's `dst` is exposed via a **symlink** created by sockerless's bootstrap before the user entrypoint runs. The symlink target is `/mnt/sockerless-shared/<EFSSubpath>`, where `EFSSubpath` is declared on the `SharedVolume` (the directory under the AP root where that volume's data lives — e.g. `_work` for the runner workspace, `externals` for actions/runner externals).
- **`SOCKERLESS_LAMBDA_BIND_LINKS` env var** carries `<dst>=/mnt/sockerless-shared/<EFSSubpath>` mappings into the sub-task function. The bootstrap parses this on startup, `mkdir -p`s the parent of each `dst`, and `ln -sfn`s the link.
- **`/var/run/docker.sock` binds drop silently** — Lambda has no docker socket; the runner-side process should be using sockerless on `localhost:3375` instead.

`SOCKERLESS_LAMBDA_SHARED_VOLUMES` syntax accommodates the AP-subpath:

    name=containerPath=fsap-XXXX                            # AP root contains the volume's data
    name=containerPath=fsap-XXXX=fs-YYYY                    # explicit FS id, AP root
    name=containerPath=fsap-XXXX==subpath                   # AP root + subpath
    name=containerPath=fsap-XXXX=fs-YYYY=subpath            # explicit FS + subpath

When `subpath` is set, the volume's data lives under `<APRoot>/<subpath>` on EFS; bind translations point their symlinks at `/mnt/sockerless-shared/<subpath>` accordingly.

This is the only correct mapping of Docker's `-v` semantics onto Lambda's volume primitive — same nature as sockerless's reverse-agent translation of `docker exec` for Lambda (which has no docker exec), or sockerless's metadata-only network driver for Fargate (which has its own netns).

### Lambda exec semantics

Lambda has no native `docker exec` primitive — once a function is invoked, there's no inbound channel to push additional commands into the running execution environment. Sockerless implements `docker exec` against Lambda containers via two complementary translations:

**Path A — reverse-agent (preferred when reachable):** the bootstrap dials a long-lived WebSocket back to sockerless at `SOCKERLESS_CALLBACK_URL` during init; sockerless pushes `TypeExec` messages over the WebSocket; the bootstrap spawns the command in the same execution environment, streams stdout/stderr/exit-code back. Preserves Docker fidelity (multiple execs share `/tmp`, file descriptors, etc.). Requires a stable inbound endpoint reachable from the sub-task's VPC subnets — typically API Gateway WebSocket API or a separate sockerless service running outside Lambda (e.g. ECS Fargate behind an NLB).

**Path B — exec-via-Invoke (fallback, native to Lambda's primitive):** each `docker exec` triggers a fresh `lambda.Invoke` whose Payload is a JSON envelope `{"sockerless":{"exec":{"argv":[...],"tty":...,"workdir":...,"env":[...]}}}`. The bootstrap parses the envelope, spawns the command, returns `{"sockerlessExecResult":{"exitCode":N,"stdout":"<base64>","stderr":"<base64>"}}` via `/response`. Sockerless tunnels the response into the docker-exec attach stream. Each exec is a separate Lambda invocation: the execution environment may or may not be reused (Lambda's warm-pool decision). State persistence between execs is via EFS-mounted volumes only — `/tmp` does NOT persist across invocations. Required when no inbound endpoint is available (e.g., sockerless baked into the runner-Lambda image with no fronting API Gateway).

Choice of path is per-container, decided at exec time:
1. If `s.reverseAgents.Resolve(containerID)` returns a registered session → Path A.
2. Else if the function is `Active` and reachable via `lambda.Invoke` → Path B.
3. Else → `NotImplementedError` with a clear message.

Path B's payload format matches what `agent/cmd/sockerless-lambda-bootstrap/main.go` parses in `runUserInvocation`. An empty Payload (or a non-JSON one) keeps the existing "run user entrypoint+cmd as a subprocess" behaviour for the function's main invocation.

### ECS gitlab-runner script delivery (Fargate has no runtime stdin)

**Per-job containers, per-stage scripts.** Each gitlab-runner job creates its own pair of containers — one helper (image: `gitlab-runner-helper`, name suffix `-predefined`) and one build (image from `.gitlab-ci.yml`'s `image:` field, e.g. `alpine`) — plus one per `services:` entry. Both live for exactly that one job; `docker rm` runs at job end. The next job creates fresh containers from scratch. No state crosses job boundaries.

Within a job, gitlab-runner walks both containers through ~10 stages (`prepare_script`, `get_sources`, `download_artifacts`, `step_script`, `after_script`, `archive_*`, `upload_artifacts_*`, `cleanup_file_variables`). For each stage, gitlab-runner does `docker start <container>` followed by `docker attach -i <container>` with the stage's generated shell script piped as stdin bytes. Real Docker re-runs the container's ENTRYPOINT on each `start` of a STOPPED container; the entrypoint reads stdin, runs the script, exits when stdin EOFs.

Both containers (helper and build) are created with the same stdin-reading entrypoint — gitlab-runner overrides whatever the source image had:

```
ENTRYPOINT ["sh", "-c",
  "if [ -x /usr/local/bin/bash ]; then exec /usr/local/bin/bash; \
   elif [ -x /usr/bin/bash ]; then exec /usr/bin/bash; \
   ... etc ... \
   else echo shell not found; exit 1; fi"]
```

The "Running on $(hostname) via $(client)..." identity banner per stage comes from the FIRST LINE of the generated shell script, NOT from helper-image-specific code. The helper and build containers are functionally just shell-script-runners; only their image filesystem differs.

**Fargate breaks this lifecycle in two ways**:

1. Fargate tasks are **not restartable**. Once a task transitions to STOPPED, that task ARN is gone; `ecs.RunTask` always creates a new task. Real Docker's `docker start` semantics ("re-run entrypoint on stopped container") have no Fargate equivalent.
2. Fargate has **no runtime stdin channel**. Once `RunTask` starts, the task's PID-1 stdin is closed; no SDK call writes more bytes to it. Real Docker's `docker attach -i` ("pipe bytes into a running container's stdin") has no Fargate equivalent either.

**Sockerless's translation rule — one Fargate task per gitlab-runner job, multi-container.** ECS task definitions are multi-container by design: one task can host the helper container, the build container, and any `services:` sidecars in a single `RunTask`. The containers share the task's network namespace (so `localhost` works between helper and build), share the same task IAM role, and each container is independently addressable by `ecs.ExecuteCommand --container <name>`. This is the natural mapping for gitlab-runner's "one job uses N containers on a shared docker network" topology.

The grouping signal is the **docker network**: gitlab-runner creates a job-scoped network (`runner-XXX-project-YYY-concurrent-Z-NNN`) and creates each of the job's containers with `--network <that network>`. Sockerless detects this signal at /start time:

1. **/create** records the container's network membership in `PendingCreates` but doesn't register a task definition yet.
2. **First /start that targets a user-defined network** scans `PendingCreates` for sibling containers on the same network. If one or more siblings exist, sockerless registers a multi-container task definition with one `ContainerDefinition` per sibling (entrypoint + cmd preserved per container, including the long-lived idle loop for stdin-pipe containers; `enableExecuteCommand: true` set on every container) and runs the task once.
3. **Each container in the multi-container task** caches `(containerID → (taskARN, containerName))`. The task-level state is shared; per-container exit codes come from the task's STOPPED `containers[].exitCode` field once the task completes.
4. **Subsequent /start cycles on the same container ID** skip RunTask and rely on `ecs.ExecuteCommand --task <ARN> --container <name> --interactive --command "/bin/sh"` to deliver each stage's buffered stdin script. Sockerless writes the script bytes through the SSM session, streams stdout/stderr into the docker `/attach` hijacked connection (multiplexed-stream framing for non-tty), captures the exit-code marker emitted at the end of each script as the stage's exit status.
5. **/exec** on any container hits the same `ExecuteCommand --container <name>` path against the live task — already implemented for non-stdin /exec by `cloudExecStart`.
6. **/wait** for a container blocks until the task transitions to STOPPED and reads the per-container exit code from the task's `containers[]` array. **/stop and /kill** call `ecs.StopTask` (the entire task; its containers go down together — gitlab-runner removes the helper and build containers as a pair at job end, so this matches gitlab-runner's lifecycle). **/rm** drops cache entries and deregisters the task definition.

Per-stage / per-container script delivery rules within the multi-container task:

> **Stdin-pipe lifecycle (gitlab-runner pattern; `OpenStdin && AttachStdin`)** — applies to BOTH the helper and the build container in the multi-container task. The container's `ContainerDefinition.Command` is the long-lived idle loop (`["sh","-c","while true; do sleep 60; done"]`) so the container stays running for the whole task lifetime. Per /start cycle: stdin script bytes get written to an SSM session opened against `--container <name>`. The shell session reads + executes the script. Stdout/stderr stream back through the hijacked `/attach`. Exit-code marker (echoed by sockerless's `wrapWithExitCodeMarker`) carries the stage exit status.

> **Single-shot lifecycle** (no stdin pipe) — the original `Entrypoint`/`Cmd` from /create is preserved in the task definition; the container runs once and exits. /wait surfaces its exit code from the task's `containers[]` array. This is the path for sidecar `services:` containers that just need to start, run their image's entrypoint, and stay reachable on the task's network.

Single-container fallback: if the container at first-/start has no user-defined network OR has no sibling containers in `PendingCreates` on that network, sockerless registers a single-container task definition (current behaviour preserved for `docker run` / GitHub-Actions-runner workloads where there's only one job container).

The docker-network signal works for any client that follows the "create a network, attach all of the job's containers to it" idiom (gitlab-runner, docker-compose, k8s pods translated through libpod's pod API). No client-specific name parsing required.

**Cell 4 (GitLab × Lambda) — one Lambda function per container.** Lambda has no multi-container execution model — each function runs exactly one container. Each of gitlab-runner's per-job containers (helper, build, services) maps 1:1 to its own Lambda function: sockerless's existing `lambda.CreateFunction`-per-`docker create` flow already does this. There's no equivalent of ECS's "multi-container task" grouping; each function is independent. The functions don't need to talk to each other at runtime — gitlab-runner's helper and build containers coordinate via the EFS-mounted `$CI_PROJECT_DIR` (workspace), the same way they would on a single Docker host (Docker shares the `/builds` volume between them; sockerless's bind-mount → EFS translation routes both functions to the same access point under `/mnt/sockerless-shared/_work`). All cross-stage state lives on EFS.

Each gitlab-runner stage's stdin-piped script becomes one fresh `lambda.Invoke` whose Payload is a SCRIPT envelope `{"sockerless":{"script":{"shell":"sh","body":"<base64>","workdir":"...","env":[...]}}}`. The bootstrap parses, runs `bash -c "<decoded body>"` as a subprocess, returns `{"sockerlessScriptResult":{"exitCode":N,"stdout":"...","stderr":"..."}}`. Cross-stage state persists via EFS only — `/tmp` is per-invocation. Same per-job-isolation rule as ECS: each gitlab-runner job creates its own functions, deletes them at /rm. (Phase 117.)

The stock `gitlab-runner-helper` image is used unmodified on both backends. Sockerless's overlay-inject path (Phase 115) wraps it with `sockerless-lambda-bootstrap` as ENTRYPOINT; the bootstrap parses each Invoke's Payload (exec or script envelope) and dispatches. The helper image's `gitlab-runner-helper` binary remains on the function's PATH; per-stage scripts can invoke it directly when needed.

**Path A (reverse-agent) for either backend** stays available when `SOCKERLESS_CALLBACK_URL` is set: the bootstrap dials back via WebSocket and sockerless pushes per-stage messages over the connection. Path A preserves Docker fidelity (multiple stages share `/tmp`, file descriptors) at the cost of inbound network. Phases 114 and 117 implement the no-inbound-network paths and remain the default when CallbackURL is unset.

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
| ContainerStats (one-shot, `--no-stream`) | ✓ | ⚠ CloudWatch — latest aggregate | ⚠ CloudWatch | ⚠ Cloud Monitoring | ⚠ | ⚠ Log Analytics | ⚠ |
| ContainerStats (streaming) | ✓ | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap |
| ContainerTop | ✓ | ⚠ via SSM | ⚠ agent only — ✗ accepted gap when no agent | ⚠ agent only — ✗ accepted gap when no agent | ⚠ agent only — ✗ accepted gap when no agent | ⚠ agent only — ✗ accepted gap when no agent | ⚠ agent only — ✗ accepted gap when no agent |
| ContainerRename | ✓ | ⚠ local-name-only (accepted divergence) | ⚠ local-name-only (accepted divergence) | ⚠ local-name-only (accepted divergence) | ⚠ local-name-only (accepted divergence) | ⚠ local-name-only (accepted divergence) | ⚠ local-name-only (accepted divergence) |
| ContainerUpdate | ✓ | ⚠ limited — CPU/mem only via task-def rev | ⚠ | ⚠ via new revision | ⚠ | ⚠ via new revision | ⚠ |
| ContainerResize | ✓ | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap |
| ContainerPause | ✓ | ⚠ via SSM (bootstrap-pidfile required) | ⚠ agent+opt-in | ⚠ agent+opt-in | ⚠ agent+opt-in | ⚠ agent+opt-in | ⚠ agent+opt-in |
| ContainerUnpause | ✓ | ⚠ via SSM (bootstrap-pidfile required) | ⚠ agent+opt-in | ⚠ agent+opt-in | ⚠ agent+opt-in | ⚠ agent+opt-in | ⚠ agent+opt-in |
| ContainerCommit | ✓ | ✗ ECS no-agent | ⚠ agent+opt-in | ⚠ agent+opt-in | ⚠ agent+opt-in | ⚠ agent+opt-in | ⚠ agent+opt-in |
| ContainerExport | ✓ | ⚠ via SSM | ⚠ agent only — ✗ accepted gap when no agent | ⚠ agent only — ✗ accepted gap when no agent | ⚠ agent only — ✗ accepted gap when no agent | ⚠ agent only — ✗ accepted gap when no agent | ⚠ agent only — ✗ accepted gap when no agent |
| ContainerChanges | ✓ | ⚠ via SSM | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only |
| ContainerStatPath | ✓ | ⚠ via SSM | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only |
| ContainerGetArchive | ✓ | ⚠ via SSM | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only |
| ContainerPutArchive | ✓ | ⚠ via SSM | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only |
| ContainerPrune | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| ContainerAttach | ✓ | ✓ (CloudWatch stream) | ⚠ agent only | ⚠ agent only | ⚠ agent only | ⚠ agent only / ACA console | ⚠ agent only |

Notes:

- **ContainerStats ⚠** — cloud providers only surface aggregated per-task metrics with ~60s lag; no block-I/O or network-byte counters equivalent to docker's cgroup stats. Sockerless reports CPU-ns + mem-bytes + PIDs=0 when nothing's there yet, never synthetic numbers.
- **ECS via SSM** — Container{Top, Changes, StatPath, GetArchive, PutArchive, Export, Pause, Unpause} on ECS run their respective shell commands (`ps`, `find`, `stat`, `tar`, `kill`) over `ExecuteCommand` via the SSM AgentMessage protocol. Implementations live in `backends/ecs/ssm_capture.go` + `backends/ecs/ssm_ops.go`; outputs are normalised through `core.Parse{Top,Stat,Changes}Output` for parity with the reverse-agent path. ContainerPause/Unpause additionally need the bootstrap convention (`/tmp/.sockerless-mainpid`) — without it the SSM call exits 64 and the backend surfaces a `NotImplementedError` naming the missing prerequisite.
- **FaaS Container{Top / Stat / GetArchive / PutArchive / Attach} ⚠ agent only** — possible only when the sockerless agent is bundled into the container image (Lambda's agent-as-handler pattern; CR/ACA/GCF/AZF use the same overlay). Without a registered reverse-agent session, every backend returns a `NotImplementedError` that names the missing prerequisite (`SOCKERLESS_CALLBACK_URL`) — never a silently-empty stream. ACA additionally falls back to the cloud-native console exec API for ExecStart/Attach when no agent is present. See [Exec](#exec) below for the full resolution table.
- **ContainerCommit ⚠ agent+opt-in** — the reverse-agent runs `find / -xdev -newer /proc/1` (same reference point as `docker diff`) + `tar -cf - --null -T -` to capture the files added or modified since container boot, then stacks the resulting blob as a new layer on top of the source image's rootfs. Gated behind `SOCKERLESS_ENABLE_COMMIT=1` per backend because the approach can't capture deletions (`find(1)` can't list files that no longer exist, and sockerless has no host-side access to the base image's rootfs to compute whiteouts) — this is documented, not a silent degradation. ECS has no bootstrap equivalent, so it stays `NotImplementedError`. Push to the operator's registry uses the existing `ImageManager.Push` path.
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
- **ECS**: real `ExecuteCommand` via SSM Session Manager. Requires task IAM role grants for `ssmmessages:*` + `EnableExecuteCommand: true` at RunTask + the SSM AgentMessage decoder in the backend.
- **Lambda**: agent-as-handler. `sockerless-lambda-bootstrap` dials back to `/v1/lambda/reverse`; exec tunnels through.
- **Cloud Run / GCF / AZF**: no native exec surface. Reverse-agent overlay is the only path; backends now route through `BaseServer.ExecStart` after verifying the session exists.
- **ACA**: ACA has a native console exec API (`Microsoft.App/jobs/{job}/executions/{exec}/exec`) wired via `aca/exec_cloud.go::cloudExecStart`. The backend prefers the reverse-agent when present and falls back to cloudExecStart otherwise.

#### How other workload schedulers handle exec/attach

For reference, here is how the major job-runner ecosystems implement the same shape of problem. Sockerless's challenge — exec into a FaaS invocation — is one most schedulers sidestep entirely:

| System | Mechanism | Reverse-agent? |
|---|---|---|
| GitLab Runner — `docker` executor | `docker exec` (Moby `ContainerExecCreate` + `ContainerExecAttach`) into the long-lived helper + build containers; one container per job, not per step | No (runner dials Docker) |
| GitLab Runner — `kubernetes` executor | `POST /api/v1/namespaces/{ns}/pods/{pod}/exec` via SPDY/WebSocket from `k8s.io/client-go/tools/remotecommand` | No (runner dials kube-apiserver) |
| GitLab Runner — `shell` / `ssh` / `custom` | Native fork+pipe / SSH session / user-supplied subprocess | No |
| GitLab Runner trace upload | `PATCH /api/v4/jobs/:id/trace` with `Content-Range` every ~3s (HTTP, not WS) | n/a |
| GitHub Actions runner — container job | One `docker create` + `docker start` per job with ENTRYPOINT overridden to `tail -f /dev/null` so the container outlives any single step; every step runs as `docker exec -i ... <containerId> <cmd>` invoked via in-process `ProcessInvoker` (stdio over OS pipes). `docker attach` is **never** used. Source: `actions/runner` `Runner.Worker/Container/DockerCommandManager.cs`, `Handlers/StepHost.cs`. | No |
| GitHub Actions runner — service containers | Same `docker create` + `docker start`, no entrypoint override; logs collected at teardown via `docker logs --details <id>` (no live streaming). | No |
| GitHub Actions runner — Kubernetes (ARC) | `ACTIONS_RUNNER_CONTAINER_HOOKS` JSON-over-stdin hook protocol delegates `prepare_job`/`run_script_step`/`cleanup_job` to an external binary that translates to `kubectl exec`. | No |
| GitHub Actions runner — log streaming | Runner holds a `ClientWebSocket` to Actions' `feedStreamUrl` for live console; durable blobs via REST `AppendLogContentAsync`. | n/a |
| Buildkite Agent | Long-lived agent on host invokes `docker run --rm` per step; `docker exec` for plugin hooks | No |
| Argo Workflows | `kubectl exec` against per-step pods; init/wait containers handle artifact shuffle | No |

Both GitLab Runner and GitHub Actions runner are **strictly pull-based**: the runner process is co-located with (or has direct network access to) a docker daemon, kube-apiserver, or SSH host, and dials it. Neither supports FaaS executors precisely because Lambda/Cloud Run/ACA invocations expose no server-mediated exec primitive. Sockerless's reverse-agent (bootstrap-dials-back) pattern is what fills that gap — it inverts the typical "scheduler → workload" control flow because the cloud control plane provides no inbound channel.

The GitHub Actions `tail -f /dev/null` keep-alive idiom is directly reusable for any sockerless backend that supports long-lived containers (ECS, Cloud Run Services, ACA Apps). For invocation-scoped FaaS (Lambda, Cloud Functions, AZF) it doesn't apply — the platform forces termination at invocation completion regardless of what the entrypoint does.

#### Using a sockerless cloud backend as the docker daemon for GitLab/GitHub runners

Both GitLab Runner's `docker` executor and GitHub Actions runner expect a docker-API-compatible endpoint. Sockerless's cloud backends serve that API, so a runner can target them via `DOCKER_HOST=tcp://<sockerless-backend>:<port>`. The compatibility matrix:

| Backend | Long-lived container model | `tail -f /dev/null` keep-alive | `docker exec` for each step | Suitable as docker daemon for runners? |
|---|---|---|---|---|
| docker | ✓ | ✓ | ✓ | ✓ Out of the box. |
| ecs | ✓ Fargate task | ✓ (task runs whatever entrypoint specified) | ✓ via SSM ExecuteCommand | Each `docker exec` round-trips an SSM session — slower than local Docker but functionally identical. |
| cloudrun (Services, `UseService=true`) | ✓ Long-lived service revision | ✓ | ✓ via reverse-agent | ✓ Bootstrap must be present; CR Services stay warm. |
| aca (Apps, `UseApp=true`) | ✓ Long-lived app revision | ✓ | ✓ via reverse-agent or ACA console exec | ✓ Bootstrap or console exec available. |
| cloudrun (Jobs) | ✗ Execution scoped to one Run | ✗ entrypoint exits → execution completes | ✗ no surface | ✗ Use the Service path instead. |
| aca (Jobs) | ✗ Execution scoped to one Start | ✗ | ✗ | ✗ Use the App path instead. |
| lambda | ✗ Invocation scoped | ✗ Lambda forces termination at handler return | ✗ The bootstrap stays alive only for the duration of one Invoke | ✗ Fundamentally incompatible — Lambda has no long-lived container concept. |
| gcf | ✗ Same as Lambda | ✗ | ✗ | ✗ |
| azf | ✗ Same as Lambda | ✗ | ✗ | ✗ |

**Operational note.** A runner targeting an ECS/CR-Services/ACA-Apps sockerless backend will see one cloud "container" (task / revision / app) per CI job. Each step's `docker exec` becomes a SSM Session / reverse-agent exec round-trip. This is a real compatibility — the runner doesn't know it's not talking to local Docker — but performance is bound by the cloud's exec-channel latency. For latency-sensitive workloads, prefer self-hosted runners against the local `docker` backend.

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
| ImageBuild | ✓ | ✓ CodeBuild | ⚠ | ⚠ Cloud Build | ⚠ | ⚠ ACR build | ⚠ |
| ImageLoad | ✓ | ⚠ tarball → ECR push | ⚠ | ⚠ tarball → AR push | ⚠ | ⚠ tarball → ACR push | ⚠ |
| ImageSave | ✓ | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap |
| ImageSearch | ✓ | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap | ✗ accepted gap |
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
| VolumeCreate | ✓ | ✓ EFS access point | ✓ EFS access point (Lambda-in-VPC) | ✓ GCS bucket | ✓ GCS bucket | ✓ Azure Files | ✓ Azure Files |
| VolumeInspect | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| VolumeList | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| VolumeRemove | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |
| VolumePrune | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ | ✓ |

See "Volume provisioning per backend" section above for the per-backend mechanics. Phases 91-94 are closed; the corresponding `VolumeCreate`/`VolumeInspect`/`VolumeList`/`VolumeRemove`/`VolumePrune` paths now bind to real EFS / GCS / Azure Files. Bind-mounts (`-v /h:/c`) are still rejected with `InvalidParameterError` on every cloud backend — Fargate / Cloud Run / Cloud Functions / ACA / Function Apps have no host filesystem to bind from.

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

## Simulator coverage

Below is the current state — items marked **closed** have full sim-side emulation; items marked **gap** still fall back to real cloud for local/CI testing.

| Item | Backend(s) | Status | Reference |
|------|------------|--------|-----------|
| EFS `AccessPoint` CRUD | ecs, lambda | closed | `simulators/aws/efs.go` |
| Azure Files share slice (`fileServices/shares`) | aca, azf | closed | `simulators/azure/files.go` |
| Managed-environment `storages` sub-resource | aca | closed | `simulators/azure/containerappsenv.go` |
| GCS bucket-mount honouring on Cloud Run spec-builder path | cloudrun, gcf | closed | `simulators/gcp/cloudrun.go` + `simulators/gcp/storage.go` |
| ACA console exec proto | aca | closed | `simulators/azure/containerapps.go::handleACAJobExec` |
| Cloud Run reverse-agent route | cloudrun | closed | backend `/v1/cloudrun/reverse` |
| ACA reverse-agent route | aca | closed | backend `/v1/aca/reverse` |
| Lambda reverse-agent (agent-as-handler) | lambda | closed | `simulators/aws/lambda_runtime.go` exposes the per-invocation Runtime API; reverse-agent works end-to-end against the sim |
| AWS Session Manager agent-side ack validation | ecs | closed | `simulators/aws/ssm_proto.go` mirrors `SerializeClientMessageWithAcknowledgeContent` |
| Cloud-native streaming `ContainerStats` (analog of `docker stats`) | all | gap | future phase — sim would need to expose CW Metrics / Cloud Monitoring / Log Analytics Stats slices to test the lag-tolerant behaviour |
| TTY-resize signal propagation (`ContainerResize` / `ExecResize`) | all | gap | clouds don't propagate `SIGWINCH`; would require a sim-side pipe for local testing only |

---

## Driver framework

Every "perform docker action X against the cloud" decision flows through a typed driver interface in [`backends/core/drivers_typed.go`](../backends/core/drivers_typed.go). The 13 typed dimensions (`ExecDriver`, `AttachDriver`, `LogsDriver`, `SignalDriver`, `ProcListDriver`, `FSDiffDriver`, `FSReadDriver`, `FSWriteDriver`, `FSExportDriver`, `CommitDriver`, `BuildDriver`, `StatsDriver`, `RegistryDriver`) compose into `core.TypedDriverSet`. Each backend constructs one at startup; the BaseServer's HTTP handlers dispatch through it. Operators override per-cloud-per-dimension via `SOCKERLESS_<BACKEND>_<DIMENSION>=<impl>` resolved by [`backends/core/driver_override.go`](../backends/core/driver_override.go).

The full per-backend default-driver matrix lives in **[specs/DRIVERS.md](DRIVERS.md)**. That doc is the source of truth for which typed driver each backend wires for each dimension; this section gives the architecture context.

**Envelope:**

```go
type DriverContext struct {
    Ctx       context.Context
    Container api.Container        // pre-resolved by ResolveContainerAuto
    Backend   string               // "docker" | "ecs" | "lambda" | …
    Region    string
    Logger    zerolog.Logger
}

type Driver interface {
    Describe() string  // "<backend> <dimension> via <transport>; missing: <prereq>"
}
```

The handler resolves the container once via `ResolveContainerAuto`, builds a `DriverContext`, then invokes `s.Typed.<X>.<method>(dctx, opts)`. Per-dimension typed `<X>Options` / `<X>Result` types layer on top. An unset / `NotImpl` driver auto-emits `NotImplementedError` whose message comes from `Describe()`.

**Adapter layer.** Most dimensions ship with a `WrapLegacyXxx` adapter in `backends/core/driver_adapt_*.go` that converts an existing `BaseServer.ContainerXxx` method into the typed shape. Backends that have a cloud-native typed driver override the slot directly (e.g. `s.Typed.Logs = NewCloudLogsLogsDriver(...)` in Lambda's `NewServer`); backends that don't fall back to the wrapping adapter. The wrapper-removal pass tracked in PLAN.md collapses the indirection once every backend has a typed cloud-native driver per dimension.

**Type tightening.** `core.ImageRef` ([backends/core/image_ref.go](../backends/core/image_ref.go)) is the canonical parsed image reference (`{Domain, Path, Tag, Digest}`) used by the typed `RegistryDriver.Push/Pull` boundary. The handler parses once at the dispatch site; the typed driver receives a structured value. The pattern extends to typed Signal enums + a `ResolveImageReg(ImageRef)` helper for the registry-resolution call sites that still use `splitImageRefRegistry` for docker-hub default rewrites.

**Sim contract.** Every default driver works end-to-end against its cloud's simulator. Alternate drivers (Kaniko, OverlayUpper) are operator-installable only.

**Driver-impl testing.** Sim-only — drivers test against the real cloud SDK pointed at the simulator, matching the project culture (no mocks).

## State boundaries

These are the only places sockerless backends are allowed to keep state:

1. **Configuration** (read-only at startup): `~/.sockerless/contexts/*/config.json`, env vars.
2. **In-memory caches**: anything queried from cloud actuals, scoped to the backend lifetime, invalidated on miss.
3. **CLI run-state** (the management binary `cmd/sockerless`, not the backend itself): `~/.sockerless/run/<context>/backend.pid`.
4. **Per-process transient state**: HTTP-request-scoped, exec-session-scoped, etc. — torn down with the request.

Forbidden:

- `~/.sockerless/state/images.json` — never written. All 6 cloud backends derive `docker images` from their respective cloud registries.
- Backend-side databases, KV stores, message queues for state.
- Tags written by sockerless that store secrets or state-snapshots beyond identity (`sockerless-managed`, `sockerless-container-id`, `sockerless-name`, `sockerless-pod`, `sockerless:network`, `sockerless:network-id`, `sockerless-instance` — these are identity/discovery only).

---

## Podman pods on FaaS backends — supervisor-in-overlay

FaaS backends (`lambda`, `gcf`, `azf`) all back a Function with a single Linux container. There is no first-class "multiple containers per function" primitive on any of the three clouds. To support podman pods sockerless layers all pod containers' rootfs into one image and runs each as a child process of a small supervisor (the overlay bootstrap as PID 1 of the function's Linux container).

**Podman pod namespace defaults — what's actually shared.**

| Namespace | Podman pod default | Single-Linux-container reality |
|---|---|---|
| `net` | **shared** across pod | shared (single netns of the function's container) — ✅ matches |
| `ipc` | **shared** across pod | shared (single IPC ns) — ✅ matches |
| `uts` | **shared** across pod (hostname) | shared (single UTS ns) — ✅ matches |
| `mount` | **isolated per container** | shared by default — ❌ DIFFERS from podman default |
| `pid` | **isolated per container** (unless `--share=pid`) | shared by default — ❌ DIFFERS from podman default |
| `user` | **isolated per container** (when userns enabled) | shared (no userns) — ❌ DIFFERS |
| `cgroup` | **isolated per container** | shared — ❌ DIFFERS |

**Where sockerless can recover the per-container isolation.** Linux's `unshare(CLONE_NEWNS|CLONE_NEWPID)` lets a privileged process give a child its own mount + PID namespace while staying in the parent's net+IPC+UTS namespaces — exactly what podman does. This requires `CAP_SYS_ADMIN` inside the function's container.

| Backend | `CAP_SYS_ADMIN` available? | mount/pid isolation per pod container? |
|---|---|---|
| `lambda` (image-mode) | No (Lambda execution environment drops most capabilities; `unshare` returns EPERM) | ❌ — pod containers share mount + PID ns of the function's Linux container |
| `gcf` (Cloud Run Functions Gen2 = Cloud Run Service backing) | No (default Cloud Run sandbox is non-privileged; `unshare(CLONE_NEWNS)` fails with EPERM unless the operator opts into Cloud Run's `executionEnvironment=gen2` + a custom run-time security policy that explicitly grants the cap, which Google does not currently expose via the Functions API) | ❌ same as lambda |
| `azf` (Premium / Flex Consumption / App Service Linux container plans) | Same constraint as Cloud Run — Azure doesn't grant CAP_SYS_ADMIN to Function App containers by default | ❌ same as lambda |

**Honest mapping.** For pods on FaaS, sockerless delivers:

- ✅ **Pod-level networking** (`localhost:PORT` between pod containers) — matches podman default.
- ✅ **Shared IPC** (`/dev/shm`, SysV IPC) — matches podman default.
- ✅ **Shared UTS** (single hostname, settable by any pod container) — matches podman default.
- ⚠️ **Mount-ns approximation via chroot per child.** `chroot` + per-container subdir under `/containers/<name>` gives path-based isolation for the binaries each container looks up via `PATH`, but is NOT a real mount-ns. Two pod containers writing the same absolute path inside their chroots stay isolated; two pod containers writing the same path OUTSIDE the chroot (e.g. both opening `/proc/self/cgroup`, both touching `/dev/null`) see the same file. Surfaces in `docker inspect <pod-member>.HostConfig.MountNamespaceMode = "shared-degraded"` so operators can detect.
- ❌ **PID-ns is shared, not isolated.** A pod container running `ps -ef` sees every other pod container's processes (and the supervisor). `kill -9 <peer-pid>` reaches across containers. This matches podman's `--share=pid` mode but NOT podman's default. Sockerless surfaces this in `docker inspect` as `HostConfig.PidMode = "shared-degraded"`.
- ❌ **User-ns + cgroup-ns are shared** — degradation symmetric with PID. Surfaced via `docker inspect`.

**Why we don't fake the isolation.** Per the project's no-fakes / no-fallbacks rule: when the cloud sandbox blocks `unshare(CLONE_NEWNS|CLONE_NEWPID)`, we don't pretend isolation exists. Operators relying on per-container mount/PID isolation get a clear `inspect` field telling them the truth, plus a startup warning in the function's Cloud Logging stream:

```
sockerless-<backend>-bootstrap: WARNING — pod uses degraded namespace isolation:
  mount-ns: shared (chroot only — would require CAP_SYS_ADMIN)
  pid-ns:   shared (would require CAP_SYS_ADMIN)
  net-ns:   shared per podman default ✓
  ipc-ns:   shared per podman default ✓
  uts-ns:   shared per podman default ✓
```

**Workloads this works for.** GitLab/GitHub runner jobs that use `services:` (e.g. a postgres sidecar): the runner's main container reaches the postgres container via `localhost:5432` (shared net), and rarely cares about mount-ns isolation across the pair. CI workloads are the primary target.

**Workloads this doesn't work for.** Pods where one container does `mount`/`pivot_root`/`unshare` for its own private filesystem layout (e.g. running a containerized container runtime inside a pod). The chroot approximation isn't enough; operators get a `NotImpl` from the bootstrap when the user image's ENTRYPOINT actually fails on `mount`.

**Overlay shape.** For pod `mypod` containing `[web: nginx:alpine, sidecar: alpine sleep 3600]`, sockerless's overlay-build merges both rootfs into one image with per-container subdirectories:

```
FROM <merge-base>
COPY --from=nginx:alpine / /containers/web/
COPY --from=alpine / /containers/sidecar/
COPY sockerless-<backend>-bootstrap /opt/sockerless/bootstrap
ENV SOCKERLESS_POD_CONTAINERS=<base64-JSON [
   {name:"web",     root:"/containers/web",     entrypoint:["nginx","-g","daemon off;"], cmd:[]},
   {name:"sidecar", root:"/containers/sidecar", entrypoint:[],                            cmd:["sleep","3600"]}]>
ENTRYPOINT ["/opt/sockerless/bootstrap"]
```

The bootstrap parses `SOCKERLESS_POD_CONTAINERS` and for each entry: forks; in the child, `chroot` into the per-container root, `chdir` to "/", exec entrypoint+cmd. Stdout/stderr tee to per-container ring buffer + the supervisor's stdout (so Cloud Logging captures with per-container labels via a `[<container-name>]` line prefix). Per-container `docker exec mypod-web ...` re-enters via the bootstrap's child-PID registry, which forks a new chroot'd process.

**Limitations.**

- **No per-container restart.** Restarting one pod container would require killing its child without disturbing peers, and its filesystem state inside the merged rootfs persists. `docker container restart <pod-member>` returns NotImpl with a clear message; operators use `podman pod restart` (deletes + recreates the function).
- **Image overlap on COPY.** If two pod members ship conflicting versions of the same path (`/etc/nginx.conf` from both a base and an override), the second `COPY --from=` wins. Sockerless detects collisions at build time and fails the pod create with an actionable error.
- **Bake-time merge cost.** Pod overlay = base + N user images → larger image than single-container overlays. Built once per pod-content-hash, cached in AR/ECR/ACR.

**Operator escape hatch for genuinely-isolated pods.** When operators need full per-container mount/PID isolation (e.g. running container-of-containers workloads, or strict security tenancy), the supervisor pattern is insufficient. Sockerless surfaces this via `inspect.HostConfig.MountNamespaceMode = "shared-degraded"` — operators detecting that field can fall through to a different sockerless backend (`cloudrun-jobs`, `aca`) that does allow per-container Linux containers (one Cloud Run Job per pod container, networked via VPC connector + Cloud DNS). FaaS pods are explicitly the lower-isolation tier.

**Stateless invariant.** Pod metadata round-trips via `SOCKERLESS_POD_CONTAINERS` env var on the function (or app-setting on AZF). `docker pod ps` queries the cloud for sockerless-managed functions with `sockerless_pod=<name>` label and reconstructs the pod from the env var. Per-container exit codes ride in `Store.LogBuffers` keyed by `<containerID>` — the bootstrap stamps each subprocess's exit code into its own buffer, sockerless reads them via the existing FaaS LogBuffers path.

---

## Stateless image cache + Function/Site reuse pool (FaaS backends)

FaaS backends (`lambda`, `gcf`, `azf`) wrap the user image with a sockerless bootstrap so the cloud's invocation runtime (Lambda Runtime API, HTTP, etc.) can drive a one-shot subprocess execution of the user CMD. Two independent caches live entirely in cloud-side resources to keep the stateless invariant ironclad while still meeting the amortized-startup goal (second-touch of a known image runs at the cloud's normal warm cold-start, not at first-touch build cost).

**1. Content-addressed overlay image cache.**

| Element | Definition |
|---|---|
| Content tag | `<contentTag> = sha256(resolvedUserImage, bootstrapBinaryPath, userEntrypoint, userCmd, userWorkdir)[:16]` |
| Image URI | `lambda`: `<account>.dkr.ecr.<region>.amazonaws.com/sockerless-overlay:<contentTag>`. `gcf`: `<region>-docker.pkg.dev/<project>/sockerless-overlay/gcf:<contentTag>`. `azf`: `<acr>.azurecr.io/sockerless-overlay/azf:<contentTag>`. |
| Cache check | `ECR.DescribeImages(imageIds=[contentTag])` / `ArtifactRegistry.GetDockerImage(URI)` / `ContainerRegistry.GetTag` — 200 ⇒ skip build, 404 ⇒ build via `core.CloudBuildService` and push. |
| Cache eviction | Operator-controlled lifecycle policy on the registry (TTL on untagged + per-image-tag retention). Sockerless does not delete overlay images — multiple Functions/Sites can reference the same content tag. |

**2. Stateless reuse pool keyed on overlay content hash.**

The cloud Function (gcf) / Function App (azf) / Lambda function (lambda) is the unit of pool capacity. Each is labeled with the overlay-hash it was built for and an allocation marker (the container-id currently using it, or empty for "free").

| Label / tag | Semantics | Read at |
|---|---|---|
| `sockerless-managed=true` | Marks resources sockerless owns | every list call |
| `sockerless-overlay-hash=<contentTag>` | Groups reusable resources by image content | every list call |
| `sockerless-allocation=<containerID>` | "In use" marker. Empty/absent ⇒ free, in pool | every list call |

**Claim sequence (`docker run`):** list resources matching `sockerless_managed=true AND sockerless_overlay_hash=<contentTag>`; pick first with empty allocation; atomic CAS via `Update*({allocation: <containerID>}, etag=<currentEtag>)`. Etag mismatch ⇒ another sockerless instance won; loop. If no free resource exists, build overlay (cache check above), create new resource (lambda: `CreateFunction`; gcf: `CreateFunction(stub-source) + UpdateService(image=overlay)`; azf: `WebApps.CreateOrUpdate(linuxFxVersion=DOCKER|<overlay>)`).

**Release sequence (`docker rm`):** find the container's claimed resource via `sockerless_allocation=<containerID>`; count free resources for this overlay-hash; if count `>= SOCKERLESS_<BACKEND>_POOL_MAX` (default 10) ⇒ delete; otherwise clear the allocation label so a future `docker run` reuses it.

**Multi-instance safety:** etag-conditional updates are documented compare-and-swap primitives on every cloud (Cloud Functions `Function.etag`, Lambda function tags via `RevisionId` analog, Azure resource ETag). Two sockerless backends can't both claim the same free resource.

**Restart safety:** all cache and pool state lives in cloud resource labels. A sockerless backend restart with empty in-memory state derives the full pool by listing tagged resources — no on-disk JSON, no in-memory `StateStore` of pool entries.

**Operator knobs:**
- `SOCKERLESS_<BACKEND>_POOL_MAX` (default `10`): cap on free resources kept warm per overlay-hash. Set to `0` to disable pooling (every container creates+deletes a fresh resource — preserves the shape but eliminates amortization).
- Pool reuse never crosses overlay-hashes. Different user-image / cmd / entrypoint combinations get distinct pools.

**Stateless invariant — implementation:** FaaS backends have zero per-container `StateStore` entries. Container/pool state is the cloud-side label set, queried fresh on every operation. Older `s.<Backend> *StateStore[<Backend>State]` caches that held `FunctionName`/`InvocationResult` are gone in the pool model — concurrent allocation changes across sockerless instances make a transient cache stale, and the cloud query is the only safe answer.

---

## Recovery contract

After a backend restart with no in-memory state and no on-disk JSON:

- `docker ps -a` returns the same containers as before.
- `docker network ls` returns the same user-defined networks as before.
- `docker images` returns the same images as before (queried from the cloud registry).
- `docker stop <id>` works on any previously-created container.
- `docker exec <id>` works on any previously-running container (when the backend supports exec).
- `docker pod ps` returns the same pods as before (for backends that map pods to multi-container task defs).

A backend that fails any of these contracts is in violation of the stateless-backend invariant.

---

## Acceptable gaps

The "no fakes / no fallbacks" principle treats every functional gap as a bug by default — every gap lands in [BUGS.md](../BUGS.md) until it ships a real fix. The list below is the narrow set of gaps the maintainers have explicitly classified as **acceptable** (for now): each one is documented here, returns `NotImplementedError` with a clear message, and is excluded from the open-bugs scoreboard. Anything not on this list is still a bug. Adding to this list requires explicit maintainer sign-off, not implementor judgment.

| Gap | Backend(s) | Why acceptable |
|-----|------------|----------------|
| `docker commit` | ecs | Fargate exposes no host filesystem to snapshot from, and ECS doesn't run a sockerless bootstrap that could capture a rootfs diff over SSM exec. The other backends (Lambda/CR/ACA/GCF/AZF) implement commit via the reverse-agent — ECS is the one platform where the architectural prerequisite simply isn't there. Operators wanting commit-style workflows on ECS should build images via `docker build` + `docker push` to ECR instead. |
| `docker pause` / `docker unpause` | ecs (without bootstrap convention) | ECS pause/unpause runs `kill -SIGSTOP $(cat /tmp/.sockerless-mainpid)` over SSM exec — it works only when the user image cooperates by writing the main PID to that file. Sockerless can't insert a bootstrap into ECS user images (we run the operator's image as-is), so the no-bootstrap case returns `NotImplementedError` and that's accepted. With the convention in place the path works. Other backends (Lambda/CR/ACA/GCF/AZF) ship the convention in their bootstrap by default, so pause works there. |
| `ContainerResize` / `ExecResize` (TTY size events / `SIGWINCH`) | all clouds | Cloud platforms don't propagate window-size events through to the container. Returning success would be a fake; the only honest answer is `NotImplementedError`. Affects only interactive TTY sessions where the user resizes the terminal mid-session. |
| `docker image save` | all clouds | Cloud registries don't serve a single multi-blob tarball. Implementing `save` would require pulling the manifest + every layer blob and retaring locally — substantial work for an air-gapped-export use case that operators can replicate with `crane export` or `skopeo copy` against the registry directly. |
| `docker image search` | all clouds | Docker Hub's search API isn't reachable through ECR / Artifact Registry / ACR. Cloud registries have no equivalent free-text search across public images. Operators looking for images should use Docker Hub's web UI or `crane catalog` / `oras discover`. |
| `docker stats` (streaming) | all clouds | CloudWatch / Cloud Monitoring / Log Analytics surface metrics with 30–60 s+ lag, so a "streaming" stats response would be a polling reskin that misleads callers into thinking it's real-time. One-shot `docker stats --no-stream` stays ⚠ (returns the latest available aggregate), but `docker stats` (the streaming form) returns `NotImplementedError`. |
| `docker container top` | every backend without an exec path | `top` (which translates to running `ps aux` inside the container) only works when sockerless can exec into the container — the reverse-agent for FaaS+CR+ACA, SSM for ECS. When neither is registered the call returns `NotImplementedError` rather than an empty / fabricated process list. (ECS does have an exec path via SSM and is `⚠ via SSM` in the matrix, not an accepted gap — only the FaaS-without-agent case is.) |
| `docker container export` | every backend without an exec path | Same constraint as `top` — `export` requires "tar the entire FS over exec" via SSM (ECS) or the reverse-agent (FaaS+CR+ACA). When the exec path is available, export works (slowly); when it isn't, `NotImplementedError` instead of an empty tar. Overlay-rootfs mode (`SOCKERLESS_OVERLAY_ROOTFS=1`) gives a faster implementation that reads from the upper-dir directly. |
| `docker rename` semantics | all clouds | Cloud resources (ECS task ARN, Cloud Run job name, ACA app name, Lambda function name, etc.) are immutable. Sockerless updates the local `Container.Name` field and re-stamps the `sockerless-name` tag on the cloud resource, so `docker inspect` reflects the new name — but the cloud resource's *own* name doesn't change. This is a documented semantic divergence, not a partial implementation: the rename is real for sockerless-internal lookups; it does not propagate to the cloud's resource naming.|

## Stateless invariant — reference implementation

Summary of how each backend honours the stateless contract pinned down by the [Recovery contract](#recovery-contract):

- **`Store.Images` is purely an in-process cache.** All 6 cloud backends implement `CloudImageLister.ListImages`: ECS + Lambda via ECR `DescribeRepositories`+`DescribeImages`; Cloud Run + GCF via shared `core.OCIListImages` against `<region>-docker.pkg.dev` with `ARAuthProvider` token; ACA + AZF via `core.OCIListImages` against the configured ACR with `ACRAuthProvider` token. `BaseServer.ImageList` merges cache + cloud, deduped by ID.
- **Pod state derives from cloud tags.** `core.CloudPodLister` interface + `BaseServer.PodList` merging cache + cloud. ECS groups tasks by `sockerless-pod` tag. Cloud Run + ACA pod listing works on the Service/App paths. GCF + AZF don't support pods.
- **`resolve*State` cache+cloud-fallback helpers** landed across 4 backends (ECS, Lambda, Cloud Run, ACA). Every cloud-state-dependent callsite (Stop, Kill, Remove, Restart, Wait, Logs, ExecCreate, cloudExecStart, etc.) goes through them.
- **`resolveNetworkState` cache+cloud-fallback helpers** in ECS, Cloud Run, ACA. Cloud Map namespaces tagged with `sockerless:network-id` at create time. Lambda + GCF + AZF don't have user-defined cloud networks.
