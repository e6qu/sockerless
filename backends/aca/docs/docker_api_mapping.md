# Docker API Mapping: ACA Backend

The Azure Container Apps (ACA) backend translates Docker API calls into Azure Container Apps Jobs operations. A Docker "container" becomes a Container Apps Job + Execution.

## Container Lifecycle

### `POST /containers/create` — Create Container

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| What happens | Creates a container from an image on the local daemon | Stores container metadata locally (no Azure API call at create time) |
| Image | Must exist locally or be pullable | Stored for later use when job is created at start time |
| Entrypoint/Cmd | Stored as container config | Stored locally, merged into job spec at start time |
| Environment | Stored as container config | Stored locally, converted to `EnvironmentVar` at start time |
| Network | Joins specified network | Synthetic IP assigned from virtual bridge (172.17.0.x) |
| Return value | Container ID | Container ID (locally generated) |

**Why deferred creation:** Jobs are created at start time to support clean restarts — the old job is deleted before creating a new one.

### `POST /containers/{id}/start` — Start Container

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| What happens | Starts the container process | `Jobs.BeginCreateOrUpdate` → `Jobs.BeginStart` (creates Execution) |
| Job name | N/A | `sockerless-{containerID[:12]}` |
| Resources | Configurable CPU/memory | Fixed: 1.0 CPU, 2Gi memory |
| Trigger type | N/A | `Manual` (job is triggered explicitly) |
| Replica count | N/A | 1 |
| Agent (forward) | N/A | Polls `Jobs.Executions.NewListPager` until Running, agent: `{executionName}:9111` |
| Agent (reverse) | N/A | Waits for agent callback to `SOCKERLESS_CALLBACK_URL` (60s timeout) |
| Helper containers | N/A | Non-tail-dev-null commands auto-stop after 500ms |
| Background | Process runs in daemon | Goroutine polls execution for completion |
| Polling intervals | N/A | 2s (normal), 500ms (simulator mode) |
| Tags | N/A | `sockerless-container-id`, `managed-by: sockerless` |

**ACA-specific details:**
- Job created within Container Apps Environment (`SOCKERLESS_ACA_ENVIRONMENT`)
- Agent entrypoint wraps user command (forward or callback mode)
- Agent env vars: `SOCKERLESS_AGENT_TOKEN`, `SOCKERLESS_CONTAINER_ID`

### `POST /containers/{id}/stop` — Stop Container

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| What happens | Sends SIGTERM, then SIGKILL after timeout | Calls `Jobs.BeginStopExecution` |
| State after | `exited` with exit code | `exited` with exit code 0 |

### `POST /containers/{id}/kill` — Kill Container

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| What happens | Sends specified signal to process | Disconnects reverse agent, calls `Jobs.BeginStopExecution` |
| Signal support | Full POSIX signals | SIGKILL/9/KILL → exit code 137; others → exit code 0 |

### `DELETE /containers/{id}` — Remove Container

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| What happens | Removes container and its filesystem | Calls `Jobs.BeginDelete` (best-effort), removes local state |
| Force | Kills running container first | Stops execution + disconnects agent if running |

### `POST /containers/{id}/restart` — Restart Container

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| What happens | Stops then starts the container | Stops execution, deletes old job, creates new job + execution |

## Exec

Handled by core via the driver chain → Agent → Synthetic.

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| Execution | Runs in container namespace | Routed to agent inside Container App execution |
| Fallback | N/A | Synthetic driver if no agent connected |

## Images

### `POST /images/create` — Pull Image

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| What happens | Downloads image layers from registry | Creates synthetic image (no download) |
| Image config | Full manifest + layers | Optional real config via `SOCKERLESS_FETCH_IMAGE_CONFIG=true` |

### `POST /images/load` — Load Image

**Not implemented.** Returns `NotImplementedError`.

## Logs

### `GET /containers/{id}/logs` — Container Logs

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| Source | Container stdout/stderr | Azure Monitor Log Analytics via `Logs.QueryWorkspace` |
| Query | N/A | KQL: `ContainerAppConsoleLogs_CL \| where ContainerGroupName_s == "{jobName}"` |
| Workspace | N/A | `SOCKERLESS_ACA_LOG_ANALYTICS_WORKSPACE` (required for logs) |
| Follow mode | Real-time streaming | Polls Log Analytics every 2s |
| Timestamps | From Docker daemon | From query results (RFC3339Nano) |
| Stdout/stderr | Separate streams | All treated as stdout |
| Format | Docker multiplexed stream | Docker multiplexed stream (8-byte header per line) |
| Availability | Immediate | Depends on Log Analytics ingestion delay (can be 30s+) |

## Networks

Handled entirely by core. Synthetic in-memory tracking only.

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| Network creation | Creates real Docker network | In-memory tracking only |
| Inter-container networking | Via shared Docker network | Not available (jobs are isolated within the ACA Environment) |

## Volumes

Handled by core with ACA overrides for remove/prune.

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| Volume creation | Creates real Docker volume | In-memory tracking only |
| Persistent storage | Volumes persist | No persistent storage (Azure Files placeholder exists but unused) |

## Archive (Copy)

Handled by core via the driver chain → Agent → Synthetic.

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| Copy to/from | Direct filesystem access | Via agent or synthetic fallback |

## System

| Aspect | Vanilla Docker | ACA Backend |
|--------|---------------|-------------|
| Info | Real daemon info | Static: Driver=container-apps-jobs, OS=Azure Container Apps, 2 CPUs, 4GB RAM |

## Pause/Unpause

**Not supported.** Returns `NotImplementedError`. Azure Container Apps has no pause concept.

## CLI Command Mapping

| `docker` CLI command | Vanilla Docker | ACA Backend |
|---------------------|---------------|-------------|
| `docker create <image>` | Creates container locally | Stores metadata (no Azure call) |
| `docker start <id>` | Starts local process | `Jobs.BeginCreateOrUpdate` + `Jobs.BeginStart` |
| `docker stop <id>` | SIGTERM + SIGKILL | `Jobs.BeginStopExecution` |
| `docker kill <id>` | Send signal | `Jobs.BeginStopExecution` |
| `docker rm <id>` | Remove container + fs | `Jobs.BeginDelete` |
| `docker logs <id>` | Read from daemon | Log Analytics `QueryWorkspace` |
| `docker logs -f <id>` | Stream from daemon | Poll Log Analytics every 2s |
| `docker exec <id> <cmd>` | nsenter into container | Agent relay (forward or reverse) |
| `docker cp <src> <id>:<dst>` | Write to container layer | Agent relay or synthetic |
| `docker pull <image>` | Download layers | Synthetic (metadata only) |
| `docker build .` | Build from Dockerfile | Core Dockerfile parser (RUN is no-op) |
| `docker network create` | Create real network | In-memory only |
| `docker volume create` | Create real volume | In-memory only |
| `docker pause <id>` | Freeze cgroups | **Not supported** |
| `docker restart <id>` | Stop + start | Delete old job + create new |

## Summary: What's Not Supported

| Feature | Reason |
|---------|--------|
| Container pause/unpause | ACA has no pause capability |
| Image load from tar | ACA uses container registry images |
| Real Docker networks | Jobs isolated within ACA Environment |
| Real Docker volumes | Azure Files integration not yet implemented |
| Stderr separation | Log Analytics returns single text field |
| Resource customization | CPU/memory hardcoded (1.0 CPU / 2Gi) |
| Log availability | Log Analytics has ingestion delay (30s+) |
| Restart policies | Manual trigger only, no auto-restart |
