# Docker API Mapping: Cloud Run Functions Backend

The Cloud Run Functions (GCF 2nd gen) backend translates Docker API calls into Google Cloud Functions operations. A Docker "container" becomes a Cloud Function with Docker runtime, invoked via HTTP.

## Container Lifecycle

### `POST /containers/create` — Create Container

| Aspect | Vanilla Docker | GCF Backend |
|--------|---------------|-------------|
| What happens | Creates a container from an image on the local daemon | Calls `Functions.CreateFunction` and waits for operation to complete |
| Image | Must exist locally or be pullable | Used as function source with `BuildConfig.Runtime = "docker"` |
| Entrypoint/Cmd | Stored as container config | Wrapped with agent callback entrypoint if reverse agent enabled |
| Environment | Stored as container config | Set as `ServiceConfig.EnvironmentVariables` |
| Working directory | Stored as container config | Not directly mapped (function runtime handles it) |
| Memory | Configurable limits | `SOCKERLESS_GCF_MEMORY` (default: 1Gi) |
| CPU | Configurable limits | `SOCKERLESS_GCF_CPU` (default: 1) |
| Timeout | N/A | `SOCKERLESS_GCF_TIMEOUT` (default: 3600s / 1 hour) |
| Name | Container name | Function name: `skls-{containerID[:12]}` |
| Return value | Container ID | Container ID (locally generated) |

**GCF-specific details:**
- Function creation is synchronous (waits for long-running operation to complete)
- Function URL extracted from `ServiceConfig.Uri` response
- Agent env vars: `SOCKERLESS_AGENT_TOKEN`, `SOCKERLESS_CONTAINER_ID`, `SOCKERLESS_AGENT_CALLBACK_URL`
- Service account configurable via `SOCKERLESS_GCF_SERVICE_ACCOUNT`

### `POST /containers/{id}/start` — Start Container

| Aspect | Vanilla Docker | GCF Backend |
|--------|---------------|-------------|
| What happens | Starts the container process | HTTP POST to function URL (async invocation) |
| Blocking | Returns immediately | Returns immediately; invoke runs in background goroutine |
| Agent mode | N/A | Reverse agent only — waits for callback (60s timeout) |
| Helper containers | N/A | Non-tail-dev-null commands auto-stop after 500ms |
| Exit handling | Process exits naturally | Goroutine waits for HTTP response, then 30 min for agent disconnect |
| Pre-create channel | N/A | `AgentRegistry.Prepare(id)` called BEFORE invoke |

### `POST /containers/{id}/stop` — Stop Container

| Aspect | Vanilla Docker | GCF Backend |
|--------|---------------|-------------|
| What happens | Sends SIGTERM, then SIGKILL after timeout | **No-op** — returns 204 without doing anything |
| Reason | Process can be signalled | Cloud Functions run to completion; no stop API exists |

### `POST /containers/{id}/kill` — Kill Container

| Aspect | Vanilla Docker | GCF Backend |
|--------|---------------|-------------|
| What happens | Sends specified signal to process | Disconnects reverse agent (unblocks invoke goroutine) |
| Effect | Process receives signal | Agent disconnected, container transitions to exited |

### `DELETE /containers/{id}` — Remove Container

| Aspect | Vanilla Docker | GCF Backend |
|--------|---------------|-------------|
| What happens | Removes container and its filesystem | Calls `Functions.DeleteFunction` (best-effort), removes local state |
| Force | Kills running container first | Disconnects agent if connected |

### `POST /containers/{id}/restart` — Restart Container

| Aspect | Vanilla Docker | GCF Backend |
|--------|---------------|-------------|
| What happens | Stops then starts the container | **No-op** — returns 204 (functions run to completion) |

## Exec

Handled by core via the driver chain → Agent → Synthetic.

| Aspect | Vanilla Docker | GCF Backend |
|--------|---------------|-------------|
| Execution | Runs in container namespace | Routed to reverse agent inside the function |
| Availability | Anytime while running | Only while function is executing and agent is connected |

## Images

### `POST /images/create` — Pull Image

| Aspect | Vanilla Docker | GCF Backend |
|--------|---------------|-------------|
| What happens | Downloads image layers from registry | Creates synthetic image (no download) |
| Image config | Full manifest + layers | Synthetic: hash of reference as ID |

### `POST /images/load` — Load Image

**Not implemented.** Returns `NotImplementedError`.

## Logs

### `GET /containers/{id}/logs` — Container Logs

| Aspect | Vanilla Docker | GCF Backend |
|--------|---------------|-------------|
| Source | Container stdout/stderr | Cloud Logging via `LogAdmin.Entries` |
| Filter | N/A | `resource.type="cloud_run_revision" AND resource.labels.service_name="{functionName}"` |
| Follow mode | Real-time streaming | **Not supported** (single snapshot fetch) |
| Timestamps | From Docker daemon | From entry timestamps (RFC3339Nano) |
| Stdout/stderr | Separate streams | All treated as stdout |
| Format | Docker multiplexed stream | Docker multiplexed stream (8-byte header per line) |
| Timeout | N/A | 2s in simulator mode |

## Networks, Volumes, Archive, System

All handled by core with synthetic/in-memory implementations. See the core documentation for details.

| Feature | Vanilla Docker | GCF Backend |
|---------|---------------|-------------|
| Networks | Real Docker networks | In-memory tracking only |
| Volumes | Real Docker volumes | In-memory tracking only |
| Archive copy | Direct filesystem access | Via reverse agent or synthetic |
| System info | Real daemon info | Static: Driver=cloud-run-functions, OS=Google Cloud Functions |

## CLI Command Mapping

| `docker` CLI command | Vanilla Docker | GCF Backend |
|---------------------|---------------|-------------|
| `docker create <image>` | Creates container locally | `Functions.CreateFunction` (synchronous) |
| `docker start <id>` | Starts local process | HTTP POST to function URL |
| `docker stop <id>` | SIGTERM + SIGKILL | **No-op** |
| `docker kill <id>` | Send signal | Disconnects reverse agent |
| `docker rm <id>` | Remove container + fs | `Functions.DeleteFunction` |
| `docker logs <id>` | Read from daemon | Cloud Logging query |
| `docker logs -f <id>` | Stream from daemon | **Not supported** |
| `docker exec <id> <cmd>` | nsenter into container | Reverse agent relay |
| `docker cp <src> <id>:<dst>` | Write to container layer | Reverse agent relay |
| `docker pull <image>` | Download layers | Synthetic (metadata only) |
| `docker network create` | Create real network | In-memory only |
| `docker volume create` | Create real volume | In-memory only |
| `docker restart <id>` | Stop + start | **No-op** |

## Summary: What's Not Supported

| Feature | Reason |
|---------|--------|
| Container stop | Cloud Functions run to completion |
| Container restart | No meaningful restart for FaaS |
| Log follow mode | Cloud Logging queried as snapshot |
| Image load from tar | GCF uses registry images only |
| Real Docker networks | Functions are isolated |
| Real Docker volumes | No persistent storage |
| Forward agent | Functions cannot accept inbound connections (reverse only) |
| Stderr separation | All output is single-stream in Cloud Logging |
| Pause/unpause | No capability in Cloud Functions |
| Port bindings | Functions use HTTP invoke, not TCP ports |
