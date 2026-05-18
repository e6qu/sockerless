# Sockerless: Backend Comparisons — API Calls, CLI Commands, and Cloud Services

> **Date:** February 2026
>
> **Purpose:** Per-operation comparison of what Docker does natively vs. what each Sockerless backend uses to achieve the same result. For current per-backend coverage, see [FEATURE_MATRIX.md](../FEATURE_MATRIX.md).
>
> **Note:** Cloud backends derive state from cloud-native tags/labels (the cloud is the source of truth) and call cloud APIs for container lifecycle, logs, networking, and service discovery. ECS uses VPC Security Groups + Cloud Map, CloudRun uses Cloud DNS, and ACA uses NSG + in-process DNS. ECS and ACA also have cloud-native exec drivers.

---

## Legend

- **REST API Call**: The Docker Engine REST API endpoint
- **CLI Command**: The `docker` CLI command that maps to this endpoint
- **Docker Native**: What Docker Engine does internally (the reference behavior)
- **Sockerless Internal**: What the frontend sends to the backend via the internal API
- **Backend columns**: Which cloud services/APIs each backend uses to implement the operation

---

## 1. System Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Docker Backend | All Cloud Backends |
|---|---|---|---|---|---|
| `GET /_ping` | `docker ping` (implicit) | Returns `OK` from dockerd | Frontend handles directly (no backend call) | N/A | N/A |
| `HEAD /_ping` | *(implicit)* | Returns headers only | Frontend handles directly | N/A | N/A |
| `GET /version` | `docker version` | Returns dockerd version info | `GET /internal/v1/version` | Docker API `GET /version` | Returns Sockerless version with backend name |
| `GET /info` | `docker info` | Returns system info (OS, runtimes, storage driver, etc.) | `GET /internal/v1/info` | Docker API `GET /info` | Returns backend descriptor and cloud-derived capability metadata where implemented |
| `POST /auth` | `docker login` | Validates credentials against registry | `POST /internal/v1/auth` | Docker API `POST /auth` | Stores credentials in local state for later pull operations |
| `GET /events` | `docker events` | Streams real-time events | `GET /internal/v1/events` | Docker API `GET /events` | Backend event stream when implemented; otherwise explicit unsupported/no-event behavior |
| `GET /system/df` | `docker system df` | Returns disk usage statistics | `GET /internal/v1/system/df` | Docker API `GET /system/df` | Backend disk-usage implementation where available |

---

## 2. Image Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|
| `POST /images/create` (pull) | `docker pull nginx:latest` | Downloads layers from registry to local store; `X-Registry-Auth` header for private registries | `POST /internal/v1/images/pull` | Docker API `POST /images/create` (real pull) | Records ref in state; **ECS pulls at `RunTask` time** from ECR/Docker Hub | Records ref; **Lambda** pulls from **ECR** at `CreateFunction` time | Records ref; **Cloud Run** pulls at `CreateJob` time from **Artifact Registry**/GCR/Docker Hub | Records ref; **Cloud Run Functions** pulls at `CreateFunction` time from **Artifact Registry** | Records ref; **ACA** pulls at job creation time from **ACR**/Docker Hub | Records ref; **Azure Functions** pulls from **ACR** at Function App deploy |
| `GET /images/{name}/json` | `docker inspect nginx:latest` | Returns image metadata from local store (config, layers, size) | `GET /internal/v1/images/{name}` | Docker API `GET /images/{name}/json` | Registry manifest/config metadata | ECR image config metadata | Artifact Registry / registry metadata | Artifact Registry / registry metadata | ACR / registry metadata | ACR image config metadata |
| `POST /images/load` | `docker load -i image.tar` | Imports image from tar archive into local store | `POST /internal/v1/images/load` | Docker API `POST /images/load` | Pushes the loaded image to a configured cloud registry path or returns an explicit unsupported-operation error | Same as ECS | Same | Same | Same | Same |
| `POST /images/{name}/tag` | `docker tag nginx:latest myapp:v1` | Creates new tag reference in local store | `POST /internal/v1/images/{name}/tag` | Docker API `POST /images/{name}/tag` | Creates/records a registry-backed tag mapping | Same | Same | Same | Same | Same |
| `POST /build` | `docker build .` | Parses Dockerfile, executes RUN steps, creates image layers | `POST /internal/v1/images/build` | Docker API `POST /build` (real build) | Real builder path where configured, otherwise explicit unsupported-operation error | Same | Same | Same | Same | Same |
| `GET /images/json` | `docker images` | Lists all images from local store | `GET /internal/v1/images` | Docker API `GET /images/json` | Lists image refs known through registry-backed operations | Same | Same | Same | Same | Same |
| `DELETE /images/{name}` | `docker rmi nginx` | Removes image from local store | `DELETE /internal/v1/images/{name}` | Docker API `DELETE /images/{name}` | Removes the backend image reference and registry mapping where supported | Same | Same | Same | Same | Same |

---

## 3. Container Lifecycle

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|
| `POST /containers/create` | `docker create --name web nginx` | Creates container: allocates ID, stores config, creates filesystem layers, sets up networking config. Does NOT start. | `POST /internal/v1/containers` | Docker API `POST /containers/create` | Stores config; **`RegisterTaskDefinition`** | Stores config; **`CreateFunction`** (container image) | Stores config only (deferred job creation) | Stores config; **`CreateFunction`** (Docker runtime, synchronous 1-3 min) | Stores config only (deferred job creation) | Stores config; **Create App Service Plan + Function App** |
| `POST /containers/{id}/start` | `docker start web` | Starts container process: mounts volumes, configures network, runs entrypoint | `POST /internal/v1/containers/{id}/start` | Docker API `POST /containers/{id}/start` | **`RunTask`** (Fargate); polls until RUNNING | **`Invoke`** (async); agent dials back via reverse WebSocket | **`CreateJob` + `RunJob`** for one-shot containers or **Service create/update** for runner workloads; Service path waits for reverse-agent registration | HTTP POST invoke to underlying Service; agent dials back | **Job create/start** or **App create/update**; App path waits for reverse-agent registration | Start Function App; agent dials back |
| `GET /containers/{id}/json` | `docker inspect web` | Returns full container metadata: state, config, network settings, mounts, health status | `GET /internal/v1/containers/{id}` | Docker API `GET /containers/{id}/json` | ECS task/task-definition/tags | Lambda function/invocation state | Cloud Run Job/Service state | Cloud Functions + underlying Service state | ACA Job/App state | Function App state |
| `GET /containers/json` | `docker ps` / `docker ps -a` | Lists containers from daemon state, filtered by labels/status/id/name | `GET /internal/v1/containers` | Docker API `GET /containers/json` | ECS `ListTasks`/`DescribeTasks` + tags | Lambda functions tagged by sockerless | Cloud Run Jobs/Services with labels | Cloud Functions + underlying Services with labels | ACA Jobs/Apps with tags | Function Apps with tags |
| `POST /containers/{id}/stop` | `docker stop web` | Sends SIGTERM, waits timeout, then SIGKILL | `POST /internal/v1/containers/{id}/stop` | Docker API `POST /containers/{id}/stop` | **`StopTask`** | Disconnects reverse agent | **Cancel Execution** | Disconnects reverse agent | **`BeginStopExecution`** | Stop Function App |
| `POST /containers/{id}/kill` | `docker kill web` | Sends specified signal (default SIGKILL) immediately | `POST /internal/v1/containers/{id}/kill` | Docker API `POST /containers/{id}/kill` | **`StopTask`** | Disconnects reverse agent | **Cancel Execution** | Disconnects reverse agent | **`BeginStopExecution`** | Disconnects reverse agent |
| `POST /containers/{id}/wait` | `docker wait web` | Blocks until container exits; returns exit code | `WS /internal/v1/containers/{id}/wait` | Docker API `POST /containers/{id}/wait` | Polls ECS task stop state | Waits for invocation/agent completion | Polls Cloud Run execution or recorded Service result | Waits for invocation/agent completion | Polls ACA execution/App result | Waits for invocation/agent completion |
| `DELETE /containers/{id}` | `docker rm web` / `docker rm -f web` | Removes container: deletes filesystem, network endpoints, state. If force: kills first. | `DELETE /internal/v1/containers/{id}` | Docker API `DELETE /containers/{id}` | **`StopTask`** (if force) + **`DeregisterTaskDefinition`** | **`DeleteFunction`** | **`DeleteJob`** | **`DeleteFunction`** | **`BeginDelete`** (Job) | **Delete Function App + App Service Plan** |

---

## 4. Container I/O

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|
| `GET /containers/{id}/logs` | `docker logs web` | Reads from container's log driver (json-file, journald, etc.). Returns multiplexed stream (8-byte headers). | `GET /internal/v1/containers/{id}/logs` (one-shot) or `WS .../logs/stream` (follow) | Docker API `GET /containers/{id}/logs` | **CloudWatch Logs**: `GetLogEvents` (one-shot) or `FilterLogEvents` with polling (follow) | **CloudWatch Logs**: `GetLogEvents` for function execution logs | **Cloud Logging**: `entries.list` via Log Admin API | **Cloud Logging**: `entries.list` via Log Admin API | **Azure Monitor**: Log Analytics `QueryWorkspace` | **Azure Monitor**: Log Analytics query |
| `POST /containers/{id}/attach` | `docker attach web` | Hijacks HTTP connection; bidirectional multiplexed stream (stdin→container, container stdout/stderr→client with 8-byte headers) | Backend hijacks connection; dispatches through StreamDriver chain | Docker API `POST /containers/{id}/attach` (native) | ECS SSM / agent transport | Reverse-agent WebSocket | Reverse-agent WebSocket for Service-backed workloads | Reverse-agent WebSocket | Reverse-agent WebSocket for App-backed workloads | Reverse-agent WebSocket |

---

## 5. Exec Operations

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Docker Backend | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|---|---|---|
| `POST /containers/{id}/exec` | `docker exec web sh` (create phase) | Creates exec instance: stores cmd/env/workdir config, allocates exec ID | `POST /internal/v1/containers/{id}/exec` | Docker API `POST /containers/{id}/exec` | Stores exec config in backend state, returns exec ID | Stores exec config (reverse agent handles start) | Stores exec config | Stores exec config (reverse agent handles start) | Stores exec config | Stores exec config (reverse agent handles start) |
| `POST /exec/{id}/start` | `docker exec web sh` (start phase) | Hijacks connection; forks process inside container's namespaces; streams stdin/stdout/stderr with multiplexed framing | Backend hijacks connection; dispatches through ExecDriver (Agent) | Docker API `POST /exec/{id}/start` (native hijack) | **ECS SSM / agent transport** → exec → stream bridge | **Reverse agent** (agent already connected via callback) → exec → stream bridge | **Reverse agent** → exec → stream bridge | **Reverse agent** → exec → stream bridge | **Reverse agent** → exec → stream bridge | **Reverse agent** → exec → stream bridge |
| `GET /exec/{id}/json` | `docker inspect <exec-id>` | Returns exec instance state (running, exit code, pid) | `GET /internal/v1/exec/{id}` | Docker API `GET /exec/{id}/json` | Returns exec state from local store | Same | Same | Same | Same | Same |

---

## 6. Network Operations

> **Implementation note:** Current cloud networking behavior is backend-specific. ECS maps Docker networks to VPC Security Groups plus Cloud Map, Cloud Run maps service discovery through Cloud DNS / Service materialization where applicable, and ACA maps to managed-environment networking plus Private DNS. See [FEATURE_MATRIX.md](../FEATURE_MATRIX.md) and [specs/CLOUD_RESOURCE_MAPPING.md](CLOUD_RESOURCE_MAPPING.md) for the current driver matrix.

| REST API Call | CLI Command | Docker Native | Sockerless Internal | All Backends (except Docker) | Docker Backend |
|---|---|---|---|---|---|
| `POST /networks/create` | `docker network create mynet` | Creates bridge/overlay network via libnetwork. Allocates subnet, sets up iptables rules, DNS resolver. | `POST /internal/v1/networks` | Backend cloud network driver creates or records the cloud isolation primitive | Docker API `POST /networks/create` |
| `GET /networks` | `docker network ls` | Lists all networks from libnetwork state | `GET /internal/v1/networks` | Backend network driver lists known cloud-backed networks | Docker API `GET /networks` |
| `GET /networks/{id}` | `docker network inspect mynet` | Returns network details including connected containers, IPAM config, driver | `GET /internal/v1/networks/{id}` | Backend network driver returns cloud-derived network details | Docker API `GET /networks/{id}` |
| `POST /networks/{id}/connect` | `docker network connect mynet web` | Connects container to network | `POST /internal/v1/networks/{id}/connect` | Backend network driver attaches the cloud resource or records materialization intent | Docker API `POST /networks/{id}/connect` |
| `POST /networks/{id}/disconnect` | `docker network disconnect mynet web` | Detaches container from network; removes veth pair, DNS entry | `POST /internal/v1/networks/{id}/disconnect` | Backend network driver detaches cloud mapping where supported | Docker API `POST /networks/{id}/disconnect` |
| `DELETE /networks/{id}` | `docker network rm mynet` | Deletes network: removes bridge, iptables rules, DNS config | `DELETE /internal/v1/networks/{id}` | Backend network driver deletes the cloud mapping | Docker API `DELETE /networks/{id}` |
| `POST /networks/prune` | `docker network prune` | Removes all unused networks (no connected containers) | `POST /internal/v1/networks/prune` | Backend network driver prunes unused mappings | Docker API `POST /networks/prune` |

---

## 7. Volume Operations

> **Implementation note:** Cloud volume behavior is backend-specific and must map to real cloud storage or fail loudly. ECS/Lambda use EFS where configured, Cloud Run/GCF use GCS-backed storage or memory tmpfs where supported, and ACA/AZF use Azure Files where configured.

| REST API Call | CLI Command | Docker Native | Sockerless Internal | Docker Backend | All Cloud Backends |
|---|---|---|---|---|---|
| `POST /volumes/create` | `docker volume create cache` | Creates named volume on local filesystem | `POST /internal/v1/volumes` | Docker API `POST /volumes/create` | Backend storage driver creates or records the real cloud storage mapping |
| `GET /volumes` | `docker volume ls` | Lists all named volumes | `GET /internal/v1/volumes` | Docker API `GET /volumes` | Backend storage driver lists configured cloud-backed volumes |
| `GET /volumes/{name}` | `docker volume inspect cache` | Returns volume metadata | `GET /internal/v1/volumes/{name}` | Docker API `GET /volumes/{name}` | Backend storage driver returns cloud storage metadata |
| `DELETE /volumes/{name}` | `docker volume rm cache` | Deletes volume | `DELETE /internal/v1/volumes/{name}` | Docker API `DELETE /volumes/{name}` | Backend storage driver deletes the cloud mapping where supported |
| `POST /volumes/prune` | `docker volume prune` | Removes unused volumes | `POST /internal/v1/volumes/prune` | Docker API `POST /volumes/prune` | Backend storage driver prunes unused cloud mappings |

---

## 8. Bind Mount and Archive Mapping

### 8.1 Bind Mounts

Bind mounts (`-v /host/path:/container/path`) are specified in `POST /containers/create` under `HostConfig.Binds`. They are not separate API calls but are critical for CI runners.

| Bind Mount Use | Docker Native | Cloud Backends |
|---|---|---|
| `/builds` (shared between helper + build containers) | Same host directory mounted into multiple containers | Volume mounts configured at infrastructure level (EFS/GCS/Azure Files) |
| `/cache` (persistent across jobs) | Named volume or host directory | Same infrastructure-level mounts |
| Docker socket (`/var/run/docker.sock`) | Direct host socket passthrough | Unsupported unless an explicit backend implementation provides a real socket contract |
| Host path bind mounts | Direct host path access | Mapped to cloud storage paths |

### 8.2 Archive Operations (`docker cp`)

| REST API Call | CLI Command | Docker Native | Cloud Backends |
|---|---|---|---|
| `PUT /containers/{id}/archive` | `docker cp local_path container:/path` | Extracts tar to container filesystem | Via agent FilesystemDriver (forward/reverse agent writes to host filesystem) |
| `HEAD /containers/{id}/archive` | *(used by `docker cp` internally)* | Returns file stat as base64 JSON header | Via agent FilesystemDriver |
| `GET /containers/{id}/archive` | `docker cp container:/path local_path` | Returns tar of path from container | Via agent FilesystemDriver |

---

## 9. Health Check Flow

Health checks are implemented in `backends/core/health.go` (~200 lines, 6 unit tests). They affect `GET /containers/{id}/json` responses and are used by CI runners and Docker Compose for readiness polling.

| Aspect | Docker Native | Cloud Backends (Agent) |
|---|---|---|
| Health check source | `HEALTHCHECK` in Dockerfile or compose `healthcheck:` | Same |
| Health check execution | dockerd runs command inside container periodically | `StartHealthCheck` goroutine runs exec via agent on interval |
| Health check parsing | `parseHealthcheckCmd`: NONE → nil, CMD → args[1:], CMD-SHELL → `["sh", "-c", joined]` | Same |
| Status reported in | `State.Health.Status` (`starting` → `healthy` / `unhealthy`) | Same |
| Status tracking | `HealthChecks` map on Store holds `context.CancelFunc` per container | Same |
| Stop behavior | Cancels health check context | Same |
| Output capture | Via exec | Same |

---

## 10. Agent Operations (Not Docker API — Internal)

These are not Docker REST API calls but show how the agent is used internally by each backend.

### 10.1 ECS SSM / Agent Transport

| Operation | When Triggered | ECS |
|---|---|---|
| Exec transport | `POST /exec/{id}/start` | Backend uses ECS ExecuteCommand / SSM and bridges the Docker hijack stream |
| Attach/archive/process helpers | Docker attach/cp/top/stat/diff surfaces | Backend uses the configured ECS cloud access path and fails loudly when the required cloud capability is absent |

### 10.2 Reverse Agent (FaaS / Service / App Backends)

| Operation | When Triggered | Lambda | Cloud Run Service / GCF | ACA App | Azure Functions |
|---|---|---|---|---|---|
| Bootstrap materialization | Create/start path | Function image includes bootstrap | Service/Function image is overlay-wrapped | App image is overlay-wrapped | Function App image is overlay-wrapped |
| Agent callback | After workload starts | Agent dials back to backend via `SOCKERLESS_CALLBACK_URL` (WebSocket) | Same | Same | Same |
| Registration | On callback connect | `AgentRegistry.Prepare(id)` pre-creates done channel before invoke goroutine starts | Same | Same | Same |
| Exec via agent | `POST /exec/{id}/start` | Backend uses existing reverse WebSocket connection → exec → stream bridge | Same | Same | Same |
| Lifecycle | Agent disconnect or cloud completion | Backend records real invocation/Service/App result | Same | Same | Same |

---

## 11. Capability Summary per Backend

> **Note:** This is a high-level summary. For the current per-backend matrix (cloud-native networking, exec, service discovery), see [FEATURE_MATRIX.md](../FEATURE_MATRIX.md).

| Capability | Docker | ECS | Lambda | Cloud Run | CR Functions | ACA | Azure Functions |
|---|---|---|---|---|---|---|---|
| `POST /containers/create` | `dockerd` | Pending create until `RunTask` | Pending create until `CreateFunction` | Pending create until Job/Service materialization | Pending create until Function/Service materialization | Pending create until Job/App materialization | Pending create until Function App materialization |
| `POST /containers/{id}/start` | `dockerd` | `RunTask` | `Invoke` (async) | `CreateJob` + `RunJob` | HTTP POST invoke | `BeginCreateOrUpdate` + `BeginStart` | Start Function App |
| `GET /containers/{id}/json` | `dockerd` | ECS cloud state | Lambda cloud state | Cloud Run cloud state | Cloud Functions / underlying Service state | ACA cloud state | Azure Functions cloud state |
| `GET /containers/{id}/logs` | Log driver | **CloudWatch** | **CloudWatch** | **Cloud Logging** | **Cloud Logging** | **Azure Monitor** | **Azure Monitor** |
| `POST /containers/{id}/attach` | Hijack | **SSM / agent path** | **Reverse agent** | **Reverse agent** | **Reverse agent** | **Reverse agent** | **Reverse agent** |
| `POST /exec/{id}/start` | Hijack | **SSM ExecuteCommand** | **Reverse agent** | **Reverse agent** | **Reverse agent** | **Reverse agent** | **Reverse agent** |
| `POST /containers/{id}/stop` | `dockerd` | `StopTask` | Disconnect agent | Cancel Execution | Disconnect agent | `BeginStopExecution` | Stop Function App |
| `DELETE /containers/{id}` | `dockerd` | `DeregisterTaskDef` | `DeleteFunction` | `DeleteJob` | `DeleteFunction` | `BeginDelete` (Job) | Delete App + Plan |
| `POST /networks/create` | `libnetwork` | **VPC security groups + Cloud Map** | Not supported as native peer DNS | **Cloud DNS / Service materialization** | Service materialization | **ACA managed environment / private DNS** | Not supported as native peer DNS |
| `POST /volumes/create` | Local FS | **EFS** | **EFS** | **GCS-backed storage** | **GCS-backed storage** | **Azure Files** | **Azure Files** |
| `POST /build` | BuildKit | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser | Dockerfile parser |
| Archive (docker cp) | FS access | Agent FS | Agent FS | Agent FS | Agent FS | Agent FS | Agent FS |

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
| Create job container | `POST /containers/create` (`Entrypoint: ["tail"]`, `Cmd: ["-f", "/dev/null"]`, network, labels, `-v /var/run/docker.sock`, env vars) | Allocates container; stores config with overridden entrypoint | Pending create + Task Definition. **EFS** for configured volumes. Docker socket bind is unsupported unless explicitly provided by a backend contract | Pending create + Cloud Run Service overlay for runner workloads | Pending create + ACA App overlay for runner workloads |
| Create service containers | `POST /containers/create` (per service, with `HealthCheck` if image defines one, same network) | Allocates container | Stores config; registers Task Definition with health check | Same | Same |

**`tail -f /dev/null` handling:** The runner ALWAYS overrides entrypoint to `tail -f /dev/null`. Cloud backends detect this pattern and use the agent's `--keep-alive` mode instead — the agent stays alive and serves exec/attach without running a child process. Image must contain `tail` in its PATH for Docker, but sockerless never actually runs `tail`.

**Docker socket mount:** The runner always adds `-v /var/run/docker.sock:/var/run/docker.sock`. Cloud backends must either provide a real socket contract or reject the mount clearly; they must not silently present a non-functional socket.

### 13.5 Container Start and Verification

| Step | Docker API Call | Docker Native | ECS | Cloud Run | ACA |
|---|---|---|---|---|---|
| Start container | `POST /containers/{id}/start` | Starts process (<1s) | **ECS**: `RunTask`; blocks until cloud task readiness | **Cloud Run**: Service create/update for runner workloads; blocks until reverse-agent registration | **ACA**: App create/update for runner workloads; blocks until reverse-agent registration |
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
| Start exec | `POST /exec/{id}/start` (`Detach: false`) → hijacked connection | Forks process; bidirectional multiplexed stream | Frontend uses ECS SSM / agent transport and bridges the hijacked Docker stream | Frontend uses the registered reverse-agent WebSocket from the Cloud Run Service | Frontend uses the registered reverse-agent WebSocket from the ACA App |
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
| `NetworkSettings.IPAddress` | Read for service IP (legacy mode without `FF_NETWORK_PER_BUILD`) | Not directly read | Backend reports the cloud-resolved service address where available |
| `NetworkSettings.Networks.<name>.IPAddress` | Read for service IP (modern mode) | Not directly read | Backend reports per-network cloud-resolved service addresses where available |
| `NetworkSettings.Ports` | Not directly read | Read for service port mappings (populates `job.services.<name>.ports[N]`) | Backend tracks port assignments |

---

## 15. CI Runner Backend Compatibility

| Requirement | Docker | ECS | Cloud Run | ACA | Lambda | CR Func | Az Func |
|---|:---:|:---:|:---:|:---:|:---:|:---:|:---:|
| **GitLab Runner Compatible** | **Yes** | **Yes** | **Yes** | **Yes** | No | No | No |
| **GitHub Actions Runner Compatible** | **Yes** | **Yes** | **Yes** | **Yes** | No | No | No |
| **GitHub `act` Compatible** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| **gitlab-ci-local Compatible** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** | **Yes** |
| Attach-before-start (GitLab) | Native | Agent + buffer | Agent + buffer | Agent + buffer | Reverse agent | Reverse agent | Reverse agent |
| Exec with stdin (GitHub) | Native | SSM / agent transport | Reverse agent | Reverse agent | Reverse agent | Reverse agent | Reverse agent |
| Volume sharing (GitLab) | Named volumes | Infra-level | Infra-level | Infra-level | — | — | — |
| Service DNS aliases (both) | Embedded DNS | Cloud Map | Cloud DNS / Service URL | ACA Private DNS | — | — | — |
| Health check exec (GitHub) | dockerd | Agent / SSM path | Reverse agent | Reverse agent | — | — | — |
| Image `Config.Env` / PATH (GitHub) | Local store | Registry config | Registry config | Registry config | Registry config | Registry config | Registry config |
| `tail -f /dev/null` keep-alive (GitHub) | Native `tail` | Cloud task/agent path | Service reverse-agent bootstrap | App reverse-agent bootstrap | Reverse-agent bootstrap | Reverse-agent bootstrap | Reverse-agent bootstrap |
| Docker socket mount (GitHub) | Host socket | Unsupported unless explicitly wired | Unsupported unless explicitly wired | Unsupported unless explicitly wired | Unsupported unless explicitly wired | Unsupported unless explicitly wired | Unsupported unless explicitly wired |
| Container actions (GitHub) | Native lifecycle | Agent wraps entrypoint | Agent wraps entrypoint | Agent wraps entrypoint | Reverse agent | Reverse agent | Reverse agent |
| Network create (fatal, GitHub) | libnetwork | VPC SG + Cloud Map | Cloud DNS / Service mapping | ACA environment + Private DNS | Unsupported unless explicitly mapped | Unsupported unless explicitly mapped | Unsupported unless explicitly mapped |
| Docker build | BuildKit | Real builder path or explicit unsupported error | Real builder path or explicit unsupported error | Real builder path or explicit unsupported error | Real builder path or explicit unsupported error | Real builder path or explicit unsupported error | Real builder path or explicit unsupported error |
| Docker cp (archive) | FS access | Agent | Agent | Agent | Agent | Agent | Agent |
| Startup latency | <1s | 10-45s | 5-30s | 10-60s | 1-5s | 1-5s | 1-5s |
| Helper image load from tar (GitLab) | Local store | Registry-backed load/push | Registry-backed load/push | Registry-backed load/push | Registry-backed load/push | Registry-backed load/push | Registry-backed load/push |
| Cleanup parallelism (GitLab, 5min timeout) | Native | Parallel `StopTask` | Parallel `DeleteJob` | Parallel `BeginDelete` | Parallel `DeleteFunction` | Parallel `DeleteFunction` | Parallel Delete |

FaaS backends (Lambda, Cloud Run Functions, Azure Functions) support exec/attach via reverse agent but cannot run full CI runners due to timeout limits and lack of persistent shared volumes. They work with lightweight CI tools (`act`, `gitlab-ci-local`).
