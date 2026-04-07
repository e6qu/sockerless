# Simulator Execution Model

How simulators execute workloads. Current state: bare OS processes. Target state: real containers via Docker/Podman.

## Problem

Simulators currently use `os/exec` to run commands as local processes. When a Cloud Run Job or ECS Task "starts," the simulator extracts the command from the container spec and runs it with `exec.CommandContext`. The container image is never pulled or executed.

This breaks:
- **docker exec**: No real container to exec into — there's just a PID on the host.
- **docker cp / archive**: No container filesystem — files live in the host's working directory.
- **Image behavior**: The command runs on the host, not inside the image. If the image has `/usr/local/bin/myapp`, the host must also have it.
- **Networking**: The process uses host networking, not container networking.
- **Isolation**: No cgroups, no namespaces, no resource limits.
- **Smoke tests**: `act` needs exec and archive to function. Without real containers, the smoke test can't copy files or run commands inside the "container."

## Solution

Replace `sim.StartProcess` with `sim.StartContainer` that runs the actual container image via the Docker/Podman API.

### Shared Layer: `simulators/*/shared/container.go`

```go
type ContainerConfig struct {
    Image   string            // container image (e.g., "alpine:latest")
    Command []string          // entrypoint override
    Args    []string          // command/args override
    Env     map[string]string // environment variables
    Timeout time.Duration     // max execution time (0 = no limit)
    Labels  map[string]string // container labels for tracking
    Network string            // Docker network to join (optional)
}

type ContainerHandle struct {
    ContainerID string        // Docker container ID
    // same interface as ProcessHandle: Wait(), Kill(), Pid()
}

type ContainerResult struct {
    ExitCode  int
    StartedAt time.Time
    StoppedAt time.Time
    Error     error
}

// StartContainer pulls the image (if needed), creates and starts a container.
// Returns a ContainerHandle immediately. Call handle.Wait() to block until exit.
// Stdout/stderr are captured via the LogSink, same as StartProcess.
func StartContainer(cfg ContainerConfig, sink LogSink) *ContainerHandle
```

### Docker Client

Use the Docker Engine API (via `github.com/docker/docker/client`) or shell out to `docker`/`podman`. The Docker SDK is preferred because:
- Programmatic access to container ID, exit code, logs
- No path/shell dependency issues
- Works with both Docker and Podman (Podman serves the same API)

The client connects to the default Docker socket (`/var/run/docker.sock` or `DOCKER_HOST`). This is the host's Docker daemon — the same one the user has running.

### Per-Simulator Changes

#### ECS (`simulators/aws/ecs.go`)
- `RunTask` → `StartContainer` with image from task definition
- Container ID stored alongside task metadata
- `StopTask` → `docker stop` the container
- `ExecuteCommand` → `docker exec` on the real container
- Logs: attach to container stdout/stderr, write to CloudWatch

#### Lambda (`simulators/aws/lambda.go`)
- `Invoke` → `StartContainer` with function image + handler command
- Short-lived: start, wait for exit, capture stdout as response
- Container removed after invocation (or pooled for warm starts)
- Timeout enforced via Docker's `--stop-timeout`

#### Cloud Run Jobs (`simulators/gcp/cloudrunjobs.go`)
- `RunJob` → `StartContainer` for each task in the execution
- Multi-container support: start all containers in the job template
- Execution state driven by container exit codes
- Logs: attach to container stdout/stderr, write to Cloud Logging

#### Cloud Functions (`simulators/gcp/cloudfunctions.go`)
- Same as Lambda: start, invoke, capture output, remove

#### Azure Container Apps (`simulators/azure/containerapps.go`)
- `StartExecution` → `StartContainer` with job template image
- `ReplicaTimeout` enforced via Docker's timeout
- Multi-container: start all containers in the template

#### Azure Functions (`simulators/azure/functions.go`)
- Same as Lambda/GCF: start, invoke, capture output, remove

### No Fallback

Docker or Podman must be available. If the container runtime is not reachable at simulator startup, the simulator exits with a fatal error. There is no process-mode fallback.

```go
func init() {
    if !dockerAvailable() {
        log.Fatal().Msg("Docker/Podman not available — simulators require a container runtime")
    }
}
```

### Configuration

| Env Var | Default | Description |
|---------|---------|-------------|
| `SIM_RUNTIME` | `docker` | Container runtime: `docker`, `podman`, or `process` (bare exec) |
| `SIM_DOCKER_HOST` | (system default) | Docker daemon socket override |
| `SIM_PULL_POLICY` | `if-not-present` | Image pull policy: `always`, `if-not-present`, `never` |

### Container Lifecycle

```
1. Image pull (if needed, respecting pull policy)
2. Container create (with env, command, labels, network)
3. Container start
4. Attach stdout/stderr → LogSink
5. Wait for exit (or timeout)
6. Capture exit code
7. Container remove (cleanup)
```

For long-running containers (ECS tasks, ACA jobs), step 7 happens on stop/kill, not on exit. For invocation-style workloads (Lambda, GCF, AZF), the container is removed after each invocation.

### Container Naming

Containers created by simulators use a naming convention for easy identification and cleanup:

```
sockerless-sim-{cloud}-{resource-type}-{id[:12]}
```

Examples:
- `sockerless-sim-aws-task-0e96a74d6534`
- `sockerless-sim-gcp-job-ab312bff4213`
- `sockerless-sim-azure-execution-31f08e627e39`

### Cleanup

On simulator shutdown (SIGTERM/SIGINT), all simulator-managed containers are stopped and removed. Use Docker labels for discovery:

```
docker ps -a --filter "label=sockerless-sim=true" -q | xargs docker rm -f
```

### Impact on Smoke Tests

With real containers:
- `act` can exec into the container and run commands
- `act` can copy files into the container via archive API
- The smoke test works without auto-agent
- The `SOCKERLESS_AUTO_AGENT_BIN` env var can be removed from `run.sh`

### Impact on Backend CloudState

No impact. The backend queries the simulator's cloud API (ListJobs, DescribeTasks, etc.), not Docker directly. The simulator translates between its cloud API and Docker internally.

### Implementation Priority

1. **Shared container execution layer** (`shared/container.go`) — Docker client, StartContainer, ContainerHandle
2. **ECS simulator** — highest usage in tests
3. **Cloud Run Jobs simulator** — smoke test uses this
4. **ACA simulator** — parity with Cloud Run
5. **Lambda, GCF, AZF** — invocation model (shorter containers)

## Not In Scope

- Kubernetes-style pod networking between containers (use Docker networks instead)
- GPU passthrough
- Volume mounts beyond what Docker supports natively
- Custom OCI runtimes
