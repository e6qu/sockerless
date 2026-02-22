# Docker API Mapping: Lambda Backend

The Lambda backend translates Docker API calls into AWS Lambda operations. A Docker "container" becomes a Lambda Function using a container image, invoked asynchronously.

## Container Lifecycle

### `POST /containers/create` — Create Container

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| What happens | Creates a container from an image on the local daemon | Calls `Lambda.CreateFunction` with container image URI |
| Image | Must exist locally or be pullable | Passed directly as `ImageUri` (must be in ECR or compatible registry) |
| Entrypoint/Cmd | Stored as container config | Set as `ImageConfig.EntryPoint` and `ImageConfig.Command` |
| Environment | Stored as container config | Set as `Environment.Variables` map on the function |
| Working directory | Stored as container config | Set as `ImageConfig.WorkingDirectory` |
| Memory | Configurable limits | `SOCKERLESS_LAMBDA_MEMORY_SIZE` (default: 1024 MB) |
| Timeout | N/A (runs until stopped) | `SOCKERLESS_LAMBDA_TIMEOUT` (default: 900s / 15 min) |
| Network | Joins specified network | Optional VPC config via `SOCKERLESS_LAMBDA_SUBNETS` |
| Resources | Flexible CPU/memory | Memory-proportional CPU (Lambda model) |
| Name | Function name: `skls-{containerID[:12]}` | |
| Return value | Container ID | Container ID (locally generated) |

**Lambda-specific details:**
- Package type: `Image` (container image, not ZIP)
- Agent env vars injected: `SOCKERLESS_CONTAINER_ID`, `SOCKERLESS_AGENT_TOKEN`, `SOCKERLESS_AGENT_CALLBACK_URL`
- Entrypoint wrapped with `core.BuildAgentCallbackEntrypoint()` for reverse agent

### `POST /containers/{id}/start` — Start Container

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| What happens | Starts the container process | Calls `Lambda.Invoke` asynchronously |
| Blocking | Returns immediately (process runs in background) | Returns immediately (invocation runs in background) |
| Agent mode | N/A | Reverse agent only — waits for callback (60s timeout) |
| Helper containers | N/A | Non-tail-dev-null commands auto-stop after 500ms |
| Exit handling | Process exits naturally | Goroutine waits for invoke response, then waits up to 30 min for agent disconnect |
| Pre-create channel | N/A | `AgentRegistry.Prepare(id)` called BEFORE invoke to avoid race condition |

### `POST /containers/{id}/stop` — Stop Container

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| What happens | Sends SIGTERM, then SIGKILL after timeout | **No-op** — returns 204 without doing anything |
| Reason | Process can be signalled | Lambda functions run to completion; there is no API to stop an in-flight invocation |

### `POST /containers/{id}/kill` — Kill Container

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| What happens | Sends specified signal to process | Disconnects reverse agent (unblocks invoke goroutine) |
| Signal support | Full POSIX signals | Signal is ignored; only agent disconnect occurs |
| Effect | Process receives signal | Agent disconnected, invoke goroutine proceeds to stop container |

### `DELETE /containers/{id}` — Remove Container

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| What happens | Removes container and its filesystem | Calls `Lambda.DeleteFunction` (best-effort), removes local state |
| Force | Kills running container first | Disconnects agent, deletes function regardless |
| Cleanup | Container layers deleted | Function deleted from Lambda, local state cleared |

## Exec

### `POST /containers/{id}/exec` + `POST /exec/{id}/start`

Handled by core via the driver chain.

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| Execution | Runs directly in container namespace | Routed to reverse agent running inside the Lambda function |
| Fallback | N/A | Synthetic driver if no agent connected |
| Availability | Anytime while running | Only while function is actively executing and agent is connected |

## Images

### `POST /images/create` — Pull Image

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| What happens | Downloads image layers from registry | Creates synthetic image in local store (no download) |
| Image config | Full manifest + layers | Synthetic: hash of reference as ID, minimal metadata |
| Storage | Layers on disk | Metadata only in memory |

### `POST /images/load` — Load Image

**Not implemented.** Returns `NotImplementedError`.

## Logs

### `GET /containers/{id}/logs` — Container Logs

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| Source | Container stdout/stderr from daemon | CloudWatch Logs (`DescribeLogStreams` + `GetLogEvents`) |
| Log group | N/A | `/aws/lambda/{functionName}` (auto-derived) |
| Log stream | N/A | Latest stream for the function (by LastEventTime) |
| Follow mode | Real-time streaming | **Not supported** (single snapshot fetch) |
| Timestamps | From Docker daemon | From CloudWatch event timestamps (RFC3339Nano) |
| Tail | Last N lines from buffer | Last N CloudWatch events |
| Stdout/stderr | Separate streams | All treated as stdout (stream type 1) |
| Format | Docker multiplexed stream | Docker multiplexed stream (8-byte header per line) |
| Availability | Immediate | Only after function starts writing to CloudWatch |

## Networks

Handled entirely by core. Synthetic in-memory tracking only.

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| Network creation | Creates real Docker network | In-memory tracking only |
| Inter-container networking | Via shared Docker network | Not available (Lambda functions are isolated) |
| VPC | N/A | Optional via `SOCKERLESS_LAMBDA_SUBNETS` |

## Volumes

Handled entirely by core. In-memory tracking only.

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| Volume creation | Creates real Docker volume | In-memory tracking only |
| Bind mounts | Maps host → container paths | **Not supported** (Lambda has no bind mount concept) |
| Persistent storage | Volumes persist | No persistent storage |

## Archive (Copy)

Handled by core via the driver chain → agent or synthetic.

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| Copy to | Extracts tar into container filesystem | Routed to reverse agent (writes to function filesystem) |
| Copy from | Creates tar from container filesystem | Routed to reverse agent (reads from function filesystem) |
| Before start | Writes to container layer | Staged in `StagingDirs`, merged on start |

## System

Handled by core. Returns synthetic data.

| Aspect | Vanilla Docker | Lambda Backend |
|--------|---------------|----------------|
| Info | Real daemon info | Static: Driver=lambda, OS=AWS Lambda, 2 CPUs, 4GB RAM |

## Pause/Unpause

Not explicitly handled (falls through to core defaults which set state but have no real effect).

## CLI Command Mapping

| `docker` CLI command | Vanilla Docker | Lambda Backend |
|---------------------|---------------|----------------|
| `docker create <image>` | Creates container locally | `Lambda.CreateFunction` |
| `docker start <id>` | Starts local process | `Lambda.Invoke` (async) |
| `docker stop <id>` | SIGTERM + SIGKILL | **No-op** (returns 204) |
| `docker kill <id>` | Send signal | Disconnects reverse agent |
| `docker rm <id>` | Remove container + fs | `Lambda.DeleteFunction` |
| `docker logs <id>` | Read from daemon | CloudWatch `GetLogEvents` |
| `docker logs -f <id>` | Stream from daemon | **Not supported** (single fetch only) |
| `docker exec <id> <cmd>` | nsenter into container | Reverse agent relay |
| `docker cp <src> <id>:<dst>` | Write to container layer | Reverse agent relay |
| `docker pull <image>` | Download layers | Synthetic (metadata only) |
| `docker build .` | Build from Dockerfile | Core Dockerfile parser (RUN is no-op) |
| `docker network create` | Create real network | In-memory only |
| `docker volume create` | Create real volume | In-memory only |
| `docker pause <id>` | Freeze cgroups | No real effect |
| `docker stats <id>` | Real cgroup stats | Synthetic stats |

## Summary: What's Not Supported

| Feature | Reason |
|---------|--------|
| Container stop | Lambda functions run to completion; no stop API exists |
| Log follow mode | CloudWatch queried as snapshot, not streamed |
| Image load from tar | Lambda uses registry/ECR images only |
| Real Docker networks | Lambda functions are isolated (optional VPC only) |
| Real Docker volumes | No bind mount or volume support in Lambda |
| Bind mounts | Lambda has no host filesystem concept |
| Forward agent | Lambda cannot accept inbound connections (reverse only) |
| Stderr separation | CloudWatch streams all output as stdout |
| Container restart | No meaningful restart — function would need re-invocation |
| Port bindings | Lambda uses HTTP invoke, not TCP port binding |
| Resource customization at runtime | Memory fixed at function creation time |
