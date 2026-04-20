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
| Container | Cloud Run **Service** (post-Phase 87) or **Job execution** (current) | service name / job execution id | label `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | Cloud Run Service with multi-container revision (sidecars) — Phase 87 deliverable | revision ref + sidecar container names | + `sockerless-pod=<name>` |
| Image | Artifact Registry / GCR | `<region>-docker.pkg.dev/<project>/<repo>/<image>:<tag>` | (registry-managed) |
| Network | Cloud DNS private managed zone backing per-network DNS (post-Phase 87 also needs VPC connector + internal-ingress Service IP) | zone name (sanitized from network name) | label `sockerless:network=<name>` |
| Volume | Cloud Storage Fuse mount (per-revision config) | bucket/prefix | — |
| Exec instance | Not natively supported by Cloud Run Services / Jobs. Must go through the agent overlay (same pattern as Lambda) — Phase 87 deliverable. | — | — |

**State derivation:**

- `docker ps -a` → `Services.List` (or `Jobs.List` + `Executions.List` for legacy), filter by label `sockerless-managed=true`, project to `api.Container`.
- `docker network ls` → `ManagedZones.List` filter by label `sockerless:network=*`.
- `docker images` → Artifact Registry `Images.List`.

### Azure Container Apps (backend `aca`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | ACA **App** with internal ingress (post-Phase 88) or **Job execution** (current) | app name / job execution id | tag `sockerless-managed=true`, `sockerless-container-id=<id>`, `sockerless-name=<name>` |
| Pod | ACA App with multi-container template (sidecars) — Phase 88 deliverable | app name + sidecar container names | + `sockerless-pod=<name>` |
| Image | ACR | `<acrName>.azurecr.io/<repo>:<tag>` | (registry-managed) |
| Network | Azure Private DNS Zone backing per-network DNS (per-network NSG already exists) | zone name + NSG id | tag `sockerless:network=<name>` |
| Volume | Azure Files share via ACA volumes | mount config | — |
| Exec instance | ACA exec console (different proto from SSM). Phase 88 deliverable. | — | — |

**State derivation:**

- `docker ps -a` → `ContainerApps.ListByResourceGroup` (or `Jobs.List` for legacy), filter by tag `sockerless-managed=true`.
- `docker network ls` → `PrivateZones.ListByResourceGroup` filter by tag `sockerless:network=*`.
- `docker images` → ACR `RegistryClient.NewListImportImagesPager`.

### GCP Cloud Run Functions (backend `cloudrun-functions` / `gcf`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Cloud Function (gen 2) | function name | label `sockerless-managed=true`, `sockerless-container-id=<id>` |
| Pod | Not supported (one function = one container) | — | — |
| Image | Artifact Registry | (same as Cloud Run) | — |
| Network | Not supported natively (Cloud Functions have egress VPC config but no peer discovery) | — | — |

### Azure Functions (backend `azure-functions` / `azf`)

| Docker concept | Cloud resource | Identifier(s) | Tag(s) for discovery |
|---|---|---|---|
| Container | Function App | function-app name | tag `sockerless-managed=true`, `sockerless-container-id=<id>` |
| Pod | Not supported | — | — |
| Image | ACR | (same as ACA) | — |
| Network | Not supported natively (VNet integration is for outbound only) | — | — |

### Local Docker (backend `docker`)

The `docker` backend delegates to a local Docker daemon, which is itself the source of truth — no extra mapping needed. State is the running daemon.

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
