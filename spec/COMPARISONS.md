# Sockerless: Backend Comparisons — API Calls, CLI Commands, and Cloud Services

> **Date:** February 2026
>
> **Status:** Updated to reflect actual implementation (Phase 35)
>
> **Purpose:** Per-operation comparison of what Docker does natively vs. what each Sockerless backend uses to achieve the same result.
>
> **Note:** Cloud backends use **in-memory state** for container inspect/list, network, and volume operations. They call cloud APIs only for container lifecycle operations (create, start, stop, remove) and logs. Networking and volume management are handled at the infrastructure level (Terraform), not dynamically by the backend at runtime.

---

## Legend

- **REST API Call**: The Docker Engine REST API endpoint
- **CLI Command**: The `docker` CLI command that maps to this endpoint
- **Docker Native**: What Docker Engine does internally (the reference behavior)
- **Sockerless Internal**: What the frontend sends to the backend via the internal API
- **Backend columns**: Which cloud services/APIs each backend uses to implement the operation

---

## 1. System Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | All Cloud Backends |
|---|---|---|---|---|---|---|
| `GET /_ping` | `docker ping` (implicit) | Returns `OK` from dockerd | Frontend handles directly (no backend call) | N/A | N/A | N/A |
| `HEAD /_ping` | *(implicit)* | Returns headers only | Frontend handles directly | N/A | N/A | N/A |
| `GET /version` | `docker version` | Returns dockerd version info | `GET /internal/v1/version` | Returns hardcoded Sockerless version | Docker API `GET /version` | Returns Sockerless version with backend name |
| `GET /info` | `docker info` | Returns system info (OS, runtimes, storage driver, etc.) | `GET /internal/v1/info` | Returns info with `OSType: "linux"`, container/image counts from local state | Docker API `GET /info` | Returns info from local state (no cloud API calls for resource counts) |
| `POST /auth` | `docker login` | Validates credentials against registry | `POST /internal/v1/auth` | Stores credentials in memory | Docker API `POST /auth` | Stores credentials in local state for later pull operations |
| `GET /events` | `docker events` | Streams real-time events | `GET /internal/v1/events` | Returns synthetic events from state changes | Docker API `GET /events` | Returns synthetic events from state changes |
| `GET /system/df` | `docker system df` | Returns disk usage statistics | `GET /internal/v1/system/df` | Returns sizes based on WASM rootfs directories | Docker API `GET /system/df` | Returns estimated sizes from local state |

---

## 2. Image Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `POST /images/create` (pull) | `docker pull nginx:latest` | Downloads layers from registry to local store; `X-Registry-Auth` header for private registries | `POST /internal/v1/images/pull` | Records image reference in state; no actual download. Optionally fetches image config from registry if `SOCKERLESS_FETCH_IMAGE_CONFIG=true` | Docker API `POST /images/create` (real pull) | Records ref in state; **ECS pulls at `RunTask` time** from ECR/Docker Hub | Records ref; **Lambda** pulls from **ECR** at `CreateFunction` time | Records ref; **Cloud Run** pulls at `CreateJob` time from **Artifact Registry**/GCR/Docker Hub | Records ref; **Cloud Run Functions** pulls at `CreateFunction` time from **Artifact Registry** | Records ref; **ACA** pulls at job creation time from **ACR**/Docker Hub | Records ref; **Azure Functions** pulls from **ACR** at Function App deploy |
| `GET /images/{name}/json` | `docker inspect nginx:latest` | Returns image metadata from local store (config, layers, size) | `GET /internal/v1/images/{name}` | Returns stored metadata from local state | Docker API `GET /images/{name}/json` | Returns stored metadata from local state (no cloud API call) | Returns stored metadata from local state | Returns stored metadata from local state | Same | Returns stored metadata from local state | Same |
| `POST /images/load` | `docker load -i image.tar` | Imports image from tar archive into local store | `POST /internal/v1/images/load` | Extracts metadata from tar; stores in local state; does NOT push to cloud registries | Docker API `POST /images/load` | Extracts metadata; stores in local state (creates synthetic image entry) | Same as ECS | Same | Same | Same | Same |
| `POST /images/{name}/tag` | `docker tag nginx:latest myapp:v1` | Creates new tag reference in local store | `POST /internal/v1/images/{name}/tag` | Updates tag in local state | Docker API `POST /images/{name}/tag` | Updates tag in local state (no cloud API call) | Same | Same | Same | Same | Same |
| `POST /build` | `docker build .` | Parses Dockerfile, executes RUN steps, creates image layers | `POST /internal/v1/images/build` | Parses Dockerfile (FROM, COPY, ADD, ENV, CMD, ENTRYPOINT, WORKDIR, ARG, LABEL, EXPOSE, USER). RUN is no-op. Creates image in state. COPY files staged in `BuildContexts` | Docker API `POST /build` (real build) | Same as Memory (Dockerfile parser in core) | Same | Same | Same | Same | Same |
| `GET /images/json` | `docker images` | Lists all images from local store | `GET /internal/v1/images` | Returns from local state | Docker API `GET /images/json` | Returns from local state | Same | Same | Same | Same | Same |
| `DELETE /images/{name}` | `docker rmi nginx` | Removes image from local store | `DELETE /internal/v1/images/{name}` | Removes from local state | Docker API `DELETE /images/{name}` | Removes from local state | Same | Same | Same | Same | Same |

---

## 3. Container Lifecycle

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `POST /containers/create` | `docker create --name web nginx` | Creates container: allocates ID, stores config, creates filesystem layers, sets up networking config. Does NOT start. | `POST /internal/v1/containers` | Generates ID; stores config; creates WASM rootfs | Docker API `POST /containers/create` | Stores config; **`RegisterTaskDefinition`** | Stores config; **`CreateFunction`** (container image) | Stores config only (deferred job creation) | Stores config; **`CreateFunction`** (Docker runtime, synchronous 1-3 min) | Stores config only (deferred job creation) | Stores config; **Create App Service Plan + Function App** |
| `POST /containers/{id}/start` | `docker start web` | Starts container process: mounts volumes, configures network, runs entrypoint | `POST /internal/v1/containers/{id}/start` | Starts WASM process; sets state to "running" | Docker API `POST /containers/{id}/start` | **`RunTask`** (Fargate); polls until RUNNING; extracts agent address from ENI IP | **`Invoke`** (async); agent dials back via reverse WebSocket | **`CreateJob`** + **`RunJob`**; polls until RUNNING; extracts agent address | HTTP POST invoke (async); agent dials back | **`BeginCreateOrUpdate`** (Job) + **`BeginStart`**; polls until RUNNING | Start Function App; agent dials back |
| `GET /containers/{id}/json` | `docker inspect web` | Returns full container metadata: state, config, network settings, mounts, health status | `GET /internal/v1/containers/{id}` | Returns stored state | Docker API `GET /containers/{id}/json` | Returns from **local state** (no cloud API call) | Returns from **local state** | Returns from **local state** | Returns from **local state** | Returns from **local state** | Returns from **local state** |
| `GET /containers/json` | `docker ps` / `docker ps -a` | Lists containers from local state, filtered by labels/status/id/name | `GET /internal/v1/containers` | Filters in-memory state | Docker API `GET /containers/json` | Filters **local state** (no `ListTasks` call) | Filters **local state** | Filters **local state** | Filters **local state** | Filters **local state** | Filters **local state** |
| `POST /containers/{id}/stop` | `docker stop web` | Sends SIGTERM, waits timeout, then SIGKILL | `POST /internal/v1/containers/{id}/stop` | Stops WASM process; sets state to "exited" | Docker API `POST /containers/{id}/stop` | **`StopTask`** | Disconnects reverse agent | **Cancel Execution** | Disconnects reverse agent | **`BeginStopExecution`** | Stop Function App |
| `POST /containers/{id}/kill` | `docker kill web` | Sends specified signal (default SIGKILL) immediately | `POST /internal/v1/containers/{id}/kill` | Stops WASM process; sets state to "exited" | Docker API `POST /containers/{id}/kill` | **`StopTask`** | Disconnects reverse agent | **Cancel Execution** | Disconnects reverse agent | **`BeginStopExecution`** | Disconnects reverse agent |
| `POST /containers/{id}/wait` | `docker wait web` | Blocks until container exits; returns exit code | `WS /internal/v1/containers/{id}/wait` | Waits for WASM process exit | Docker API `POST /containers/{id}/wait` | Waits via local state + agent disconnect | Waits for agent disconnect | Waits via local state + agent disconnect | Waits for agent disconnect | Waits via local state + agent disconnect | Waits for agent disconnect |
| `DELETE /containers/{id}` | `docker rm web` / `docker rm -f web` | Removes container: deletes filesystem, network endpoints, state. If force: kills first. | `DELETE /internal/v1/containers/{id}` | Removes from memory; cleans up WASM rootfs | Docker API `DELETE /containers/{id}` | **`StopTask`** (if force) + **`DeregisterTaskDefinition`** | **`DeleteFunction`** | **`DeleteJob`** | **`DeleteFunction`** | **`BeginDelete`** (Job) | **Delete Function App + App Service Plan** |

---

## 4. Container I/O

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `GET /containers/{id}/logs` | `docker logs web` | Reads from container's log driver (json-file, journald, etc.). Returns multiplexed stream (8-byte headers). | `GET /internal/v1/containers/{id}/logs` (one-shot) or `WS .../logs/stream` (follow) | Returns real WASM process output via StreamDriver chain | Docker API `GET /containers/{id}/logs` | **CloudWatch Logs**: `GetLogEvents` (one-shot) or `FilterLogEvents` with polling (follow) | **CloudWatch Logs**: `GetLogEvents` for function execution logs | **Cloud Logging**: `entries.list` via Log Admin API | **Cloud Logging**: `entries.list` via Log Admin API | **Azure Monitor**: Log Analytics `QueryWorkspace` | **Azure Monitor**: Log Analytics query |
| `POST /containers/{id}/attach` | `docker attach web` | Hijacks HTTP connection; bidirectional multiplexed stream (stdin→container, container stdout/stderr→client with 8-byte headers) | Backend hijacks connection; dispatches through StreamDriver chain | Real WASM process attach via ProcessDriver; stdin forwarding supported | Docker API `POST /containers/{id}/attach` (native) | Frontend → **Agent WebSocket** at `{task ENI IP}:9111` (forward agent) | Frontend → **Reverse agent WebSocket** (agent dials back to backend) | Frontend → **Agent WebSocket** at Cloud Run ingress URL (forward agent) | Frontend → **Reverse agent WebSocket** | Frontend → **Agent WebSocket** at VNet IP:9111 (forward agent) | Frontend → **Reverse agent WebSocket** |

---

## 5. Exec Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|---|
| `POST /containers/{id}/exec` | `docker exec web sh` (create phase) | Creates exec instance: stores cmd/env/workdir config, allocates exec ID | `POST /internal/v1/containers/{id}/exec` | Stores exec config, returns ID | Docker API `POST /containers/{id}/exec` | Stores exec config in backend state, returns exec ID | Stores exec config (reverse agent handles start) | Stores exec config | Stores exec config (reverse agent handles start) | Stores exec config | Stores exec config (reverse agent handles start) |
| `POST /exec/{id}/start` | `docker exec web sh` (start phase) | Hijacks connection; forks process inside container's namespaces; streams stdin/stdout/stderr with multiplexed framing | Backend hijacks connection; dispatches through ExecDriver chain (Agent → WASM → Synthetic) | Real WASM execution via mvdan.cc/sh + wazero busybox | Docker API `POST /exec/{id}/start` (native hijack) | **Forward agent** at `{task IP}:9111` → agent exec → stream bridge | **Reverse agent** (agent already connected via callback) → exec → stream bridge | **Forward agent** at Cloud Run URL → exec → stream bridge | **Reverse agent** → exec → stream bridge | **Forward agent** at VNet IP → exec → stream bridge | **Reverse agent** → exec → stream bridge |
| `GET /exec/{id}/json` | `docker inspect <exec-id>` | Returns exec instance state (running, exit code, pid) | `GET /internal/v1/exec/{id}` | Returns stored state | Docker API `GET /exec/{id}/json` | Returns exec state from local store | Same | Same | Same | Same | Same |

---

## 6. Network Operations

> **Implementation note:** All backends handle network operations **in-memory** via `backends/core/`. No cloud APIs are called for network create/list/inspect/disconnect/remove. Cloud networking is configured at the infrastructure level (Terraform), not dynamically by the backend.

| REST API Call | CLI Command | Docker Native | Sockerless Internal | All Backends (except Docker) | Docker Backend |
|---|---|---|---|---|---|
| `POST /networks/create` | `docker network create mynet` | Creates bridge/overlay network via libnetwork. Allocates subnet, sets up iptables rules, DNS resolver. | `POST /internal/v1/networks` | Stores in local state; allocates virtual subnet from 172.18.0.0/16 IPAM pool | Docker API `POST /networks/create` |
| `GET /networks` | `docker network ls` | Lists all networks from libnetwork state | `GET /internal/v1/networks` | Returns from local state | Docker API `GET /networks` |
| `GET /networks/{id}` | `docker network inspect mynet` | Returns network details including connected containers, IPAM config, driver | `GET /internal/v1/networks/{id}` | Returns stored config + connected containers from local state | Docker API `GET /networks/{id}` |
| `POST /networks/{id}/connect` | `docker network connect mynet web` | Connects container to network | `POST /internal/v1/networks/{id}/connect` | Adds container to network in local state; assigns virtual IP | Docker API `POST /networks/{id}/connect` |
| `POST /networks/{id}/disconnect` | `docker network disconnect mynet web` | Detaches container from network; removes veth pair, DNS entry | `POST /internal/v1/networks/{id}/disconnect` | Removes container from network in local state | Docker API `POST /networks/{id}/disconnect` |
| `DELETE /networks/{id}` | `docker network rm mynet` | Deletes network: removes bridge, iptables rules, DNS config | `DELETE /internal/v1/networks/{id}` | Removes from local state | Docker API `DELETE /networks/{id}` |
| `POST /networks/prune` | `docker network prune` | Removes all unused networks (no connected containers) | `POST /internal/v1/networks/prune` | Removes networks with no connected containers from local state | Docker API `POST /networks/prune` |

---

## 7. Volume Operations

> **Implementation note:** All backends handle volume operations **in-memory** via `backends/core/`. No cloud APIs are called for volume create/list/inspect/remove. Cloud storage (EFS, GCS, Azure Files) is provisioned at the infrastructure level (Terraform) and referenced in container configurations, not dynamically created by the backend.

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Memory | Docker Backend | All Cloud Backends |
|---|---|---|---|---|---|---|
| `POST /volumes/create` | `docker volume create cache` | Creates named volume on local filesystem | `POST /internal/v1/volumes` | Stores volume metadata; creates symlink directory for WASM | Docker API `POST /volumes/create` | Stores volume metadata in local state (no cloud API call) |
| `GET /volumes` | `docker volume ls` | Lists all named volumes | `GET /internal/v1/volumes` | Returns from local state | Docker API `GET /volumes` | Returns from local state |
| `GET /volumes/{name}` | `docker volume inspect cache` | Returns volume metadata | `GET /internal/v1/volumes/{name}` | Returns stored metadata | Docker API `GET /volumes/{name}` | Returns from local state |
| `DELETE /volumes/{name}` | `docker volume rm cache` | Deletes volume | `DELETE /internal/v1/volumes/{name}` | Removes from local state; cleans up symlinks | Docker API `DELETE /volumes/{name}` | Removes from local state (no cloud API call) |
| `POST /volumes/prune` | `docker volume prune` | Removes unused volumes | `POST /internal/v1/volumes/prune` | Removes volumes not mounted by any container | Docker API `POST /volumes/prune` | Removes unused volumes from local state |

---

## 8. Bind Mount and Archive Mapping

### 8.1 Bind Mounts

Bind mounts (`-v /host/path:/container/path`) are specified in `POST /containers/create` under `HostConfig.Binds`. They are not separate API calls but are critical for CI runners.

| Bind Mount Use | Docker Native | Memory (WASM) | Cloud Backends |
|---|---|---|---|
| `/builds` (shared between helper + build containers) | Same host directory mounted into multiple containers | Symlinks in rootDir + DirMounts for WASM; shared across containers | Volume mounts configured at infrastructure level (EFS/GCS/Azure Files) |
| `/cache` (persistent across jobs) | Named volume or host directory | Same symlink mechanism | Same infrastructure-level mounts |
| Docker socket (`/var/run/docker.sock`) | Direct host socket passthrough | Silently accepted; not functional | Silently accepted; not functional |
| Host path bind mounts | Direct host path access | `resolveBindMounts` checks `filepath.IsAbs` for host paths | Mapped to cloud storage paths |

### 8.2 Archive Operations (`docker cp`)

| REST API Call | CLI Command | Docker Native | Memory (WASM) | Cloud Backends |
|---|---|---|---|---|
| `PUT /containers/{id}/archive` | `docker cp local_path container:/path` | Extracts tar to container filesystem | Extracts to WASM rootfs via FilesystemDriver. Pre-start: extracts to staging directory (`StagingDirs`); merged into rootfs at start time | Via agent FilesystemDriver (forward/reverse agent writes to host filesystem) |
| `HEAD /containers/{id}/archive` | *(used by `docker cp` internally)* | Returns file stat as base64 JSON header | Returns stat from WASM rootfs. Pre-start: checks staging dir; returns 404 for non-existent paths | Via agent FilesystemDriver |
| `GET /containers/{id}/archive` | `docker cp container:/path local_path` | Returns tar of path from container | Returns tar from WASM rootfs via FilesystemDriver | Via agent FilesystemDriver |

---

## 9. Health Check Flow

Health checks are implemented in `backends/core/health.go` (~200 lines, 6 unit tests). They affect `GET /containers/{id}/json` responses and are used by CI runners and Docker Compose for readiness polling.

| Aspect | Docker Native | Memory (WASM) | Cloud Backends (Agent) |
|---|---|---|---|
| Health check source | `HEALTHCHECK` in Dockerfile or compose `healthcheck:` | Same (parsed from container config) | Same |
| Health check execution | dockerd runs command inside container periodically | `StartHealthCheck` goroutine runs exec via WASM on interval | `StartHealthCheck` goroutine runs exec via agent on interval |
| Health check parsing | `parseHealthcheckCmd`: NONE → nil, CMD → args[1:], CMD-SHELL → `["sh", "-c", joined]` | Same | Same |
| Status reported in | `State.Health.Status` (`starting` → `healthy` / `unhealthy`) | Same (stored in `HealthChecks sync.Map` per container) | Same |
| Status tracking | `HealthChecks` map on Store holds `context.CancelFunc` per container | Same | Same |
| Stop behavior | Cancels health check context | `StopContainer` cancels health check before state transition | Same |
| Output capture | Via exec | `net.Pipe()` + `tty=true` for raw output capture | Same |

---

## 10. Agent Operations (Not Docker API — Internal)

These are not Docker REST API calls but show how the agent is used internally by each backend.

### 10.1 Forward Agent (Container-Based Backends)

| Operation | When Triggered | ECS | Cloud Run | ACA |
|---|---|---|---|---|
| Agent injection | `POST /containers/{id}/start` | Agent binary injected into container; entrypoint prepended with agent | Agent as sidecar or entrypoint wrapper | Agent injected; entrypoint prepended |
| Agent networking | After task starts | Agent listens on `:9111` on task's **ENI private IP**. Backend connects via VPC | Agent listens on Cloud Run's ingress port. Backend connects via internal URL | Agent listens on `:9111`. Backend connects via **VNet** |
| Agent auth | Backend connects to agent | Token-based: `SOCKERLESS_AGENT_TOKEN` env var | Same | Same |
| Exec via agent | `POST /exec/{id}/start` | Backend WebSocket → agent at `{ENI_IP}:9111` → exec → stream bridge | Backend WebSocket → agent at Cloud Run URL → same | Backend WebSocket → agent at `{VNet_IP}:9111` → same |

### 10.2 Reverse Agent (FaaS Backends)

| Operation | When Triggered | Lambda | Cloud Run Functions | Azure Functions |
|---|---|---|---|---|
| Agent injection | `POST /containers/{id}/start` | Agent embedded in container image; function invoked async | Same pattern | Same pattern |
| Agent callback | After function starts | Agent dials back to backend via `SOCKERLESS_CALLBACK_URL` (WebSocket) | Same | Same |
| Registration | On callback connect | `AgentRegistry.Prepare(id)` pre-creates done channel BEFORE invoke goroutine starts | Same | Same |
| Exec via agent | `POST /exec/{id}/start` | Backend uses existing reverse WebSocket connection → exec → stream bridge | Same | Same |
| Lifecycle | Agent disconnect | FaaS invoke goroutine waits for agent disconnect before stopping container | Same | Same |
| Auto-stop | Helper/cache containers | Auto-stop after 500ms (not long-running like CI containers) | Same | Same |

---

## 11. Capability Summary per Backend

| Capability | Docker | Memory | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|
| `POST /containers/create` | `dockerd` | In-memory + WASM rootfs | `RegisterTaskDefinition` | `CreateFunction` | In-memory (deferred) | `CreateFunction` | In-memory (deferred) | Create App + Plan |
| `POST /containers/{id}/start` | `dockerd` | Start WASM process | `RunTask` | `Invoke` (async) | `CreateJob` + `RunJob` | HTTP POST invoke | `BeginCreateOrUpdate` + `BeginStart` | Start Function App |
| `GET /containers/{id}/json` | `dockerd` | Local state | Local state | Local state | Local state | Local state | Local state | Local state |
| `GET /containers/{id}/logs` | Log driver | WASM process output | **CloudWatch** | **CloudWatch** | **Cloud Logging** | **Cloud Logging** | **Azure Monitor** | **Azure Monitor** |
| `POST /containers/{id}/attach` | Hijack | WASM process attach | **Forward agent** | **Reverse agent** | **Forward agent** | **Reverse agent** | **Forward agent** | **Reverse agent** |
| `POST /exec/{id}/start` | Hijack | WASM exec (mvdan.cc/sh) | **Forward agent** | **Reverse agent** | **Forward agent** | **Reverse agent** | **Forward agent** | **Reverse agent** |
| `POST /containers/{id}/stop` | `dockerd` | Stop WASM process | `StopTask` | Disconnect agent | Cancel Execution | Disconnect agent | `BeginStopExecution` | Stop Function App |
| `DELETE /containers/{id}` | `dockerd` | State delete | `DeregisterTaskDef` | `DeleteFunction` | `DeleteJob` | `DeleteFunction` | `BeginDelete` (Job) | Delete App + Plan |
| `POST /networks/create` | `libnetwork` | In-memory | In-memory | In-memory | In-memory | In-memory | In-memory | In-memory |
| `POST /volumes/create` | Local FS | In-memory + symlinks | In-memory | In-memory | In-memory | In-memory | In-memory | In-memory |
| `POST /build` | BuildKit | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser |
| Archive (docker cp) | FS access | WASM rootfs | Agent FS | Agent FS | Agent FS | Agent FS | Agent FS | Agent FS |

---

## 12. GitLab Runner Docker-Executor — Full Job Flow

This section traces the exact sequence of Docker API calls made by GitLab Runner's docker-executor during a CI job, showing what each cloud backend does at each step. Based on source code analysis of `executors/docker/` in the GitLab Runner repository.

### 12.1 API Version Negotiation

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Ping | `GET /_ping` | Returns `OK` + `API-Version` header | Frontend returns `API-Version: 1.44` directly (no backend call) | Same | Same |
| Version check | `GET /version` | Returns server version | Frontend queries backend for version info | Same | Same |
| System info | `GET /info` | Returns OS, arch, runtimes | Backend returns `OSType: "linux"`, `Architecture: "x86_64"` | Same | Same |

The runner uses `WithAPIVersionNegotiation()` — reads `API-Version` from `/_ping` and adjusts all subsequent calls. Has v1.44-specific code: MAC address in `EndpointsConfig` instead of `Config.MacAddress`.

### 12.2 Image Pull Phase

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Check helper image | `GET /images/{helper}/json` | Returns image metadata from local store, or 404 | Backend checks if image ref is recorded in state | Same | Same |
| Load helper from tar | `POST /images/load` (`.docker.tar.zst`) | Imports tar archive into local image store | Extracts image from tar; pushes to **ECR** via `PutImage` | Extracts; pushes to **Artifact Registry** | Extracts; pushes to **ACR** |
| Tag loaded image | `POST /images/{id}/tag` | Creates new tag ref | Records tag in backend state; optionally tags in **ECR** | Records tag; optionally tags in **AR** | Records tag; optionally tags in **ACR** |
| Verify helper | `GET /images/{helper}/json` | Returns image config | Returns stored metadata including `Config.Env`, `Config.Cmd` | Same | Same |
| Pull build image | `POST /images/create` + `X-Registry-Auth` | Downloads layers from registry | Records ref + creds; **ECS pulls at `RunTask` time** | Records ref; **Cloud Run pulls at `jobs.create`** | Records ref; **ACA pulls at `jobs.create`** |
| Pull service images | `POST /images/create` (per service) | Downloads each image | Same as build image | Same | Same |

**Note:** If `helper_image` is set in `config.toml`, steps 2-4 are skipped — the runner pulls from the configured registry instead. **Recommended for sockerless.**

**Image config retrieval:** When `GET /images/{name}/json` is called, the backend must return `Config.Env` (including `PATH`), `Config.Cmd`, `Config.Entrypoint`, `Config.ExposedPorts`. For cloud backends, this is fetched from the **registry manifest API** (`GET /v2/<name>/manifests/<tag>` → `GET /v2/<name>/blobs/<config-digest>`) without downloading layers.

### 12.3 Network Setup

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Create per-build network | `POST /networks/create` (name: `runner-net-<guid>`, labels, MTU option) | Creates bridge network via libnetwork; allocates subnet; enables DNS resolver | **EC2**: `CreateSecurityGroup` for isolation. **Cloud Map**: `CreatePrivateDnsNamespace` for service alias DNS | **VPC**: Tag-based grouping. **Cloud DNS**: Create private zone for aliases | **ACA Environment**: Uses managed VNet internal DNS |

With `FF_NETWORK_PER_BUILD` enabled (required for sockerless), all containers join this network via `NetworkingConfig.EndpointsConfig`.

### 12.4 Volume Setup

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Create build volume | `POST /volumes/create` (name: `runner-<hash>-build-<hash>`) | Creates named volume at `/var/lib/docker/volumes/...` | **EFS**: `CreateAccessPoint` at path `/volumes/<name>` | **GCS**: Create directory prefix in bucket | **Azure Files**: `fileShares.create` |
| Create cache volume | `POST /volumes/create` (name: `runner-<hash>-cache-<hash>`) | Same | Same | Same | Same |

All containers (helper, build, services) receive the **same `Binds` list** from `volumesManager.Binds()`. This is the primary data-sharing mechanism — helper clones the repo, build container reads it.

### 12.5 Service Containers

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Create service | `POST /containers/create` (image, network, volumes, `EndpointsConfig.Aliases: ["postgres", "db"]`) | Allocates container ID; stores config | Stores config; **registers Task Definition** with service image, env, volumes | Stores config; **creates Cloud Run Job** | Stores config; **creates ACA Job** |
| Start service | `POST /containers/{id}/start` | Starts container process | **ECS**: `RunTask` with Fargate; assigns ENI IP; agent starts | **Cloud Run**: `executions.create`; agent on ingress port | **ACA**: `executions.start`; agent on VNet port |
| Poll service status | `GET /containers/{id}/json` (poll until `State.Status != "created"`) | Returns container state | **ECS**: `DescribeTasks` → `containers[].lastStatus` | **Cloud Run**: `executions.get` → status | **ACA**: `executions.get` → status |
| Create health-check container | `POST /containers/create` (helper image, cmd: `["gitlab-runner-helper", "health-check"]`, same network) | Allocates container | Same as service — a separate cloud task | Same | Same |
| Start health-check | `POST /containers/{id}/start` | Starts TCP probing | Launches health-check as separate Fargate task; probes service via **Cloud Map** DNS | Separate Cloud Run execution; probes via internal DNS | Separate ACA job; probes via VNet DNS |
| Wait for health-check | `POST /containers/{id}/wait` | Blocks until exit code 0 (service ready) or timeout | Polls **ECS** `DescribeTasks` until stopped; reads exit code | Polls `executions.get` | Polls `executions.get` |

**Service readiness detail:** GitLab Runner does NOT use Docker's `State.Health` for services. It creates a SEPARATE helper container that TCP-probes the service's `ExposedPorts` (or `HEALTHCHECK_TCP_PORT` env var). The `wait_for_services_timeout` (default 30s) controls the overall timeout.

### 12.6 Build/Helper Container Execution

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Create container | `POST /containers/create` (image, cmd=[shell], `OpenStdin: true`, `AttachStdin/Stdout/Stderr: true`, network, volumes) | Allocates container; stores config | Stores config; registers Task Definition | Stores config; creates Job | Stores config; creates Job |
| **Attach BEFORE start** | `POST /containers/{id}/attach` (`stream=true, stdin=true, stdout=true, stderr=true`) | Returns hijacked connection **immediately** (registers interest; doesn't wait for output) | Frontend returns `101 Switching Protocols` immediately; **buffers** hijacked connection until agent is reachable | Same | Same |
| Start container | `POST /containers/{id}/start` | Starts process; attach stream begins receiving data | **ECS**: `RunTask`; waits for agent readiness at `{ENI_IP}:9111`; frontend connects WebSocket to agent; begins bridging buffered attach to agent stream | **Cloud Run**: `executions.create`; waits for agent at ingress URL; bridges | **ACA**: `executions.start`; waits for agent at VNet IP; bridges |
| I/O streaming | *(hijacked connection)* | `stdcopy.StdCopy` reads multiplexed stdout/stderr (8-byte headers); stdin writes build script | Frontend reads agent WebSocket `stdout`/`stderr` JSON messages → wraps in 8-byte mux headers → sends to Docker client. Client stdin → `stdin` WebSocket message → agent | Same | Same |
| Wait for exit | `POST /containers/{id}/wait` (condition: `not-running`) | Blocks until container exits; returns `StatusCode` | **ECS**: Polls `DescribeTasks` until `lastStatus=STOPPED` | **Cloud Run**: Polls `executions.get` until completed | **ACA**: Polls `executions.get` until terminal |

**Critical timing:** The runner calls `ContainerAttach` BEFORE `ContainerStart`. There is NO attach-specific timeout — only the job-level timeout applies. The hijacked connection can sit idle for the entire cloud startup latency (5-60s). The frontend must NOT time out this connection.

### 12.7 Cleanup

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| SIGTERM via exec | `POST /containers/{id}/exec` + `POST /exec/{id}/start` (cmd: `sh -c <sigterm-script>`) | Forks `sh` in container; sends SIGTERM to all PIDs | Frontend → Agent WebSocket → `fork+exec` the sigterm script | Same | Same |
| Stop container | `POST /containers/{id}/stop` (timeout=0) | SIGTERM → immediate SIGKILL | **ECS**: `StopTask` | **Cloud Run**: `executions.cancel` | **ACA**: `executions.stop` |
| Disconnect network | `POST /networks/{id}/disconnect` (container, force=true) | Removes network endpoint | **Cloud Map**: `DeregisterInstance`; **EC2**: remove SG association | Remove DNS entry | Remove from VNet DNS |
| Remove container | `DELETE /containers/{id}` (force=true, removeVolumes=true) | Kills if running; deletes FS + state | **ECS**: `StopTask` + `DeregisterTaskDefinition`; cleanup Cloud Map + EFS | **Cloud Run**: `jobs.delete` | **ACA**: `jobs.delete` |
| Remove network | `DELETE /networks/{id}` | Deletes bridge + iptables | **EC2**: `DeleteSecurityGroup`. **Cloud Map**: `DeleteNamespace` | Remove VPC tags + DNS zone | N/A (environment-scoped) |
| Remove volumes | `DELETE /volumes/{name}` | Deletes local directory | **EFS**: `DeleteAccessPoint` | **GCS**: Delete prefix | **Azure Files**: `fileShares.delete` |

Cleanup uses a **5-minute timeout** and removes all containers **in parallel** via goroutines.

---

## 13. GitHub Actions Runner — Full Job Flow

This section traces the exact Docker API calls made by GitHub Actions Runner for container jobs and service containers. Based on source code analysis of `DockerCommandManager.cs` and `ContainerOperationProvider.cs`.

### 13.1 Version and Auth

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Version check | `GET /version` (runner requires `ApiVersion ≥ 1.35`) | Returns server version | Frontend returns `ApiVersion: "1.44"` | Same | Same |
| Registry login | `POST /auth` (if credentials configured) | Validates creds against registry | Stores credentials for later pull | Same | Same |

### 13.2 Network Setup

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Create network | `POST /networks/create` (name: `github_network_<GUID>`, label: `<runner-hash>`) | Creates bridge network | **EC2**: `CreateSecurityGroup`. **Cloud Map**: `CreatePrivateDnsNamespace` | VPC tags + **Cloud DNS** private zone | ACA environment internal DNS |

**Network creation is MANDATORY.** Failure is fatal — no fallback to default bridge. All containers join this network.

### 13.3 Image Pull (with retries)

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Pull job image | `POST /images/create` (3 attempts with backoff) | Downloads layers | Records ref; cloud backend pulls at task launch | Same | Same |
| Pull service images | `POST /images/create` (per service, 3 attempts) | Downloads layers | Same | Same | Same |

### 13.4 Container Creation

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Create job container | `POST /containers/create` (`Entrypoint: ["tail"]`, `Cmd: ["-f", "/dev/null"]`, network, labels, `-v /var/run/docker.sock`, env vars) | Allocates container; stores config with overridden entrypoint | Stores config; registers Task Definition. **Agent substitutes** for `tail -f /dev/null` as keep-alive (`--keep-alive` mode). **EFS** for volumes. Accepts docker.sock mount silently | Stores config; creates Job. Agent replaces `tail` as keep-alive. Accepts socket mount silently | Same pattern |
| Create service containers | `POST /containers/create` (per service, with `HealthCheck` if image defines one, same network) | Allocates container | Stores config; registers Task Definition with health check | Same | Same |

**`tail -f /dev/null` handling:** The runner ALWAYS overrides entrypoint to `tail -f /dev/null`. Cloud backends detect this pattern and use the agent's `--keep-alive` mode instead — the agent stays alive and serves exec/attach without running a child process. Image must contain `tail` in its PATH for Docker, but sockerless never actually runs `tail`.

**Docker socket mount:** The runner always adds `-v /var/run/docker.sock:/var/run/docker.sock`. Cloud backends accept this bind mount silently without error. The mount is not applied to the cloud task.

### 13.5 Container Start and Verification

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Start container | `POST /containers/{id}/start` | Starts process (<1s) | **ECS**: `RunTask`; **blocks** until task `lastStatus=RUNNING` AND agent passes readiness check at `{ENI_IP}:9111/health`. Returns 204 only when fully ready (absorbs 10-45s latency) | **Cloud Run**: `executions.create`; blocks until agent ready at ingress URL (absorbs 5-30s) | **ACA**: `executions.start`; blocks until agent ready (absorbs 10-60s) |
| Verify running | `GET /containers/json` (`filters={"id":["<id>"],"status":["running"]}`) | Returns container in list | Backend returns container with `State: "running"` (guaranteed because start blocked until ready) | Same | Same |

**Critical:** The runner checks immediately after start. By blocking `start` until the cloud task is running and the agent is accepting connections, the subsequent `docker ps` check always succeeds.

### 13.6 Inspect and Setup

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Full inspect | `GET /containers/{id}/json` (full JSON, no `--format`) | Returns complete container metadata | Backend returns state with `Config.Env` (image env merged with user env, **must include PATH**), `Config.Healthcheck`, `State`, `NetworkSettings.Ports` | Same | Same |
| PATH extraction | *(runner parses `Config.Env` array for `PATH=` entry)* | `Config.Env` contains image env + user env | Backend fetches image config from **registry manifest API** at pull time; merges with user env at create time | Same | Same |

**`Config.Env` is mandatory.** If `PATH` is missing, the runner cannot locate executables for `docker exec` steps.

### 13.7 Health Check Polling (Services Only)

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Poll health | `GET /containers/{id}/json` (runner checks `Config.Healthcheck` presence, then reads `State.Health.Status`) | Returns `"starting"` → `"healthy"` → or `"unhealthy"` | Agent runs HEALTHCHECK command from image config via `fork+exec`; reports result to backend; backend stores in state | Same | Same |
| Backoff | *(runner uses exponential backoff: 2s, 3s, 7s, 13s... ~5-6 retries)* | Docker daemon runs health check at configured interval | Agent runs at image's health check interval (default 30s) | Same | Same |
| No HEALTHCHECK | *(if `Config.Healthcheck` is absent, runner skips polling — container considered ready immediately)* | `Config.Healthcheck` not present in inspect | Backend returns inspect without `Config.Healthcheck` field if image has no HEALTHCHECK | Same | Same |

### 13.8 Port Discovery (Services Only)

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Read ports | `GET /containers/{id}/json` → `NetworkSettings.Ports` | Returns port mappings: `{"5432/tcp": [{"HostIp": "0.0.0.0", "HostPort": "32768"}]}` | Backend assigns virtual ports; returns in inspect response. Service accessible via **Cloud Map** DNS alias, not host port | Returns virtual port mappings. Service accessible via internal URL | Returns virtual port mappings. Service accessible via VNet DNS |

The runner's `docker port` command reads from inspect `NetworkSettings.Ports`. The `docker` CLI formats this itself — sockerless only needs to return correct JSON.

### 13.9 Step Execution (Repeated per Workflow Step)

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Create exec | `POST /containers/{id}/exec` (`AttachStdin: true`, `AttachStdout: true`, `AttachStderr: true`, `Cmd: ["bash", "-e", "/path/to/script.sh"]`, `Env: [...]`, `WorkingDir: "..."`) | Stores exec config; allocates exec ID | Stores exec config in backend state | Same | Same |
| Start exec | `POST /exec/{id}/start` (`Detach: false`) → hijacked connection | Forks process; bidirectional multiplexed stream | Frontend connects to **Agent WebSocket** at `{ENI_IP}:9111`; sends `{"type":"exec","cmd":[...]}`; bridges hijacked connection ↔ WebSocket. Agent `fork+exec` inside container | Frontend → Agent at Cloud Run URL; same protocol | Frontend → Agent at VNet IP; same protocol |
| Read exit code | *(from `docker exec` process return code, NOT from container inspect)* | exec process returns exit code | Agent sends `{"type":"exit","code":N}` → frontend returns to Docker client as exec exit code | Same | Same |

**No exec retry.** If exec fails, the step fails immediately. The agent must be ready to accept exec on the first attempt — guaranteed by the start-blocking strategy in step 13.5.

**Exec with stdin (`-i`).** The runner keeps stdin open. The script file is volume-mounted and the runner executes `bash -e /path/to/script.sh` via exec. The agent must support `AttachStdin: true` with proper bidirectional streaming.

### 13.10 Container Actions (`uses: docker://image`)

Container actions have a DIFFERENT flow from container jobs:

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Create | `POST /containers/create` (original entrypoint, NOT `tail -f /dev/null`; workspace mount; `WorkingDir: "/github/workspace"`) | Stores config with image's own entrypoint | Stores config; Agent wraps original entrypoint: `["/sockerless-agent", "--", <original-entrypoint>]` | Same | Same |
| Start | `POST /containers/{id}/start` | Starts container with original entrypoint | **ECS**: `RunTask`; agent runs original command as child process | Same | Same |
| Stream logs | `GET /containers/{id}/logs` (`follow=true, stdout=true, stderr=true`) | Streams multiplexed log output | **CloudWatch Logs**: polls `FilterLogEvents` with follow | **Cloud Logging**: polls `entries.list` | **Azure Monitor**: polls log query |
| Wait | `POST /containers/{id}/wait` | Blocks until exit; returns container exit code | Polls **ECS** `DescribeTasks` until stopped; reads container exit code | Polls `executions.get` | Polls `executions.get` |
| Remove | `DELETE /containers/{id}` | Removes container | **ECS**: `DeregisterTaskDefinition` + cleanup | **Cloud Run**: `jobs.delete` | **ACA**: `jobs.delete` |

**Key difference:** Container actions run to completion with their own entrypoint (not kept alive for exec). Exit code comes from the container process, not from exec.

### 13.11 Cleanup

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Force-remove containers | `DELETE /containers/{id}` (`force=true`) | Kills + removes | **ECS**: `StopTask` + `DeregisterTaskDefinition` | **Cloud Run**: `jobs.delete` | **ACA**: `jobs.delete` |
| Remove network | `DELETE /networks/{id}` | Deletes bridge | **EC2**: `DeleteSecurityGroup`. **Cloud Map**: `DeleteNamespace` | Remove DNS zone + VPC tags | N/A (environment-scoped) |
| Prune orphan networks | `POST /networks/prune` (`filters={"label":["<runner-hash>"]}`) | Removes unused networks with matching label | **EC2**: Find + delete security groups with matching tag. **Cloud Map**: Find + delete namespaces | Find + remove tagged VPC resources | Find + remove tagged resources |

**Note:** The runner uses `docker rm --force` (NOT `docker stop`) for cleanup. It goes directly to kill + remove.

---

## 14. CI Runner Image Config Requirements

Both CI runners depend on image metadata from `GET /images/{name}/json` and `GET /containers/{id}/json`. This table shows exactly which fields each runner reads and how each backend provides them.

| Field | GitLab Runner Uses | GitHub Runner Uses | How Backend Gets It |
|---|---|---|---|
| `Config.Env` (array of `KEY=VALUE`) | Reads `HEALTHCHECK_TCP_PORT` for service port discovery | Reads `PATH` for exec command resolution | **Registry manifest API**: `GET /v2/<name>/blobs/<config-digest>` → parse `config.Env` |
| `Config.Cmd` (array) | Uses as container command (overrides with shell) | Discarded (replaced by `tail -f /dev/null` or action entrypoint) | Registry config blob |
| `Config.Entrypoint` (array) | Uses as container entrypoint | Discarded for jobs (replaced by `tail`); kept for container actions | Registry config blob |
| `Config.ExposedPorts` (map) | Reads for service health-check port discovery (sorted ascending, lowest TCP port used) | Not directly read | Registry config blob |
| `Config.Healthcheck` (object) | Not used (runner has its own TCP health check) | Checked for PRESENCE only — if present, runner polls `State.Health.Status` | Registry config blob (if image has `HEALTHCHECK` instruction) |
| `Config.WorkingDir` | Used if set | Used for container actions | Registry config blob |
| `Config.Labels` | Read for metadata | Not directly read | Registry config blob |
| `State.Status` | Polls until `!= "created"` for service readiness | Checks `"running"` after start | Backend tracks cloud task status |
| `State.Health.Status` | NOT used by runner | Polled for services with HEALTHCHECK (`"starting"` → `"healthy"` / `"unhealthy"`) | Agent runs health check cmd; reports to backend |
| `State.ExitCode` | Read after container exits | Not read (uses exec exit code) | Backend reads from cloud task completion |
| `NetworkSettings.IPAddress` | Read for service IP (legacy mode without `FF_NETWORK_PER_BUILD`) | Not directly read | Backend assigns virtual IP |
| `NetworkSettings.Networks.<name>.IPAddress` | Read for service IP (modern mode) | Not directly read | Backend assigns virtual IP per network |
| `NetworkSettings.Ports` | Not directly read | Read for service port mappings (populates `job.services.<name>.ports[N]`) | Backend tracks port assignments |

---

## 15. CI Runner Backend Compatibility

| Requirement | Docker | Memory | ECS | Cloud Run | ACA | Lambda | CR Func | Az Func |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| **GitLab Runner Compatible** | **Yes** | Yes† | **Yes** | **Yes** | **Yes** | No | No | No |
| **GitHub Actions Runner Compatible** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | No | No | No |
| **GitHub `act` Compatible** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| **gitlab-ci-local Compatible** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| Attach-before-start (GitLab) | Native | WASM | Agent + buffer | Agent + buffer | Agent + buffer | Reverse agent | Reverse agent | Reverse agent |
| Exec with stdin (GitHub) | Native | WASM exec | Forward agent | Forward agent | Forward agent | Reverse agent | Reverse agent | Reverse agent |
| Volume sharing (GitLab) | Named volumes | Symlinks | Infra-level | Infra-level | Infra-level | — | — | — |
| Service DNS aliases (both) | Embedded DNS | In-memory | In-memory | In-memory | In-memory | — | — | — |
| Health check exec (GitHub) | dockerd | WASM exec | Agent | Agent | Agent | — | — | — |
| Image `Config.Env` / PATH (GitHub) | Local store | Local state | Local state (opt: registry) | Local state (opt: registry) | Local state (opt: registry) | Local state | Local state | Local state |
| `tail -f /dev/null` keep-alive (GitHub) | Native `tail` | WASM context block | Agent `--keep-alive` | Agent `--keep-alive` | Agent `--keep-alive` | Agent `--keep-alive` | Agent `--keep-alive` | Agent `--keep-alive` |
| Docker socket mount (GitHub) | Host socket | Silently ignored | Silently ignored | Silently ignored | Silently ignored | Silently ignored | Silently ignored | Silently ignored |
| Container actions (GitHub) | Native lifecycle | WASM | Agent wraps entrypoint | Agent wraps entrypoint | Agent wraps entrypoint | Reverse agent | Reverse agent | Reverse agent |
| Network create (fatal, GitHub) | libnetwork | In-memory | In-memory | In-memory | In-memory | In-memory | In-memory | In-memory |
| Docker build | BuildKit | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser |
| Docker cp (archive) | FS access | WASM rootfs | Agent | Agent | Agent | Agent | Agent | Agent |
| Startup latency | <1s | <1s | 10-45s | 5-30s | 10-60s | 1-5s | 1-5s | 1-5s |
| Helper image load from tar (GitLab) | Local store | Local state | Local state | Local state | Local state | Local state | Local state | Local state |
| Cleanup parallelism (GitLab, 5min timeout) | Native | In-memory | Parallel `StopTask` | Parallel `DeleteJob` | Parallel `BeginDelete` | Parallel `DeleteFunction` | Parallel `DeleteFunction` | Parallel Delete |

† GitLab Runner E2E requires `SOCKERLESS_SYNTHETIC=1` because helper binaries (`gitlab-runner-helper`, `gitlab-runner-build`) can't run in WASM. GitHub `act` runner works without synthetic mode.

FaaS backends (Lambda, Cloud Run Functions, Azure Functions) now support exec/attach via reverse agent but still cannot run full CI runners due to timeout limits and lack of persistent shared volumes. They work with lightweight CI tools (`act`, `gitlab-ci-local`).
