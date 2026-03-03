# Known Bugs

## BUG-001: `docker run` with simple commands produces no output against cloud backends in simulator mode

**Severity**: High
**Component**: ECS backend (and likely CloudRun, ACA backends too)
**Discovered**: 2026-03-02 during Phase 80 manual E2E verification

### Symptoms

Running `docker run --rm alpine echo "hello"` against the Docker frontend connected to an ECS backend (with AWS simulator) produces no output and exits 0. The echo message is silently lost.

### Root Cause

Two interacting issues in the ECS backend's `handleContainerStart` (`backends/ecs/containers.go:201-207`):

**Issue A — Helper container shortcut**: When `SOCKERLESS_CALLBACK_URL` is set, the start handler classifies containers as either "job containers" (command = `tail -f /dev/null`) or "helper containers" (everything else). Helper containers are auto-stopped after 500ms without calling RunTask at all. This means `docker run alpine echo "hello"` never reaches the simulator — the container is marked "exited 0" immediately.

```go
// containers.go:201-207
if s.config.CallbackURL != "" && !core.IsTailDevNull(c.Config.Entrypoint, c.Config.Cmd) {
    go func() {
        time.Sleep(500 * time.Millisecond)
        s.Store.StopContainer(id, 0)
    }()
    w.WriteHeader(http.StatusNoContent)
    return
}
```

This optimization exists for CI runner helper containers (git clone helpers, cache containers) that don't need real execution. But it breaks standalone `docker run` usage.

**Issue B — No agent injection in simulator mode**: When RunTask IS called (for `tail -f /dev/null` containers), the task definition builder skips agent injection in simulator mode (`backends/ecs/taskdef.go:43-46`):

```go
if s.config.EndpointURL != "" {
    // Simulator mode: pass original command through, no agent wrapping
    entrypoint = config.Entrypoint
    command = config.Cmd
}
```

The backend still sets `SOCKERLESS_CALLBACK_URL` and waits for agent callback, but neither the backend nor the simulator injects the agent binary. The simulator runs the raw command via `sim.StartProcess`, output goes to CloudWatch logs, but the agent never connects back. After timeout, exec falls back to synthetic mode (echoes command text instead of executing).

### Reproduction

```bash
# Build binaries
cd simulators/aws && GOWORK=off go build -tags noui -o /tmp/sim-aws .
cd ../.. && go build -tags noui -o /tmp/be-ecs ./backends/ecs/cmd/sockerless-backend-ecs
go build -tags noui -o /tmp/fe-docker ./frontends/docker/cmd

# Start simulator
SIM_AWS_PORT=4566 /tmp/sim-aws &

# Create cluster
curl -X POST http://localhost:4566/ \
  -H "Content-Type: application/x-amz-json-1.1" \
  -H "X-Amz-Target: AmazonEC2ContainerServiceV20141113.CreateCluster" \
  -d '{"clusterName":"test"}'

# Start backend (with callback URL)
SOCKERLESS_ENDPOINT_URL=http://127.0.0.1:4566 \
SOCKERLESS_ECS_CLUSTER=test \
SOCKERLESS_ECS_SUBNETS=subnet-1 \
SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::0:role/test \
SOCKERLESS_CALLBACK_URL=http://127.0.0.1:9100 \
SOCKERLESS_SKIP_IMAGE_CONFIG=true \
AWS_REGION=us-east-1 \
/tmp/be-ecs --addr :9100 &

# Start frontend
/tmp/fe-docker --addr :2375 --backend http://127.0.0.1:9100 &

# Test — produces no output (should print "hello")
export DOCKER_HOST=tcp://localhost:2375
docker run --rm alpine echo "hello"
```

### Expected Behavior

`docker run --rm alpine echo "hello"` should print `hello` and exit 0.

### Actual Behavior

- With `SOCKERLESS_CALLBACK_URL` set: Command exits 0 with no output (helper shortcut auto-stops without running)
- Without `SOCKERLESS_CALLBACK_URL`: RunTask is called but backend tries to dial forward agent at simulated IP `10.0.x.x:9111` which doesn't exist — times out

### Impact

This doesn't affect CI runner workflows (which use `tail -f /dev/null` containers and `docker exec`), but it means standalone `docker run` with simple commands doesn't produce output when using cloud backends in simulator mode. The memory backend and docker backend work correctly.

### Possible Fixes

1. **Simulator-side agent injection**: When `SOCKERLESS_AGENT_CALLBACK_URL` is in the task environment, the simulator should prepend the agent binary to the command before spawning the process
2. **Backend-side**: Remove the helper container shortcut and always call RunTask, letting the simulator handle execution
3. **Hybrid**: Detect simulator mode in the start handler and use a different code path that streams process output directly without requiring an agent

---

## BUG-002: `docker exec` uses synthetic fallback in simulator mode (echoes command instead of executing)

**Severity**: Medium
**Component**: ECS backend (and likely CloudRun, ACA backends too)
**Related**: BUG-001 Issue B

### Symptoms

After starting a container with `tail -f /dev/null` via `docker start`, running `docker exec <container> echo "hello"` returns the text `echo hello` instead of executing the command and returning `hello`.

### Root Cause

The exec handler falls through to the synthetic driver because no agent is connected. In simulator mode with callback URL:

1. Backend calls RunTask on simulator
2. Simulator runs the raw command (no agent wrapping)
3. Backend waits for agent callback — times out after ~30s
4. Backend logs warning: `"agent callback timeout, exec will use synthetic fallback"`
5. Any subsequent `docker exec` uses the synthetic driver which echoes the command text

The synthetic driver is the last-resort fallback in the exec driver chain: Agent → WASM → Synthetic.

### Expected Behavior

`docker exec` should execute the command inside the simulated task and return real output.

### Actual Behavior

`docker exec ci-runner echo "hello"` returns the string `echo hello` (synthetic echo).

### Impact

Same as BUG-001 — doesn't affect CI runner workflows (which work because the integration tests use Docker-in-Docker or specific test harnesses), but breaks interactive use in simulator mode.

---

## BUG-003: Root README quick-start had wrong build path for memory backend

**Severity**: Low (documentation)
**Component**: README.md
**Status**: Fixed in Phase 80

### Description

The README quick-start section contained an incorrect build command:
```bash
go build -o sockerless-backend-memory ./backends/memory/cmd/sockerless-backend-memory
```

The correct path is:
```bash
go build -o sockerless-backend-memory ./backends/memory/cmd
```

The `main.go` file is directly in `backends/memory/cmd/`, not in a subdirectory.

**Fixed in**: Phase 80 commit `0145465`

---

## BUG-004: ECS backend request-level logging missing despite `--log-level debug`

**Severity**: Low
**Component**: ECS backend

### Symptoms

Running the ECS backend with `--log-level debug` only shows 3-4 startup log lines. Individual HTTP request handling, RunTask calls, agent connection attempts, and container lifecycle events are not logged to the backend's output, even though the simulator shows the corresponding API calls arriving.

### Root Cause

The ECS backend's `main.go` creates a logger and passes it to `backend.NewServer()`, but the core's HTTP middleware and request handlers may use a different logger instance or log at a level that doesn't reach the console.

### Expected Behavior

With `--log-level debug`, every incoming HTTP request, cloud API call, agent connection attempt, and container state change should appear in the backend's log output (similar to how the frontend logs every Docker API request).

### Impact

Makes debugging end-to-end issues significantly harder — you have to correlate simulator logs with frontend logs to understand what the backend is doing, because the backend itself is silent.
