# Docker API Compatibility Matrix

> See also: [ARCHITECTURE.md](ARCHITECTURE.md), [STATUS.md](STATUS.md)

This document maps Docker/Podman CLI commands to their REST API endpoints and the cloud-specific services each Sockerless backend uses to implement them.

## Backends

| Short Name | Package | Cloud Provider | Container Service |
|------------|---------|----------------|-------------------|
| **Core** | `backends/core` | Shared | Docker API handlers, typed driver framework, and shared cloud helpers |
| **Docker** | `backends/docker` | Local | Real Docker Engine passthrough |
| **ECS** | `backends/ecs` | AWS | ECS Fargate tasks |
| **CloudRun** | `backends/cloudrun` | GCP | Cloud Run Jobs and Services |
| **ACA** | `backends/aca` | Azure | Container Apps Jobs and Apps |
| **Lambda** | `backends/lambda` | AWS | Lambda functions (FaaS) |
| **GCF** | `backends/cloudrun-functions` | GCP | Cloud Run Functions 2nd gen (FaaS) |
| **AZF** | `backends/azure-functions` | Azure | Azure Functions (FaaS) |

## Status Legend

| Symbol | Meaning |
|--------|---------|
| ✅ | Fully implemented |
| ⚠️ | Partial (see notes) |
| ❌ | Not implemented / not applicable |

---

## Container Lifecycle

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker create` | `POST /containers/create` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker start` | `POST /containers/{id}/start` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker stop` | `POST /containers/{id}/stop` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker restart` | `POST /containers/{id}/restart` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker kill` | `POST /containers/{id}/kill` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker rm` | `DELETE /containers/{id}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker ps` | `GET /containers/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker inspect` | `GET /containers/{id}/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker wait` | `POST /containers/{id}/wait` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker rename` | `POST /containers/{id}/rename` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker top` | `GET /containers/{id}/top` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker stats` | `GET /containers/{id}/stats` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker update` | `POST /containers/{id}/update` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker diff` | `GET /containers/{id}/changes` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker export` | `GET /containers/{id}/export` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker resize` | `POST /containers/{id}/resize` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker pause` | `POST /containers/{id}/pause` | ✅ | ✅ | ❌ | ⚠️ agent | ⚠️ agent | ⚠️ agent | ⚠️ agent | ⚠️ agent |
| `docker unpause` | `POST /containers/{id}/unpause` | ✅ | ✅ | ❌ | ⚠️ agent | ⚠️ agent | ⚠️ agent | ⚠️ agent | ⚠️ agent |
| `docker container prune` | `POST /containers/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Cloud Service Mapping — Container Lifecycle

| Operation | ECS (AWS) | CloudRun (GCP) | ACA (Azure) | Lambda (AWS) | GCF (GCP) | AZF (Azure) |
|-----------|-----------|----------------|-------------|--------------|-----------|-------------|
| create | Pending create until task registration/start | Pending create until Job/Service materialization | Pending create until Job/App materialization | Pending create until Function materialization | Pending create until Function/Service materialization | Pending create until Function App materialization |
| start | `ecs:RunTask` | `run.jobs.run` or Service revision start | `containerApps.jobs.start` or App revision start | `lambda:Invoke` + reverse-agent wait | Function URL invoke + reverse-agent wait | HTTP trigger invoke + reverse-agent wait |
| stop | `ecs:StopTask` | cancel execution or stop Service-backed path | stop execution or App-backed path | reverse-agent/platform result | reverse-agent/platform result | reverse-agent/platform result |
| kill | `ecs:StopTask` | cancel execution or stop Service-backed path | stop execution or App-backed path | reverse-agent/platform result | reverse-agent/platform result | reverse-agent/platform result |
| remove | stop + task definition cleanup | delete managed Job/Service resources | delete managed Job/App resources | `lambda:DeleteFunction` | `functions.delete` + Service cleanup | `webApps.Delete` |

---

## Logs

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker logs` | `GET /containers/{id}/logs` | ✅ | ✅ | ✅ | ✅ | ✅ | ⚠️ | ⚠️ | ⚠️ |
| `docker logs -f` | (follow mode) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker logs --tail N` | (tail filter) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker logs --since/--until` | (time filter) | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

All cloud backends use `core.StreamCloudLogs` with a `CloudLogFetchFunc` closure. FaaS backends check in-memory `LogBuffers` first (captured invocation output), then fall back to cloud logging.

### Cloud Service Mapping — Logs

| Backend | Logging Service | Query Method | Follow Mode |
|---------|----------------|--------------|-------------|
| Core | In-memory log buffers | Direct byte read | Log subscriber channel |
| Docker | Docker Engine | Docker SDK `ContainerLogs` | Native follow |
| ECS | CloudWatch Logs | `GetLogEvents` + NextToken cursor | 1s poll with NextToken |
| Lambda | CloudWatch Logs | `DescribeLogStreams` + `GetLogEvents` | 1s poll with NextToken |
| CloudRun | Cloud Logging | `logadmin.Entries()` with filter | 1s poll with timestamp cursor |
| GCF | Cloud Logging | `logadmin.Entries()` with filter | 1s poll with timestamp cursor |
| ACA | Azure Monitor | KQL `ContainerAppConsoleLogs_CL` | 1s poll with timestamp cursor |
| AZF | Azure Monitor | KQL `AppTraces` | 1s poll with timestamp cursor |

---

## Exec and Attach

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker exec` (create) | `POST /containers/{id}/exec` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker exec` (start) | `POST /exec/{id}/start` | ✅ | ✅ | ✅ | ⚠️ | ✅ | ✅ | ✅ | ✅ |
| `docker exec` (inspect) | `GET /exec/{id}/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker exec` (resize) | `POST /exec/{id}/resize` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker attach` | `POST /containers/{id}/attach` | ✅ | ✅ | ✅ | ⚠️ | ✅ | ✅ | ✅ | ✅ |

Exec/attach has exactly one implementation path per backend. ECS uses its cloud-native SSM ExecuteCommand path. FaaS-style and runner Service/App paths use the registered reverse-agent WebSocket. If the required session is missing, the backend returns an explicit error.

### Cloud Service Mapping — Exec/Attach

| Backend | Agent Path | Cloud-Native Path | Cloud API |
|---------|------------|-------------------|-----------|
| Core | Shared typed driver slot | N/A | Backend-selected implementation |
| Docker | N/A | Docker SDK | `ContainerExecCreate` + `ContainerExecAttach` |
| ECS | N/A (no in-container bootstrap) | ECS ExecuteCommand | `ecs:ExecuteCommand` → SSM WebSocket session |
| CloudRun | ReverseAgentExecDriver | Not supported | Cloud Run Services path; Jobs have no exec API |
| ACA | ReverseAgentExecDriver | Not supported for runner Apps | Container Apps Apps path |
| Lambda | ReverseAgentExecDriver | N/A | FaaS — no persistent process |
| GCF | ReverseAgentExecDriver | N/A | FaaS — no persistent process |
| AZF | ReverseAgentExecDriver | N/A | FaaS — no persistent process |

The agent path (when present) is wired through the typed `ExecDriver` slot in `TypedDriverSet` via `core.WrapLegacyExec` wrapping `ReverseAgentExecDriver`. ECS uses SSM ExecuteCommand directly through its typed Exec adapter (`backends/ecs/typed_drivers.go`).

---

## Container Archive (Copy Files)

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker cp` (to) | `PUT /containers/{id}/archive` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker cp` (stat) | `HEAD /containers/{id}/archive` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker cp` (from) | `GET /containers/{id}/archive` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

All backends use the `FilesystemDriver` (agent-based staging directories for cloud backends).

---

## Images

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker pull` | `POST /images/create` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker images` | `GET /images/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker inspect` (image) | `GET /images/{name}/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker rmi` | `DELETE /images/{name}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker tag` | `POST /images/{name}/tag` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker history` | `GET /images/{name}/history` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker push` | `POST /images/{name}/push` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker save` | `GET /images/get` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker load` | `POST /images/load` | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `docker search` | `GET /images/search` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker import` | `POST /images/create?fromSrc=` | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `docker image prune` | `POST /images/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Cloud Service Mapping — Images

| Backend | Registry | Auth | Push Target |
|---------|----------|------|-------------|
| Core | In-memory store | N/A | In-memory |
| Docker | Docker Hub / any | Docker SDK | Docker SDK `ImagePush` |
| ECS | ECR | `ecr:GetAuthorizationToken` | ECR repository |
| Lambda | ECR | `ecr:GetAuthorizationToken` | ECR repository |
| CloudRun | Artifact Registry | `gcloud auth` token | Artifact Registry |
| GCF | Artifact Registry | `gcloud auth` token | Artifact Registry |
| ACA | ACR | `acr.ListCredentials` | Azure Container Registry |
| AZF | ACR | `acr.ListCredentials` | Azure Container Registry |

---

## Networks

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker network create` | `POST /networks/create` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network ls` | `GET /networks` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network inspect` | `GET /networks/{id}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network connect` | `POST /networks/{id}/connect` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network disconnect` | `POST /networks/{id}/disconnect` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network rm` | `DELETE /networks/{id}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker network prune` | `POST /networks/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Cloud Service Mapping — Networks

| Backend | Network Isolation | Service Used | What Happens |
|---------|-------------------|--------------|--------------|
| Core | Shared network bookkeeping | In-memory metadata for non-cloud tests | Tracks memberships for backends that do not override |
| Core (Linux) | LinuxNetworkDriver | Network namespaces + veth pairs | Real L2 isolation |
| Docker | Docker Engine | Docker SDK `NetworkCreate/Connect` | Real Docker networking |
| ECS | VPC Security Groups | `ec2:CreateSecurityGroup`, `ec2:AuthorizeSecurityGroupIngress` | Per-network SG, self-referencing ingress rule |
| CloudRun | Cloud DNS private zones | `dns.managedZones.create`, `dns.rrsets.create` | DNS-based name resolution per network |
| ACA | Environment networking | Container Apps Environment shared VNet | Internal DNS + NSG rule tracking |
| Lambda | Limited | VPC config / service mesh when configured | No native Docker peer network |
| GCF | Limited | Underlying Cloud Run Service materialization | No native Docker bridge |
| AZF | Limited | Private DNS / Function App networking when configured | No native Docker bridge |

---

## Service Discovery

When containers connect to a Docker network, service discovery enables them to resolve each other by hostname. Each cloud maps this to its native DNS/discovery service.

| Backend | Service | Registration | Resolution |
|---------|---------|-------------|------------|
| Core | In-memory metadata | Shared network driver tracks endpoints | Direct lookup for non-cloud tests |
| Docker | Docker DNS | Docker Engine internal DNS | Container name resolution |
| ECS | AWS Cloud Map | `servicediscovery:RegisterInstance` | `servicediscovery:DiscoverInstances` |
| CloudRun | Cloud DNS | `dns.rrsets.create` (A record) | `dns.rrsets.list` lookup |
| ACA | Azure Private DNS | Environment internal DNS + registry | Hostname-to-IP mapping |
| Lambda | Limited | Service-mesh/cloud DNS only when configured | No native Docker DNS |
| GCF | Underlying Service materialization | Multi-container Service path | Service-local resolution |
| AZF | Limited | Private DNS only when configured | No native Docker DNS |

---

## Storage / Volumes

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker volume create` | `POST /volumes/create` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker volume ls` | `GET /volumes` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker volume inspect` | `GET /volumes/{name}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker volume rm` | `DELETE /volumes/{name}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker volume prune` | `POST /volumes/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Cloud Service Mapping — Storage

| Backend | Volume Type | Cloud Service | Mount Type |
|---------|------------|---------------|------------|
| Core | Local tmpdir | `os.MkdirTemp` | Bind mount |
| Docker | Docker volumes | Docker SDK `VolumeCreate` | Docker volume |
| ECS | EFS | `efs:CreateFileSystem`, `efs:CreateAccessPoint` | EFS volume in task def |
| ECS | EBS | `ec2:CreateVolume` | EBS volume attachment |
| CloudRun | GCS FUSE | `storage.BucketHandle` | GCS FUSE mount |
| CloudRun | Persistent Disk | Compute Engine PD | Block storage |
| ACA | Azure Files | `storage.FileShares` | Azure Files mount |
| ACA | Azure Disk | Managed Disk | Block storage |
| Lambda | EFS | `efs:CreateAccessPoint` / configured EFS | EFS mount in function config |
| GCF | GCS-backed storage | Cloud Run Service volume / `gcs-sync` | GCS-backed workspace |
| AZF | Azure Files | Storage file shares | Azure Files mount |

---

## Build

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker build` | `POST /build` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker builder prune` | `POST /build/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker commit` | `POST /commit` | ✅ | ✅ | ❌ | ⚠️ opt-in | ⚠️ opt-in | ⚠️ opt-in | ⚠️ opt-in | ⚠️ opt-in |

- Core: shared Dockerfile parsing/build response helpers for tests and explicit development paths
- Docker: Proxies to Docker Engine build API
- Cloud/FaaS: Uses the configured cloud builder and registry path where implemented; unsupported build modes fail explicitly

---

## System

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker info` | `GET /info` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker version` | `GET /version` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker ping` | `GET/HEAD/POST /_ping` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker events` | `GET /events` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker system df` | `GET /system/df` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker login` | `POST /auth` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

---

## Pod API (Podman-compatible)

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `podman pod create` | `POST /libpod/pods/create` | ✅ | ✅ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| `podman pod list` | `GET /libpod/pods/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod inspect` | `GET /libpod/pods/{name}/json` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod start` | `POST /libpod/pods/{name}/start` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod stop` | `POST /libpod/pods/{name}/stop` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod kill` | `POST /libpod/pods/{name}/kill` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod rm` | `DELETE /libpod/pods/{name}` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Cloud Service Mapping — Pods

| Backend | Multi-Container Pod | How It Works |
|---------|-------------------|--------------|
| Docker | ✅ | `sockerless-pod` label on local Docker containers; PodList merges Store.Pods with the label filter so restarts don't drop pods. |
| ECS | ✅ | Multiple containers in one ECS task definition. |
| CloudRun | ✅ | Multiple containers in one Cloud Run Job/Service. |
| ACA | ✅ | Multiple containers in one Container Apps Job/App. |
| Lambda/GCF/AZF | ❌ | FaaS backends reject multi-container pods (platform is 1-container-per-function). |

---

## Unsupported Docker API Endpoints

| Category | Endpoints | Reason |
|----------|-----------|--------|
| Swarm | `POST /swarm/*`, `GET /swarm` | Sockerless uses cloud orchestration, not Swarm |
| Nodes | `GET /nodes` | No node concept in serverless |
| Services | `GET /services` | Use pods or cloud-native services |
| Tasks | `GET /tasks` | Swarm tasks not applicable |
| Secrets | `GET /secrets` | Use cloud secrets managers |
| Configs | `GET /configs` | Use cloud configuration services |
| Plugins | `GET /plugins` | Driver interfaces replace plugins |
| Session | `POST /session` | BuildKit sessions not implemented |
| Distribution | `GET /distribution/*` | Use cloud registry APIs |

---

## Driver Architecture

Sockerless dispatches every Docker API operation through `core.TypedDriverSet` — 13 typed driver dimensions (Exec, Attach, Logs, Signal, ProcList, FSDiff, FSRead, FSWrite, FSExport, Commit, Build, Stats, Registry). Each backend constructs a `TypedDriverSet` at startup; cloud backends populate slots with cloud-native typed drivers (CloudWatch / Cloud Logging / Azure Monitor for Logs+Attach; SSM for ECS exec/fs/signal; reverse-agent for FaaS+CR+ACA exec/fs/commit/proclist). The full per-backend default-driver matrix lives in [specs/DRIVERS.md](specs/DRIVERS.md).

The narrow `core.DriverSet` (Exec/Filesystem/Stream/Network) predates the typed framework and remains for the Network driver chain (which has platform-specific Linux netns logic that doesn't fit the per-container DriverContext envelope) and as bridge points for the typed framework's default adapters.

| Driver | Interface | Default | Cloud-native overrides |
|--------|-----------|---------|---|
| Exec | `core.ExecDriver` | `WrapLegacyExecStart` (rwc bridge) | `WrapLegacyExec(ReverseAgentExecDriver)` for FaaS+CR+ACA |
| Attach | `core.AttachDriver` | `WrapLegacyContainerAttach` | `NewCloudLogsAttachDriver` for 6 cloud backends |
| Logs | `core.LogsDriver` | `WrapLegacyLogs` | `NewCloudLogsLogsDriver` for 6 cloud backends |
| Signal | `core.SignalDriver` | `WrapLegacyKill` | ECS `ssmSignalDriver` |
| ProcList | `core.ProcListDriver` | `WrapLegacyTop` | ECS `ssmProcListDriver`; FaaS+CR+ACA `NewReverseAgentProcListDriver` |
| FSDiff | `core.FSDiffDriver` | `WrapLegacyChanges` | ECS `ssmFSDiffDriver`; FaaS+CR+ACA `NewReverseAgentFSDiffDriver` |
| FSRead | `core.FSReadDriver` | `WrapLegacyFSRead` | ECS `ssmFSReadDriver`; FaaS+CR+ACA `NewReverseAgentFSReadDriver` |
| FSWrite | `core.FSWriteDriver` | `WrapLegacyFSWrite` | ECS `ssmFSWriteDriver`; FaaS+CR+ACA `NewReverseAgentFSWriteDriver` |
| FSExport | `core.FSExportDriver` | `WrapLegacyFSExport` | ECS `ssmFSExportDriver`; FaaS+CR+ACA `NewReverseAgentFSExportDriver` |
| Commit | `core.CommitDriver` | `WrapLegacyCommit` | FaaS+CR+ACA `NewReverseAgentCommitDriver` |
| Build | `core.BuildDriver` | `WrapLegacyBuild` (delegates to api.Backend.ImageBuild) | per-cloud build service inside the legacy impl |
| Stats | `core.StatsDriver` | `WrapLegacyStats` | (handler builds responses inline) |
| Registry | `core.RegistryDriver` | `WrapLegacyRegistry` (takes typed `core.ImageRef`) | (per-cloud handled inside the legacy impl) |
| Network | `api.NetworkDriver` | Core network bookkeeping driver | `LinuxNetworkDriver` on Linux; cloud networking/discovery via api.Backend |

### Per-Cloud Driver Implementations

| Driver | AWS (ECS) | GCP (CloudRun) | Azure (ACA) |
|--------|-----------|----------------|-------------|
| Exec | ECS ExecuteCommand + SSM | Reverse-agent WebSocket for Services | Reverse-agent WebSocket for Apps |
| CloudNetwork | EC2 Security Groups | Cloud DNS managed zones | Environment networking + NSGs |
| ServiceDiscovery | Cloud Map | Cloud DNS A records | Private DNS + internal registry |
| Storage | EFS + EBS | GCS FUSE + Persistent Disk | Azure Files + Azure Disk |
| Logging | CloudWatch Logs | Cloud Logging (logadmin) | Azure Monitor (KQL) |
| ImageRegistry | ECR | Artifact Registry | ACR |

---

## Backend Compatibility Summary

| Backend   | Exec | Attach | Networks | Volumes | Agent | Runner-Compatible | Runner Tests |
|-----------|:----:|:------:|:--------:|:-------:|:-----:|:-----------------:|:------------:|
| Docker    | Y    | Y      | Y        | Y       | N     | YES               | N/A (passthrough) |
| ECS       | Y    | Y      | Y        | Y       | Y     | YES               | via `SOCKERLESS_ECS_SOCKET` |
| Cloud Run | Y    | Y      | Y        | Y       | Y     | YES               | via `SOCKERLESS_CLOUDRUN_SOCKET` |
| ACA       | Y    | Y      | Y        | Y       | Y     | YES               | via `SOCKERLESS_ACA_SOCKET` |
| Lambda    | Y†   | N      | Y        | Y       | Y†    | YES†              | via `SOCKERLESS_LAMBDA_SOCKET` |
| GCF       | Y†   | N      | Y        | Y       | Y†    | YES†              | via `SOCKERLESS_GCF_SOCKET` |
| AZF       | Y†   | N      | Y        | Y       | Y†    | YES†              | via `SOCKERLESS_AZF_SOCKET` |

†Requires: agent binary in image, `SOCKERLESS_CALLBACK_URL` configured, backend reachable from FaaS network. Subject to function timeout limits.

---

## Test Results

See [STATUS.md](STATUS.md) for overall test counts.

### Simulator Integration Tests

All cloud backends can be tested locally against simulators using `SOCKERLESS_ENDPOINT_URL`. Run the top-level fan-out or invoke a single backend via path delegation:

```bash
make test-integration                       # every backend + sim/cli/sdk test category
make backends/ecs/test-integration          # just ECS
make backends/lambda/test-integration       # just Lambda
make backends/cloudrun/test-integration     # just Cloud Run
make backends/cloudrun-functions/test-integration   # just GCF
make backends/aca/test-integration          # just ACA
make backends/azure-functions/test-integration      # just AZF
```

| Backend   | Sim Tests | PASS | Known Failures |
|-----------|:---------:|:----:|----------------|
| ECS       | 6         | 6    | — |
| Lambda    | 7         | 7    | — |
| Cloud Run | 6         | 6    | — |
| GCF       | 7         | 7    | — |
| ACA       | 6         | 6    | — |
| AZF       | 7         | 7    | — |

### Real Runner Smoke Tests

Unmodified runner binaries tested against Sockerless + simulators via Docker-based smoke tests:

| Runner | Backend | Status |
|--------|---------|:------:|
| `act` (GitHub Actions) | ECS (sim) | PASS |
| `act` (GitHub Actions) | Cloud Run (sim) | PASS |
| `act` (GitHub Actions) | ACA (sim) | PASS |
| `gitlab-runner` (docker executor) | ECS (sim) | PASS |
| `gitlab-runner` (docker executor) | Cloud Run (sim) | PASS |
| `gitlab-runner` (docker executor) | ACA (sim) | PASS |

```bash
make smoke-test-act-all        # act against all 3 simulator backends
make smoke-test-gitlab-all     # gitlab-runner against all 3 simulator backends
```

### Full Terraform Integration Tests

Full terraform modules (`terraform/modules/*`) apply and destroy cleanly against local simulators:

| Backend   | Cloud | Resources | Apply | Destroy | Status |
|-----------|-------|:---------:|:-----:|:-------:|:------:|
| ECS       | AWS   | 21        | PASS  | PASS    | PASS   |
| Lambda    | AWS   | 5         | PASS  | PASS    | PASS   |
| CloudRun  | GCP   | 13        | PASS  | PASS    | PASS   |
| GCF       | GCP   | 7         | PASS  | PASS    | PASS   |
| ACA       | Azure | 18        | PASS  | PASS    | PASS   |
| AZF       | Azure | 11        | PASS  | PASS    | PASS   |

```bash
make tf-int-test-all    # All 6 backends (Docker, ~10-15 min)
make tf-int-test-aws    # ECS + Lambda
make tf-int-test-gcp    # CloudRun + GCF
make tf-int-test-azure  # ACA + AZF
```

---

## Backend Notes

- **Docker**: Passthrough to a real Docker daemon. All capabilities delegated.
- **ECS**: AWS Fargate tasks with agent sidecar for exec/attach. Core's enhanced exec handler dials the agent automatically.
- **Cloud Run**: GCP Cloud Run Jobs with agent injection for exec/attach. Core's enhanced exec handler dials the agent automatically.
- **ACA**: Azure Container Apps Jobs with agent injection for exec/attach. Core's enhanced exec handler dials the agent automatically.
- **Lambda**: AWS Lambda container image functions. When `SOCKERLESS_CALLBACK_URL` is set, the agent is injected into the function entrypoint and dials back to the backend via reverse WebSocket.
- **GCF**: GCP Cloud Run Functions (2nd gen). When `SOCKERLESS_CALLBACK_URL` is set, the agent is injected and connects back via reverse WebSocket.
- **AZF**: Azure Functions with container image support. When `SOCKERLESS_CALLBACK_URL` is set, the agent is injected via `AppCommandLine` and connects back via reverse WebSocket.

### FaaS Agent Architecture (Reverse Connection)

Container backends (ECS, Cloud Run, ACA) use direct agent connections — the backend dials the agent at a known IP. FaaS backends can't accept inbound connections, so they use reverse WebSocket connections:

```
Container backends:  Backend ──dial ws──▶ Agent:9111 (inside container)
FaaS backends:       Agent ──dial ws──▶ Backend /internal/v1/agent/connect
```

The agent runs in "callback mode" (`--callback <url>`), connecting to the backend at startup. The backend stores the connection in an `AgentRegistry` and routes exec sessions through it with session multiplexing.

### FaaS Limitations

1. Agent binary must be present in the container image
2. Function timeout limits apply (Lambda: 15min, GCF 2nd gen: 60min, AZF consumption: 10min)
3. Attach is not supported for FaaS (main process is the function handler)
4. Backend must be network-reachable from the FaaS function via `SOCKERLESS_CALLBACK_URL`
