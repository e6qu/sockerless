# Docker API Mapping: Cloud Run Backend

The Cloud Run backend translates Docker API calls into Google Cloud Run Jobs operations. A Docker "container" becomes a Cloud Run Job + Execution.

## Container Lifecycle

### `POST /containers/create` — Create Container

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| What happens | Creates a container from an image on the local daemon | Stores container metadata locally (no GCP API call at create time) |
| Image | Must exist locally or be pullable | Stored for later use when job is created at start time |
| Entrypoint/Cmd | Stored as container config | Stored locally, used in job spec at start time |
| Environment | Stored as container config | Stored locally, converted to `EnvVar` at start time |
| Network | Joins specified network | Synthetic IP assigned from virtual bridge (172.17.0.x) |
| Return value | Container ID | Container ID (locally generated) |

**Why deferred creation:** Cloud Run Jobs are created at start time (not create time) to support clean restarts — the old job is deleted before creating a new one.

### `POST /containers/{id}/start` — Start Container

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| What happens | Starts the container process | `Jobs.CreateJob` → `Jobs.RunJob` (creates Execution) |
| Job name | N/A | `sockerless-{containerID[:12]}` |
| Resources | Configurable CPU/memory | Fixed: 1 CPU, 512 Mi memory |
| Timeout | N/A (runs until stopped) | 4 hours (hardcoded) |
| Retries | N/A | 0 (no retries) |
| Agent (forward) | N/A | Polls `Executions.GetExecution` until RUNNING, agent address: `{executionName}:9111` |
| Agent (reverse) | N/A | Waits for agent callback to `SOCKERLESS_CALLBACK_URL` (60s timeout) |
| VPC | N/A | Optional via `SOCKERLESS_GCR_VPC_CONNECTOR` with `Egress: ALL_TRAFFIC` |
| Helper containers | N/A | Non-tail-dev-null commands auto-stop after 500ms |
| Background | Process runs in daemon | Goroutine polls execution for completion, auto-stops container |
| Labels | N/A | `sockerless-container-id`, `managed-by: sockerless` |

### `POST /containers/{id}/stop` — Stop Container

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| What happens | Sends SIGTERM, then SIGKILL after timeout | Calls `Executions.CancelExecution` |
| State after | `exited` with exit code | `exited` with exit code 0 |

### `POST /containers/{id}/kill` — Kill Container

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| What happens | Sends specified signal to process | Disconnects reverse agent, calls `Executions.CancelExecution` |
| Signal support | Full POSIX signals | SIGKILL/9/KILL → exit code 137; others → exit code 0 |

### `DELETE /containers/{id}` — Remove Container

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| What happens | Removes container and its filesystem | Calls `Jobs.DeleteJob` (best-effort), removes local state |
| Force | Kills running container first | Cancels execution + disconnects agent if running |

### `POST /containers/{id}/restart` — Restart Container

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| What happens | Stops then starts the container | Stops execution, then creates a new job + execution |

## Exec

Handled by core via the driver chain → Agent → Synthetic.

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| Execution | Runs directly in container namespace | Routed to agent inside Cloud Run execution |
| Fallback | N/A | Synthetic driver if no agent connected |

## Images

### `POST /images/create` — Pull Image

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| What happens | Downloads image layers from registry | Creates synthetic image (no download) |
| Image config | Full manifest + layers | Optional real config via `SOCKERLESS_FETCH_IMAGE_CONFIG=true` |

### `POST /images/load` — Load Image

**Not implemented.** Returns `NotImplementedError`.

## Logs

### `GET /containers/{id}/logs` — Container Logs

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| Source | Container stdout/stderr from daemon | Cloud Logging via `LogAdmin.Entries` |
| Filter | N/A | `resource.type="cloud_run_job" AND resource.labels.job_name="{shortJobName}"` |
| Follow mode | Real-time streaming | Polls Cloud Logging every 5s (1s in simulator mode) |
| Timestamps | From Docker daemon | From log entry timestamps (RFC3339Nano) |
| Stdout/stderr | Separate streams | All treated as stdout (stream type 1) |
| Format | Docker multiplexed stream | Docker multiplexed stream (8-byte header per line) |
| Dedup | N/A | Tracks last timestamp to avoid duplicate entries in follow mode |

## Networks

Handled entirely by core. Synthetic in-memory tracking only.

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| Network creation | Creates real Docker network | In-memory tracking only |
| IP allocation | IPAM assigns real IPs | Synthetic IPs (172.17.0.x) |
| Inter-container networking | Via shared Docker network | Not directly available (VPC connector for egress) |

## Volumes

Handled by core with Cloud Run overrides for remove/prune.

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| Volume creation | Creates real Docker volume | In-memory tracking only |
| Persistent storage | Volumes persist | No persistent storage (GCS integration placeholder exists but unused) |

## Archive (Copy)

Handled by core via the driver chain → Agent → Synthetic.

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| Copy to/from | Direct filesystem access | Via agent inside execution or synthetic fallback |

## System

| Aspect | Vanilla Docker | Cloud Run Backend |
|--------|---------------|-------------------|
| Info | Real daemon info | Static: Driver=cloudrun-jobs, OS=Google Cloud Run, 2 CPUs, 4GB RAM |

## Pause/Unpause

**Not supported.** Returns `NotImplementedError`. Cloud Run has no pause concept.

## CLI Command Mapping

| `docker` CLI command | Vanilla Docker | Cloud Run Backend |
|---------------------|---------------|-------------------|
| `docker create <image>` | Creates container locally | Stores metadata (no GCP call) |
| `docker start <id>` | Starts local process | `Jobs.CreateJob` + `Jobs.RunJob` |
| `docker stop <id>` | SIGTERM + SIGKILL | `Executions.CancelExecution` |
| `docker kill <id>` | Send signal | `Executions.CancelExecution` |
| `docker rm <id>` | Remove container + fs | `Jobs.DeleteJob` |
| `docker logs <id>` | Read from daemon | Cloud Logging query |
| `docker logs -f <id>` | Stream from daemon | Poll Cloud Logging every 5s |
| `docker exec <id> <cmd>` | nsenter into container | Agent relay (forward or reverse) |
| `docker cp <src> <id>:<dst>` | Write to container layer | Agent relay or synthetic |
| `docker pull <image>` | Download layers | Synthetic (metadata only) |
| `docker build .` | Build from Dockerfile | Core Dockerfile parser (RUN is no-op) |
| `docker network create` | Create real network | In-memory only |
| `docker volume create` | Create real volume | In-memory only |
| `docker pause <id>` | Freeze cgroups | **Not supported** |
| `docker restart <id>` | Stop + start | Delete old job + create new job + run |

## Summary: What's Not Supported

| Feature | Reason |
|---------|--------|
| Container pause/unpause | Cloud Run has no pause capability |
| Image load from tar | Cloud Run uses Artifact Registry / Container Registry images |
| Real Docker networks | Jobs use VPC connector for egress only |
| Real Docker volumes | GCS integration not yet implemented |
| Stderr separation in logs | Cloud Logging entries are single-stream |
| Resource customization | CPU/memory hardcoded (1 CPU / 512 Mi) |
| Restart policies | Retries hardcoded to 0 |
