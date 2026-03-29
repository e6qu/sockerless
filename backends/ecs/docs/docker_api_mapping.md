# Docker API Mapping: ECS Backend

The ECS backend translates Docker API calls into AWS ECS Fargate operations. A Docker "container" becomes an ECS Task Definition + Fargate Task.

## Container Lifecycle

### `POST /containers/create` — Create Container

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| What happens | Creates a container from an image on the local daemon | Calls `RegisterTaskDefinition` to create an ECS Task Definition |
| Image | Must exist locally or be pullable | Used as the task definition container image URI |
| Entrypoint/Cmd | Stored as container config | Wrapped with agent entrypoint (forward or reverse) |
| Environment | Stored as container config | Converted to ECS `KeyValuePair` array in task definition |
| Working directory | Stored as container config | Set as `WorkingDirectory` in task definition |
| Bind mounts | Host paths or named volumes | Converted to ECS Volumes + MountPoints |
| User | Stored as container config | Set as `User` in task definition |
| Network | Joins specified network | Always `awsvpc` (Fargate requirement) |
| Port bindings | Maps host → container ports | Converted to ECS PortMappings; port 9111 always added for agent |
| Resources | Configurable CPU/memory limits | Fixed: 256 mCPU, 512 MB (hardcoded) |
| Return value | Container ID | Container ID (locally generated) |

**ECS-specific details:**
- Task definition family: `sockerless-{containerID[:12]}`
- Agent env vars injected: `SOCKERLESS_CONTAINER_ID`, `SOCKERLESS_AGENT_TOKEN`
- If reverse agent: `SOCKERLESS_AGENT_CALLBACK_URL` added
- Logging: `awslogs` driver configured with CloudWatch Logs log group and stream prefix
- IAM roles: execution role (required) and optional task role attached

### `POST /containers/{id}/start` — Start Container

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| What happens | Starts the container process | Calls `RunTask` to launch a Fargate task |
| Networking | Container joins its network | Task uses configured subnets + security groups |
| Public IP | Based on network/port config | Controlled by `SOCKERLESS_ECS_PUBLIC_IP` |
| Launch type | N/A (always local) | Always Fargate |
| Agent (forward) | N/A | Polls task via `DescribeTasks` until RUNNING, extracts ENI IP, health-checks agent on port 9111 |
| Agent (reverse) | N/A | Waits for agent callback to `SOCKERLESS_CALLBACK_URL` (60s timeout) |
| Background | Process runs in daemon | Goroutine polls `DescribeTasks` for task completion, auto-stops container on exit |
| Helper containers | N/A | Non-tail-dev-null commands auto-stop after 500ms |

### `POST /containers/{id}/stop` — Stop Container

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| What happens | Sends SIGTERM, then SIGKILL after timeout | Calls `StopTask` on the Fargate task |
| Grace period | Configurable via `t` param | Not applicable (ECS stops the task) |
| State after | `exited` with exit code | `exited` with exit code 0 |

### `POST /containers/{id}/kill` — Kill Container

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| What happens | Sends specified signal to process | Disconnects reverse agent, calls `StopTask` |
| Signal support | Full POSIX signals | SIGKILL/9/KILL → exit code 137; others → exit code 0 |
| Immediate | Yes (signal delivered directly) | Best-effort (task may take time to stop) |

### `DELETE /containers/{id}` — Remove Container

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| What happens | Removes container and its filesystem | Calls `StopTask` + `DeregisterTaskDefinition` (best-effort), removes local state |
| Force | Kills running container first | Stops task first if running |
| Volumes | Optional removal via `v` param | Local state cleanup only (no EFS cleanup) |

### `POST /containers/{id}/restart` — Restart Container

Inherited from core. Stops the task, cleans up, and starts a new task with the same configuration.

## Exec

### `POST /containers/{id}/exec` — Create Exec

Handled by core. Creates an exec instance in local state. The command will be executed when `exec start` is called.

### `POST /exec/{id}/start` — Start Exec

Handled by core via the driver chain. Dispatches through:
1. **Agent driver** — if agent connected (forward or reverse), forwards exec to agent inside the Fargate task
2. **Synthetic driver** — fallback if no agent (returns empty output)

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| Execution | Runs directly in container namespace | Routed to agent running inside the task |
| Stdin | Direct pipe to process | Forwarded over agent WebSocket |
| Stdout/stderr | Direct pipe from process | Streamed back over agent WebSocket |
| TTY | Allocates PTY | Handled by agent |
| Working directory | Resolved in container | Resolved by agent inside task |

## Images

### `POST /images/create` — Pull Image

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| What happens | Downloads image layers from registry | Creates synthetic image in local store (no actual download). Real image config (Cmd, Entrypoint, Env, WorkingDir) is fetched from the registry via Docker v2 API |
| Registry auth | Uses stored credentials | Anonymous auth for public images; ECR auth for private ECR images |
| Image config | Full manifest + layers | Synthetic ID (hash of reference), real config fetched from registry (Cmd, Entrypoint, Env, WorkingDir, Labels, ExposedPorts) |
| Progress | Real download progress | Simulated progress events |
| Storage | Layers stored on disk | Metadata only in memory |

### `POST /images/load` — Load Image

**Not implemented.** Returns `NotImplementedError`. ECS uses registry-based images only.

## Logs

### `GET /containers/{id}/logs` — Container Logs

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| Source | Container stdout/stderr from daemon | CloudWatch Logs `GetLogEvents` |
| Log group | N/A | `SOCKERLESS_ECS_LOG_GROUP` (default: `/sockerless`) |
| Log stream | N/A | `{containerID[:12]}/main/{taskID}` |
| Follow mode | Real-time streaming | Polls CloudWatch every 1 second |
| Timestamps | From Docker daemon | From CloudWatch event timestamps (RFC3339Nano) |
| Tail | Last N lines from buffer | Last N CloudWatch events |
| Stdout/stderr | Separate streams | All treated as stdout (stream type 1) |
| Format | Docker multiplexed stream | Docker multiplexed stream (8-byte header per line) |

## Networks

### `POST /networks/create`, `GET /networks`, etc.

Networks are tracked in memory with synthetic IP allocation. Docker network create also creates a VPC security group, and connecting a container to a network associates that security group with the container's ECS task.

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| Network creation | Creates real Docker network | In-memory tracking + VPC security group (`skls-{name}`) with self-referencing ingress |
| IP allocation | IPAM assigns real IPs | Synthetic IPs (172.17.0.x); real ENI IP available after task starts |
| Network connect | Joins container to network | Associates network's security group with the container's ECS task |
| DNS resolution | Docker DNS between containers | Cloud Map private DNS namespace (when configured) |
| Inter-container networking | Via shared Docker network | Via shared VPC security group (containers in same network can communicate) |

## Volumes

### `POST /volumes/create`, `DELETE /volumes/{name}`, etc.

Handled by core with ECS overrides for remove/prune.

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| Volume creation | Creates real Docker volume | In-memory tracking only |
| Bind mounts | Maps host → container paths | Converted to ECS Volume + MountPoint in task definition. When `SOCKERLESS_AGENT_EFS_ID` is set, bind mounts use EFS with per-container subdirectories |
| Persistent storage | Volumes persist across containers | EFS-backed when `SOCKERLESS_AGENT_EFS_ID` is configured; otherwise ephemeral scratch volumes |
| Volume prune | Removes unused real volumes | Removes from in-memory store |

## Archive (Copy)

### `PUT /containers/{id}/archive` — Copy to Container
### `HEAD /containers/{id}/archive` — Stat Path
### `GET /containers/{id}/archive` — Copy from Container

Handled by core via the driver chain. Dispatches through Agent → Synthetic.

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| Copy to | Extracts tar into container filesystem | Routed to agent (writes to task filesystem) or synthetic fallback |
| Copy from | Creates tar from container filesystem | Routed to agent (reads from task filesystem) or synthetic fallback |
| Before start | Writes to container layer | Staged in `StagingDirs`, merged on start |

## System

### `GET /system/df`, `GET /events`, `GET /info`

Handled by core. Returns synthetic data.

| Aspect | Vanilla Docker | ECS Backend |
|--------|---------------|-------------|
| Disk usage | Real disk usage from daemon | Calculated from in-memory state |
| Events | Real Docker events stream | Empty stream (kept open) |
| Info | Real daemon info | Static: Driver=ecs-fargate, OS=AWS Fargate, 2 CPUs, 4GB RAM |

## Pause/Unpause

**Not supported.** Returns `NotImplementedError`. ECS Fargate does not support pausing tasks.

## CLI Command Mapping

| `docker` CLI command | Vanilla Docker | ECS Backend |
|---------------------|---------------|-------------|
| `docker create <image>` | Creates container locally | `RegisterTaskDefinition` |
| `docker start <id>` | Starts local process | `RunTask` (Fargate) |
| `docker stop <id>` | SIGTERM + SIGKILL | `StopTask` |
| `docker kill <id>` | Send signal | `StopTask` (best-effort) |
| `docker rm <id>` | Remove container + fs | `StopTask` + `DeregisterTaskDefinition` |
| `docker logs <id>` | Read from daemon | CloudWatch Logs `GetLogEvents` |
| `docker logs -f <id>` | Stream from daemon | Poll CloudWatch every 1s |
| `docker exec <id> <cmd>` | nsenter into container | Agent relay (forward or reverse) |
| `docker cp <src> <id>:<dst>` | Write to container layer | Agent relay or synthetic |
| `docker pull <image>` | Download layers | Synthetic (metadata only) |
| `docker build .` | Build from Dockerfile | Core Dockerfile parser (RUN is no-op) |
| `docker network create` | Create real network | In-memory only |
| `docker volume create` | Create real volume | In-memory only |
| `docker pause <id>` | Freeze cgroups | **Not supported** |
| `docker stats <id>` | Real cgroup stats | Synthetic stats via agent or fallback |
| `docker top <id>` | ps inside container | Agent relay or synthetic |

## Summary: What's Not Supported

| Feature | Reason |
|---------|--------|
| Container pause/unpause | ECS Fargate has no pause capability |
| Image load from tar | ECS uses registry images only |
| Real Docker networks | Docker networks create VPC security groups for isolation, but no Docker-style L2 networking |
| Stderr separation in logs | CloudWatch streams all output as stdout |
| Resource customization | CPU/memory hardcoded (256 mCPU / 512 MB) |
| Multiple processes per container | One task = one container definition |
| Host networking | Fargate only supports `awsvpc` |
