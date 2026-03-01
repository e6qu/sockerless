# Azure Backend Gap Analysis (ACA + Azure Functions)

Comparing the backends against:
1. **Docker API coverage** — what Docker operations work end-to-end on cloud
2. **Cloud SDK feature usage** — what Azure features exist but aren't leveraged

---

## Part A: Docker API Coverage on ACA Backend

### 1. Container Lifecycle

| Docker API | ACA Backend | Status | Notes |
|---|---|---|---|
| `POST /containers/create` | Stores config; creates ACA Job via `BeginCreateOrUpdate` | **FULL** | Maps image, cmd, env, resources, tags |
| `POST /containers/{id}/start` | Calls `BeginStart` on ACA Job (creates execution) | **FULL** | Polls Location header for execution status |
| `POST /containers/{id}/stop` | Calls `BeginStopExecution` | **FULL** | |
| `POST /containers/{id}/kill` | Calls `BeginStopExecution` | **FULL** | Same as stop — no signal granularity |
| `DELETE /containers/{id}` | Calls `BeginDelete` | **FULL** | |
| `POST /containers/{id}/wait` | Polls execution status via `NewListPager` | **FULL** | Checks Running/Succeeded/Failed/Stopped/Degraded |
| `POST /containers/{id}/restart` | Handled by core (stop + start) | **FULL** | |
| `POST /containers/{id}/pause` | **NOT SUPPORTED** | **GAP** | ACA Jobs have no pause concept |
| `POST /containers/{id}/unpause` | **NOT SUPPORTED** | **GAP** | |
| `POST /containers/{id}/update` | **NOT SUPPORTED** | **GAP** | Cannot update running execution |
| `POST /containers/{id}/rename` | Handled by core (local state only) | **PARTIAL** | Renames locally; ACA Job name unchanged |

### 2. Container Inspection & Streaming

| Docker API | ACA Backend | Status | Notes |
|---|---|---|---|
| `GET /containers/{id}/json` | Handled by core (local state) | **FULL** | |
| `GET /containers/json` | Handled by core | **FULL** | |
| `GET /containers/{id}/logs` | Override: queries Log Analytics via KQL | **FULL** | Queries `ContainerAppConsoleLogs_CL` table |
| `POST /containers/{id}/attach` | **Via agent only** | **PARTIAL** | Requires forward/reverse agent |
| `GET /containers/{id}/top` | Synthetic | **STUB** | |
| `GET /containers/{id}/stats` | Synthetic | **STUB** | |
| `GET /containers/{id}/changes` | Stubbed (empty) | **STUB** | |
| `GET /containers/{id}/export` | **Via agent only** | **PARTIAL** | |

### 3. Exec

| Docker API | ACA Backend | Status | Notes |
|---|---|---|---|
| `POST /containers/{id}/exec` | Creates exec in store | **FULL** | |
| `POST /exec/{id}/start` | **Via agent only** | **PARTIAL** | No native exec in ACA Jobs |
| `GET /exec/{id}/json` | Handled by core | **FULL** | |

### 4. Images

| Docker API | ACA Backend | Status | Notes |
|---|---|---|---|
| `POST /images/create` (pull) | Override: may handle ACR auth | **FULL** | |
| All other image ops | Handled by core (synthetic) | **FULL** | |

### 5. Docker API Operations Not Possible on ACA

| Feature | Why |
|---|---|
| `pause` / `unpause` | ACA Jobs don't support pause |
| `update` | Cannot modify running execution resources |
| Signal-specific `kill` | Only stop; no SIGTERM/SIGUSR1 granularity |
| `commit` | No filesystem snapshot |
| Privileged mode | Not supported on ACA |
| Host networking | ACA manages VNet integration |
| Device access | Not supported |

---

## Part B: Docker API Coverage on Azure Functions Backend

### 1. Container Lifecycle

| Docker API | Azure Functions Backend | Status | Notes |
|---|---|---|---|
| `POST /containers/create` | Calls `BeginCreateOrUpdate` (creates Function App + App Service Plan) | **FULL** | Uses LinuxFxVersion for Docker image |
| `POST /containers/{id}/start` | HTTP POST to function endpoint | **FULL** | Async invocation via `/api/function` |
| `POST /containers/{id}/stop` | No-op (function runs to completion) | **PARTIAL** | |
| `POST /containers/{id}/kill` | Disconnects agent | **PARTIAL** | |
| `DELETE /containers/{id}` | Calls `Delete` on Function App | **FULL** | |
| `POST /containers/{id}/wait` | Handled by core | **PARTIAL** | Waits for agent disconnect |
| `POST /containers/{id}/pause` | **NOT SUPPORTED** | **GAP** | |
| `POST /containers/{id}/unpause` | **NOT SUPPORTED** | **GAP** | |

### 2. Container Inspection & Streaming

| Docker API | Azure Functions Backend | Status | Notes |
|---|---|---|---|
| `GET /containers/{id}/logs` | Override: queries Log Analytics (`AppTraces` table) | **FULL** | |
| Other inspection ops | Synthetic/agent-dependent | **PARTIAL** | |

### 3. Docker API Operations Not Possible on Azure Functions

Same as Lambda — stateless, event-driven, no persistent filesystem, no attach,
max timeout (varies by plan), no port mapping.

---

## Part C: Cloud SDK Feature Gaps

### 1. ACA Features Available But Not Used

| Azure Feature | Description | Could Enable |
|---|---|---|
| **Container Apps (Services)** | Long-running, auto-scaling HTTP services (not Jobs) | Persistent container workloads |
| **Multi-container support** | Init containers + sidecar containers | Multi-container deployments |
| **Container Apps Exec** | `az containerapp exec` for interactive shell | `docker exec` without custom agent |
| **Built-in log streaming** | `az containerapp logs show --follow` via WebSocket | Real-time logs without Log Analytics latency |
| **Health probes** | Startup, liveness, readiness probes | Docker HEALTHCHECK equivalent |
| **Dapr integration** | Sidecar for microservice communication | Service-to-service networking |
| **KEDA scaling** | Event-driven autoscaling (CPU, HTTP, custom) | Auto-scaling containers |
| **Custom domains + TLS** | Managed certificates, custom domains | HTTPS endpoints |
| **Managed Identity** | System/user-assigned identity for Azure resource access | Secure credential-free auth |
| **Secret management** | Container Apps Secrets (env vars or volume mounts) | Docker secret equivalent |
| **Volume mounts** | Azure Files, ephemeral storage | Persistent volumes |
| **Revision management** | Traffic splitting between revisions | Canary deployments |
| **Session affinity** | Sticky sessions for Services | Stateful containers |
| **GPU support** | GPU workloads (preview) | ML/AI containers |
| **Container Apps Environment** | Shared logging, VNET, Dapr config | Environment management |

### 2. Azure Functions Features Available But Not Used

| Azure Feature | Description | Could Enable |
|---|---|---|
| **Durable Functions** | Stateful workflows with orchestration | Complex multi-step containers |
| **Event Grid triggers** | React to Azure events | Event-driven container execution |
| **Timer triggers** | Cron-based scheduling | Scheduled containers |
| **Queue triggers** | Process messages from Storage Queue or Service Bus | Message-driven containers |
| **Binding expressions** | Declarative I/O bindings | Simplified data access |
| **Deployment slots** | Staging/production swap | Zero-downtime deployments |
| **Premium plan** | Pre-warmed instances, VNet integration | Better performance |
| **Container Apps hosting** | Run Azure Functions on Container Apps | Unified platform |

### 3. Log Analytics Features Available But Not Used

| Azure Feature | Description | Could Enable |
|---|---|---|
| **Advanced KQL** | Complex queries with aggregations, joins, time series | Richer log analysis |
| **Alerts** | Log-based alerts via Azure Monitor | Automated notifications |
| **Workbooks** | Interactive dashboards from log data | Visual monitoring |
| **Export** | Continuous export to Storage, Event Hub | Log archival |
| **Diagnostic settings** | Auto-route platform logs to workspace | More log sources |

---

## Part D: Summary of Critical Gaps

### Docker API Gaps

1. **`docker exec` without agent** — ACA has a native `az containerapp exec`
   command (uses WebSocket tunneling) that could replace the custom agent.
   This is the single most impactful gap for Docker compatibility.

2. **`docker pause/unpause`** — no ACA equivalent. Must be documented as
   unsupported.

3. **`docker attach` without agent** — agent-dependent. ACA's exec/log
   streaming could partially replace this.

4. **`docker stats` fidelity** — returns synthetic data. Azure Monitor Container
   Insights provides real metrics (CPU, memory, network) for Container Apps.

5. **`docker stop` on Azure Functions** — cannot stop a running function
   invocation. Function runs until completion or timeout.

6. **Log latency** — Log Analytics has minutes of ingestion delay. The backend's
   log streaming may show stale data. ACA's built-in log streaming
   (`az containerapp logs show --follow`) uses a direct WebSocket connection
   that's near-real-time.

### Cloud Feature Gaps

1. **Container Apps Services** — the backend only uses Jobs (one-shot).
   Services would enable long-running containers with auto-scaling, ingress,
   custom domains — the primary Docker use case.

2. **Container Apps Exec** — native exec would eliminate the agent dependency.
   Uses WebSocket tunneling to the container's console.

3. **Container Apps log streaming** — the real-time log streaming API would
   provide much lower latency than Log Analytics queries.

4. **Azure Files volume mounts** — native persistent volume support for
   Container Apps would improve Docker volume compatibility.

5. **Health probes** — startup/liveness/readiness probes would enable
   Docker HEALTHCHECK parity.

6. **Managed Identity** — would provide secure, credential-free access to
   Azure resources from containers without manual secret management.

7. **Container Apps Secrets** — would provide `docker secret`-like functionality
   with built-in secret management and rotation.
