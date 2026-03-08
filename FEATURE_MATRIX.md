# Docker API Compatibility Matrix

This document maps Docker/Podman CLI commands to their REST API endpoints and the cloud-specific services each Sockerless backend uses to implement them.

## Backends

| Short Name | Package | Cloud Provider | Container Service |
|------------|---------|----------------|-------------------|
| **Core** | `backends/core` | Local | In-memory driver chain (agent/process/synthetic) |
| **Docker** | `backends/docker` | Local | Real Docker Engine passthrough |
| **ECS** | `backends/ecs` | AWS | ECS Fargate tasks |
| **CloudRun** | `backends/cloudrun` | GCP | Cloud Run Jobs |
| **ACA** | `backends/aca` | Azure | Container Apps Jobs |
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
| `docker pause` | `POST /containers/{id}/pause` | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `docker unpause` | `POST /containers/{id}/unpause` | ✅ | ✅ | ❌ | ❌ | ❌ | ❌ | ❌ | ❌ |
| `docker container prune` | `POST /containers/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Cloud Service Mapping — Container Lifecycle

| Operation | ECS (AWS) | CloudRun (GCP) | ACA (Azure) | Lambda (AWS) | GCF (GCP) | AZF (Azure) |
|-----------|-----------|----------------|-------------|--------------|-----------|-------------|
| create | Register TaskDef | Store config | Store config | Store config | Store config | Store config |
| start | `ecs:RunTask` | `run.jobs.executions.run` | `containerApps.jobs.start` | `lambda:Invoke` | `functions.callFunction` | `HTTP POST function` |
| stop | `ecs:StopTask` | `run.jobs.executions.cancel` | `containerApps.jobs.stopExecution` | N/A (stateless) | N/A | N/A |
| kill | `ecs:StopTask` (force) | `run.jobs.executions.cancel` | `containerApps.jobs.stopExecution` | N/A | N/A | N/A |
| remove | `ecs:DeregisterTaskDef` | `run.jobs.delete` | `containerApps.jobs.delete` | `lambda:DeleteFunction` | `functions.delete` | `webApps.Delete` |

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

Exec/attach has two paths per cloud backend:
1. **Agent path**: If an agent is connected to the container, exec proxies through the agent driver chain (works for all backends).
2. **Cloud-native path**: If no agent is connected, uses the cloud provider's native exec API (where available).

### Cloud Service Mapping — Exec/Attach

| Backend | Agent Path | Cloud-Native Path | Cloud API |
|---------|------------|-------------------|-----------|
| Core | AgentExecDriver (forward/reverse agent) | N/A | Local process |
| Docker | N/A | Docker SDK | `ContainerExecCreate` + `ContainerExecAttach` |
| ECS | Agent driver chain | ECS ExecuteCommand | `ecs:ExecuteCommand` → SSM WebSocket session |
| CloudRun | Agent driver chain | Not supported | Cloud Run Jobs have no exec API |
| ACA | Agent driver chain | Container Apps exec | REST `POST .../exec` → WebSocket session |
| Lambda | Reverse agent | N/A | FaaS — no persistent process |
| GCF | Reverse agent | N/A | FaaS — no persistent process |
| AZF | Reverse agent | N/A | FaaS — no persistent process |

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
| Core | SyntheticNetworkDriver (IPAM) | In-memory IP allocation | Assigns IPs, tracks memberships |
| Core (Linux) | LinuxNetworkDriver | Network namespaces + veth pairs | Real L2 isolation |
| Docker | Docker Engine | Docker SDK `NetworkCreate/Connect` | Real Docker networking |
| ECS | VPC Security Groups | `ec2:CreateSecurityGroup`, `ec2:AuthorizeSecurityGroupIngress` | Per-network SG, self-referencing ingress rule |
| CloudRun | Cloud DNS private zones | `dns.managedZones.create`, `dns.rrsets.create` | DNS-based name resolution per network |
| ACA | Environment networking | Container Apps Environment shared VNet | Internal DNS + NSG rule tracking |
| Lambda | N/A | In-memory only | FaaS — no network isolation |
| GCF | N/A | In-memory only | FaaS — no network isolation |
| AZF | N/A | In-memory only | FaaS — no network isolation |

---

## Service Discovery

When containers connect to a Docker network, service discovery enables them to resolve each other by hostname. Each cloud maps this to its native DNS/discovery service.

| Backend | Service | Registration | Resolution |
|---------|---------|-------------|------------|
| Core | In-memory | SyntheticNetworkDriver tracks endpoints | Direct IP lookup |
| Docker | Docker DNS | Docker Engine internal DNS | Container name resolution |
| ECS | AWS Cloud Map | `servicediscovery:RegisterInstance` | `servicediscovery:DiscoverInstances` |
| CloudRun | Cloud DNS | `dns.rrsets.create` (A record) | `dns.rrsets.list` lookup |
| ACA | Azure Private DNS | Environment internal DNS + registry | Hostname-to-IP mapping |
| Lambda | N/A | Not applicable | FaaS — no service discovery |
| GCF | N/A | Not applicable | FaaS — no service discovery |
| AZF | N/A | Not applicable | FaaS — no service discovery |

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
| Lambda | N/A | Ephemeral /tmp only | No persistent volumes |
| GCF | N/A | Ephemeral /tmp only | No persistent volumes |
| AZF | N/A | Ephemeral /tmp only | No persistent volumes |

---

## Build

| CLI Command | REST API | Core | Docker | ECS | CloudRun | ACA | Lambda | GCF | AZF |
|-------------|----------|------|--------|-----|----------|-----|--------|-----|-----|
| `docker build` | `POST /build` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker builder prune` | `POST /build/prune` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `docker commit` | `POST /commit` | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

- Core: In-memory Dockerfile processing, creates image record in store
- Docker: Proxies to Docker Engine build API
- Cloud/FaaS: Uses inherited core build (in-memory), result stored locally

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
| `podman pod create` | `POST /libpod/pods/create` | ✅ | ❌ | ✅ | ✅ | ✅ | ❌ | ❌ | ❌ |
| `podman pod list` | `GET /libpod/pods/json` | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod inspect` | `GET /libpod/pods/{name}/json` | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod start` | `POST /libpod/pods/{name}/start` | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod stop` | `POST /libpod/pods/{name}/stop` | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod kill` | `POST /libpod/pods/{name}/kill` | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |
| `podman pod rm` | `DELETE /libpod/pods/{name}` | ✅ | ❌ | ✅ | ✅ | ✅ | ✅ | ✅ | ✅ |

### Cloud Service Mapping — Pods

| Backend | Multi-Container Pod | How It Works |
|---------|-------------------|--------------|
| ECS | ✅ | Multiple containers in one ECS task definition |
| CloudRun | ✅ | Multiple containers in one Cloud Run Job |
| ACA | ✅ | Multiple containers in one Container Apps Job |
| Lambda/GCF/AZF | ❌ | FaaS backends reject multi-container pods |

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

Sockerless uses a driver-based architecture where each Docker API operation dispatches through pluggable interfaces. Cloud backends override the default drivers with cloud-native implementations.

| Driver | Interface | Core Default | Purpose |
|--------|-----------|-------------|---------|
| ExecDriver | `core.ExecDriver` | AgentExecDriver | Run commands in containers |
| StreamDriver | `core.StreamDriver` | AgentStreamDriver | Attach/logs streaming |
| FilesystemDriver | `core.FilesystemDriver` | AgentFilesystemDriver | Archive ops (docker cp) |
| NetworkDriver | `api.NetworkDriver` | SyntheticNetworkDriver | Docker network operations |
| CloudExecDriver | `core.CloudExecDriver` | NoOpCloudExecDriver | Cloud-native exec (no agent) |
| CloudNetworkDriver | `core.CloudNetworkDriver` | NoOpCloudNetworkDriver | Cloud VPC/SG/firewall mgmt |
| ServiceDiscoveryDriver | `core.ServiceDiscoveryDriver` | NoOpServiceDiscoveryDriver | DNS-based service resolution |
| StorageDriver | `core.StorageDriver` | NoOpStorageDriver | Cloud-native volume mounts |
| LogDriver | `api.LogDriver` | StreamCloudLogs + CloudLogFetchFunc | Cloud log streaming |

### Per-Cloud Driver Implementations

| Driver | AWS (ECS) | GCP (CloudRun) | Azure (ACA) |
|--------|-----------|----------------|-------------|
| CloudExec | ECS ExecuteCommand + SSM | Not supported (Jobs) | Container Apps exec API |
| CloudNetwork | EC2 Security Groups | Cloud DNS managed zones | Environment networking + NSGs |
| ServiceDiscovery | Cloud Map | Cloud DNS A records | Private DNS + internal registry |
| Storage | EFS + EBS | GCS FUSE + Persistent Disk | Azure Files + Azure Disk |
| Logging | CloudWatch Logs | Cloud Logging (logadmin) | Azure Monitor (KQL) |
| ImageRegistry | ECR | Artifact Registry | ACR |
