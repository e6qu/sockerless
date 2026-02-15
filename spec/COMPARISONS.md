# Sockerless: Backend Comparisons — API Calls, CLI Commands, and Cloud Services

> **Date:** February 2026
>
> **Purpose:** Per-operation comparison of what Docker does natively vs. what each Sockerless backend uses to achieve the same result.

---

## Legend

- **REST API Call**: The Docker Engine REST API endpoint
- **CLI Command**: The `docker` CLI command that maps to this endpoint
- **Docker Native**: What Docker Engine does internally (the reference behavior)
- **Sockerless Internal**: What the frontend sends to the backend via the internal API
- **Backend columns**: Which cloud services/APIs each backend uses to implement the operation

---

## 1. System Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `GET /_ping` | `docker ping` (implicit) | Returns `OK` from dockerd | Frontend handles directly (no backend call) | N/A | N/A | N/A | N/A | N/A | N/A | N/A | N/A |
| `HEAD /_ping` | *(implicit)* | Returns headers only | Frontend handles directly | N/A | N/A | N/A | N/A | N/A | N/A | N/A | N/A |
| `GET /version` | `docker version` | Returns dockerd version info | `GET /internal/v1/version` | Returns hardcoded info | Docker API `GET /version` | Returns sockerless + ECS info | Returns sockerless + Lambda info | Returns sockerless + CR info | Returns sockerless + CRF info | Returns sockerless + ACA info | Returns sockerless + AF info |
| `GET /info` | `docker info` | Returns system info (OS, runtimes, storage driver, etc.) | `GET /internal/v1/info` | Returns simulated info | Docker API `GET /info` | **ECS**: `DescribeCluster` + `ListTasks` for resource counts | **Lambda**: `GetAccountSettings` | **Cloud Run**: `projects.locations.list` | **CRF**: `projects.locations.list` | **ACA**: `managedEnvironments.get` | **AF**: `sites.list` |
| `POST /auth` | `docker login` | Validates credentials against registry | `POST /internal/v1/auth` | Stores credentials in memory | Docker API `POST /auth` | Validates against registry; stores for ECR/Docker Hub | Validates against registry; stores for ECR | Validates against registry; stores for AR/GCR | Same as Cloud Run | Validates against registry; stores for ACR | Same as ACA |

---

## 2. Image Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `POST /images/create` (pull) | `docker pull nginx:latest` | Downloads layers from registry to local store; `X-Registry-Auth` header for private registries | `POST /internal/v1/images/pull` | Records image reference in state; no actual download | Docker API `POST /images/create` (real pull) | Records ref; **ECS pulls at task launch** from ECR/Docker Hub. Optionally pre-validates via **ECR**: `BatchGetImage` or registry API HEAD | Records ref; **Lambda** pulls from **ECR** at function create. Must be in ECR. | Records ref; **Cloud Run** pulls at job creation from **Artifact Registry**/GCR/Docker Hub | Records ref; **Cloud Run Functions** 2nd gen pulls from **Artifact Registry** | Records ref; **ACA** pulls at job creation from **ACR**/Docker Hub | Records ref; **Azure Functions** pulls from **ACR** at function deploy |
| `GET /images/{name}/json` | `docker inspect nginx:latest` | Returns image metadata from local store (config, layers, size) | `GET /internal/v1/images/{name}` | Returns stored metadata | Docker API `GET /images/{name}/json` | Returns stored metadata; optionally fetches from **Registry Manifest API** (GET manifest + config blob) | Returns stored metadata; **ECR**: `DescribeImages` | Returns stored metadata; **Artifact Registry**: `GetDockerImage` | Same as Cloud Run | Returns stored metadata; **ACR**: REST API image manifest | Same as ACA |
| `POST /images/load` | `docker load -i image.tar` | Imports image from tar archive into local store | `POST /internal/v1/images/load` | Extracts metadata from tar; stores | Docker API `POST /images/load` | Extracts image; pushes to **ECR** (`PutImage`) so ECS can pull it | Extracts image; pushes to **ECR** | Extracts image; pushes to **Artifact Registry** | Same as Cloud Run | Extracts image; pushes to **ACR** | Same as ACA |
| `POST /images/{name}/tag` | `docker tag nginx:latest myapp:v1` | Creates new tag reference in local store | `POST /internal/v1/images/{name}/tag` | Updates tag in state | Docker API `POST /images/{name}/tag` | Updates tag in state; optionally tags in **ECR** | Updates tag in state; **ECR** tag | Updates tag in state; **Artifact Registry** tag | Same as Cloud Run | Updates tag in state; **ACR** tag | Same as ACA |

---

## 3. Container Lifecycle

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `POST /containers/create` | `docker create --name web nginx` | Creates container: allocates ID, stores config, creates filesystem layers, sets up networking config. Does NOT start. | `POST /internal/v1/containers` | Generates ID; stores config in memory | Docker API `POST /containers/create` | Stores config; **registers ECS Task Definition** via `RegisterTaskDefinition` (image, cpu, mem, env, volumes, network mode) | Stores config; **creates/updates Lambda function** via `CreateFunction` or `UpdateFunctionConfiguration` | Stores config; **creates Cloud Run Job** via `jobs.create` | Stores config; **creates Cloud Run Function** via `functions.create` | Stores config; **creates ACA Job** via `jobs.create` | Stores config; **creates Azure Function** via `sites.create` |
| `POST /containers/{id}/start` | `docker start web` | Starts container process: mounts volumes, configures network, runs entrypoint | `POST /internal/v1/containers/{id}/start` | Sets state to "running" | Docker API `POST /containers/{id}/start` | **ECS**: `RunTask` (Fargate launch type, assigns ENI with private IP, starts container with agent). Returns agent address. Uses **EFS** for volumes, **VPC** subnets + **Security Groups** for networking, **Cloud Map** for DNS | **Lambda**: `Invoke` (async). No agent. | **Cloud Run**: `executions.create` (run the job). Agent listens on ingress port | **CRF**: trigger function invocation. No agent. | **ACA**: `executions.start`. Agent on VNet-accessible port | **AF**: trigger function. No agent. |
| `GET /containers/{id}/json` | `docker inspect web` | Returns full container metadata: state, config, network settings, mounts, health status | `GET /internal/v1/containers/{id}` | Returns stored state | Docker API `GET /containers/{id}/json` | **ECS**: `DescribeTasks` (task status, ENI IP, containers[].lastStatus, healthStatus). **CloudWatch**: health check logs | **Lambda**: `GetFunction` + `GetFunctionConfiguration` | **Cloud Run**: `executions.get` (status, conditions) | **CRF**: `functions.get` + `operations.get` | **ACA**: `executions.get` (status, containers) | **AF**: `sites.get` + function status |
| `GET /containers/json` | `docker ps` / `docker ps -a` | Lists containers from local state, filtered by labels/status/id/name | `GET /internal/v1/containers` | Filters in-memory state | Docker API `GET /containers/json` | **ECS**: `ListTasks` + `DescribeTasks` (filtered by cluster, family, or tags). Label filtering done in backend state (ECS tags map to labels) | **Lambda**: `ListFunctions` (filtered by tags) | **Cloud Run**: `executions.list` | **CRF**: `functions.list` | **ACA**: `executions.list` | **AF**: `sites.list` |
| `POST /containers/{id}/stop` | `docker stop web` | Sends SIGTERM, waits timeout, then SIGKILL | `POST /internal/v1/containers/{id}/stop` | Sets state to "exited" | Docker API `POST /containers/{id}/stop` | **ECS**: `StopTask` (sends SIGTERM; ECS force-kills after 30s or `stopTimeout`) | N/A (Lambda exits on its own) | **Cloud Run**: `executions.cancel` | N/A | **ACA**: `executions.stop` | N/A |
| `POST /containers/{id}/kill` | `docker kill web` | Sends specified signal (default SIGKILL) immediately | `POST /internal/v1/containers/{id}/kill` | Sets state to "exited" | Docker API `POST /containers/{id}/kill` | **ECS**: `StopTask` (no signal control; always SIGTERM→SIGKILL) | N/A | **Cloud Run**: `executions.cancel` | N/A | **ACA**: `executions.stop` | N/A |
| `POST /containers/{id}/wait` | `docker wait web` | Blocks until container exits; returns exit code | `WS /internal/v1/containers/{id}/wait` | Waits for state change to "exited" | Docker API `POST /containers/{id}/wait` | **ECS**: Polls `DescribeTasks` until `lastStatus=STOPPED`; reads `containers[].exitCode` | **Lambda**: Polls invocation status via **CloudWatch Logs** or `GetFunctionUrlConfig` | **Cloud Run**: Polls `executions.get` until `conditions[].type=Completed` | Polls function operation status | **ACA**: Polls `executions.get` until terminal state | Polls function invocation status |
| `DELETE /containers/{id}` | `docker rm web` / `docker rm -f web` | Removes container: deletes filesystem, network endpoints, state. If force: kills first. | `DELETE /internal/v1/containers/{id}` | Removes from memory | Docker API `DELETE /containers/{id}` | **ECS**: `StopTask` (if force) + `DeregisterTaskDefinition`. Clean up **Cloud Map** service instances, **EFS** access points | **Lambda**: `DeleteFunction` | **Cloud Run**: `jobs.delete` | **CRF**: `functions.delete` | **ACA**: `jobs.delete` | **AF**: `sites.delete` |

---

## 4. Container I/O

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `GET /containers/{id}/logs` | `docker logs web` | Reads from container's log driver (json-file, journald, etc.). Returns multiplexed stream (8-byte headers). | `GET /internal/v1/containers/{id}/logs` (one-shot) or `WS .../logs/stream` (follow) | Returns empty/mock logs | Docker API `GET /containers/{id}/logs` | **CloudWatch Logs**: `GetLogEvents` (one-shot) or `FilterLogEvents` with polling (follow). Log group: `/sockerless/containers`, log stream: `{task-id}/{container-name}` | **CloudWatch Logs**: `GetLogEvents` for function execution logs | **Cloud Logging**: `entries.list` with filter `resource.type="cloud_run_job"` | **Cloud Logging**: `entries.list` with filter for function | **Azure Monitor**: Log Analytics query `ContainerAppConsoleLogs` | **Azure Monitor**: `ApplicationInsights` traces query |
| `POST /containers/{id}/attach` | `docker attach web` | Hijacks HTTP connection; bidirectional multiplexed stream (stdin→container, container stdout/stderr→client with 8-byte headers) | Frontend connects to agent WebSocket, bridges to Docker client's hijacked connection | Simulated stream | Docker API `POST /containers/{id}/attach` (native) | Frontend → **Agent WebSocket** at `{task ENI IP}:9111`. Agent pipes to container's main process stdio. | **Not supported** (capability: `attach: false`) | Frontend → **Agent WebSocket** at Cloud Run ingress URL. Agent pipes stdio. | **Not supported** | Frontend → **Agent WebSocket** at VNet IP:9111. Agent pipes stdio. | **Not supported** |

---

## 5. Exec Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `POST /containers/{id}/exec` | `docker exec web sh` (create phase) | Creates exec instance: stores cmd/env/workdir config, allocates exec ID | `POST /internal/v1/containers/{id}/exec` | Stores exec config, returns ID | Docker API `POST /containers/{id}/exec` | Stores exec config in backend state, returns exec ID | **Not supported** (capability: `exec: false`) | Stores exec config in backend state | **Not supported** | Stores exec config in backend state | **Not supported** |
| `POST /exec/{id}/start` | `docker exec web sh` (start phase) | Hijacks connection; forks process inside container's namespaces; streams stdin/stdout/stderr with multiplexed framing | Frontend connects to agent, sends exec command, bridges streams | Returns mock output | Docker API `POST /exec/{id}/start` (native hijack) | Frontend → **Agent WebSocket** at `{task IP}:9111` → agent `fork+exec` inside container → multiplexed stream bridge | N/A | Frontend → **Agent WebSocket** → agent `fork+exec` → stream bridge | N/A | Frontend → **Agent WebSocket** → agent `fork+exec` → stream bridge | N/A |
| `GET /exec/{id}/json` | `docker inspect <exec-id>` | Returns exec instance state (running, exit code, pid) | `GET /internal/v1/exec/{id}` | Returns stored state | Docker API `GET /exec/{id}/json` | Returns exec state from backend store (updated by agent via `exit` message) | N/A | Returns exec state from backend store | N/A | Returns exec state from backend store | N/A |

---

## 6. Network Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `POST /networks/create` | `docker network create mynet` | Creates bridge/overlay network via libnetwork. Allocates subnet, sets up iptables rules, DNS resolver. | `POST /internal/v1/networks` | Stores network config; allocates virtual subnet | Docker API `POST /networks/create` | **VPC**: Creates **Security Group** for network isolation. **Cloud Map**: Creates `PrivateDnsNamespace` for DNS-based service discovery | N/A (no networking) | **VPC**: Tags for network grouping. DNS via Cloud Run internal routing | N/A | **ACA Environment**: Managed VNet. Internal DNS for service discovery | N/A |
| `GET /networks` | `docker network ls` | Lists all networks from libnetwork state | `GET /internal/v1/networks` | Returns from memory | Docker API `GET /networks` | Returns from backend state (security group + Cloud Map namespace metadata) | Returns empty list | Returns from backend state | Returns empty list | Returns from backend state | Returns empty list |
| `GET /networks/{id}` | `docker network inspect mynet` | Returns network details including connected containers, IPAM config, driver | `GET /internal/v1/networks/{id}` | Returns stored config + connected containers | Docker API `GET /networks/{id}` | Backend state + **EC2**: `DescribeSecurityGroups` + **Cloud Map**: `ListServiceInstances` (connected containers) | N/A | Backend state | N/A | Backend state + ACA environment info | N/A |
| `POST /networks/{id}/disconnect` | `docker network disconnect mynet web` | Detaches container from network; removes veth pair, DNS entry | `POST /internal/v1/networks/{id}/disconnect` | Removes container from network state | Docker API `POST /networks/{id}/disconnect` | **Cloud Map**: `DeregisterInstance` (remove DNS). **EC2**: Remove ENI security group association | N/A | Remove internal DNS entry | N/A | Remove from ACA environment DNS | N/A |
| `DELETE /networks/{id}` | `docker network rm mynet` | Deletes network: removes bridge, iptables rules, DNS config | `DELETE /internal/v1/networks/{id}` | Removes from memory | Docker API `DELETE /networks/{id}` | **EC2**: `DeleteSecurityGroup`. **Cloud Map**: `DeleteNamespace` | N/A | Remove VPC tags/DNS zone | N/A | N/A (environment-scoped) | N/A |
| `POST /networks/prune` | `docker network prune` | Removes all unused networks (no connected containers) | `POST /internal/v1/networks/prune` | Removes networks with no containers | Docker API `POST /networks/prune` | Bulk cleanup: **EC2** security groups + **Cloud Map** namespaces with no instances | N/A | Bulk cleanup | N/A | Bulk cleanup | N/A |

---

## 7. Volume Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `POST /volumes/create` | `docker volume create cache` | Creates named volume on local filesystem (default: `/var/lib/docker/volumes/{name}/_data`); optional driver (local, nfs, etc.) | `POST /internal/v1/volumes` | Stores volume metadata | Docker API `POST /volumes/create` | **EFS**: `CreateAccessPoint` on pre-provisioned filesystem. Access point path = `/volumes/{name}` | N/A (no volumes) | **GCS**: Create prefix/directory in bucket. Or **Filestore**: create share | N/A | **Azure Files**: Create file share via `fileShares.create` | N/A |
| `GET /volumes` | `docker volume ls` | Lists all named volumes from local store | `GET /internal/v1/volumes` | Returns from memory | Docker API `GET /volumes` | Returns from backend state (EFS access point metadata) | Returns empty list | Returns from backend state | Returns empty list | Returns from backend state | Returns empty list |
| `GET /volumes/{name}` | `docker volume inspect cache` | Returns volume metadata (driver, mountpoint, labels, options) | `GET /internal/v1/volumes/{name}` | Returns stored metadata | Docker API `GET /volumes/{name}` | Backend state + **EFS**: `DescribeAccessPoints` | N/A | Backend state | N/A | Backend state + **Azure Files** share info | N/A |
| `DELETE /volumes/{name}` | `docker volume rm cache` | Deletes volume directory from local filesystem | `DELETE /internal/v1/volumes/{name}` | Removes from memory | Docker API `DELETE /volumes/{name}` | **EFS**: `DeleteAccessPoint`. Optionally delete data at access point path | N/A | **GCS**: Delete prefix. Or **Filestore**: delete share | N/A | **Azure Files**: `fileShares.delete` | N/A |

---

## 8. Bind Mount Mapping

Bind mounts (`-v /host/path:/container/path`) are specified in `POST /containers/create` under `HostConfig.Binds`. They are not separate API calls but are critical for CI runners.

| Bind Mount Use | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|
| `/builds` (shared between helper + build containers) | Same host directory mounted into multiple containers | **EFS** access point mounted into multiple Fargate tasks at same path | **GCS FUSE** bucket prefix mounted into multiple Cloud Run executions | **Azure Files** share mounted into multiple ACA job containers |
| `/cache` (persistent across jobs) | Named volume or host directory | **EFS** access point (persistent across task runs) | **GCS** bucket (persistent) | **Azure Files** share (persistent) |
| Docker socket (`/var/run/docker.sock`) | Direct host socket passthrough | Not applicable (no Docker socket in Fargate) | Not applicable | Not applicable |

---

## 9. Health Check Flow

Health checks are not separate API calls but affect `GET /containers/{id}/json` responses and are used by CI runners and Docker Compose for readiness polling.

| Aspect | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|
| Health check source | `HEALTHCHECK` instruction in Dockerfile or compose `healthcheck:` config | **ECS Task Definition** `healthCheck` field OR agent-executed health check | Agent-executed health check | **ACA** container `probes` config OR agent-executed |
| Health check execution | dockerd runs the command inside the container periodically | Agent runs the command via `fork+exec`; reports status to backend | Agent runs the command; reports status to backend | Agent runs the command; reports status to backend |
| Status reported in | `State.Health.Status` in inspect response (`starting`, `healthy`, `unhealthy`) | Backend stores status from agent report; returned in inspect | Same | Same |
| Cloud-native health check | N/A (Docker is the health checker) | **ECS**: Container health check (built-in, runs command in container) | **Cloud Run**: Startup probe + liveness probe (HTTP/TCP/gRPC only, not exec) | **ACA**: Liveness + readiness + startup probes (HTTP/TCP) |

---

## 10. Agent Operations (Not Docker API — Internal)

These are not Docker REST API calls but show how the agent is used internally by each backend.

| Operation | When Triggered | ECS | Cloud Run | ACA |
|---|---|---|---|---|
| Agent injection | `POST /containers/{id}/start` | Mount agent binary from **EFS**; prepend to entrypoint. Agent binary stored at EFS path `/agent/sockerless-agent` | Build wrapper image with agent layer OR use **Cloud Run sidecar** | Mount agent from **Azure Files**; prepend to entrypoint |
| Agent networking | After task starts | Agent listens on `:9111` on task's **ENI private IP**. Frontend accesses via VPC. **Security Group** must allow inbound on 9111 from frontend | Agent listens on Cloud Run's `$PORT`. Frontend accesses via **Cloud Run internal URL** or VPC | Agent listens on `:9111`. Frontend accesses via **VNet** peering/integration |
| Agent auth | Frontend connects to agent | Frontend sends `Authorization: Bearer <token>` header. Token was generated by backend and injected as `SOCKERLESS_AGENT_TOKEN` env var | Same | Same |
| Exec via agent | `POST /exec/{id}/start` | Frontend WebSocket → agent at `{ENI_IP}:9111` → `fork+exec` → stream stdout/stderr → frontend → Docker client (multiplexed) | Frontend WebSocket → agent at Cloud Run URL → same flow | Frontend WebSocket → agent at `{VNet_IP}:9111` → same flow |
| Status reporting | Continuous | **ECS**: `DescribeTasks` API for task status. Agent reports health check results to backend via HTTP | **Cloud Run**: `executions.get` for status. Agent reports health to backend | **ACA**: `executions.get` for status. Agent reports health to backend |

---

## 11. Capability Summary per Backend

| Capability | Docker | Memory | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|
| `POST /containers/create` | `dockerd` | In-memory | `RegisterTaskDefinition` | `CreateFunction` | `jobs.create` | `functions.create` | `jobs.create` | `sites.create` |
| `POST /containers/{id}/start` | `dockerd` | State change | `RunTask` | `Invoke` | `executions.create` | Trigger | `executions.start` | Trigger |
| `GET /containers/{id}/json` | `dockerd` | State read | `DescribeTasks` | `GetFunction` | `executions.get` | `functions.get` | `executions.get` | `sites.get` |
| `GET /containers/{id}/logs` | Log driver | Mock | **CloudWatch** | **CloudWatch** | **Cloud Logging** | **Cloud Logging** | **Azure Monitor** | **Azure Monitor** |
| `POST /containers/{id}/attach` | Hijack | Mock | **Agent (WS)** | N/A | **Agent (WS)** | N/A | **Agent (WS)** | N/A |
| `POST /exec/{id}/start` | Hijack | Mock | **Agent (WS)** | N/A | **Agent (WS)** | N/A | **Agent (WS)** | N/A |
| `POST /containers/{id}/stop` | `dockerd` | State change | `StopTask` | N/A | `executions.cancel` | N/A | `executions.stop` | N/A |
| `DELETE /containers/{id}` | `dockerd` | State delete | `DeregisterTaskDef` | `DeleteFunction` | `jobs.delete` | `functions.delete` | `jobs.delete` | `sites.delete` |
| `POST /networks/create` | `libnetwork` | In-memory | **SG** + **Cloud Map** | N/A | VPC tags | N/A | ACA env DNS | N/A |
| `POST /volumes/create` | Local FS | In-memory | **EFS** AccessPoint | N/A | **GCS** / Filestore | N/A | **Azure Files** | N/A |
