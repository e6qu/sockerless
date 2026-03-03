# Known Bugs

## BUG-001: `docker run` with simple commands produces no output against cloud backends in simulator mode

**Severity**: High
**Component**: ECS backend (and likely CloudRun, ACA backends too)
**Status**: Fixed — replaced EndpointURL checks with IsTailDevNull, added agent PATH resolution in simulators
**Discovered**: 2026-03-02 during Phase 80 manual E2E verification

### Symptoms

Running `docker run --rm alpine echo "hello"` against the Docker frontend connected to an ECS backend (with AWS simulator) produces no output and exits 0. The echo message is silently lost.

### Root Cause

Two interacting issues in the ECS backend's `handleContainerStart` (`backends/ecs/containers.go:201-207`):

**Issue A — Helper container shortcut**: When `SOCKERLESS_CALLBACK_URL` is set, the start handler classifies containers as either "job containers" (command = `tail -f /dev/null`) or "helper containers" (everything else). Helper containers are auto-stopped after 500ms without calling RunTask at all. This means `docker run alpine echo "hello"` never reaches the simulator — the container is marked "exited 0" immediately.

**Issue B — No agent injection in simulator mode**: When RunTask IS called (for `tail -f /dev/null` containers), the task definition builder skips agent injection in simulator mode. The backend still sets `SOCKERLESS_CALLBACK_URL` and waits for agent callback, but neither the backend nor the simulator injects the agent binary.

### Impact

This doesn't affect CI runner workflows (which use `tail -f /dev/null` containers and `docker exec`), but it means standalone `docker run` with simple commands doesn't produce output when using cloud backends in simulator mode. The memory backend and docker backend work correctly.

---

## BUG-002: `docker exec` uses synthetic fallback in simulator mode (echoes command instead of executing)

**Severity**: Medium
**Component**: ECS backend (and likely CloudRun, ACA backends too)
**Status**: Fixed — same fix as BUG-001 (agent injection based on command type, not simulator detection)
**Related**: BUG-001 Issue B

### Symptoms

After starting a container with `tail -f /dev/null` via `docker start`, running `docker exec <container> echo "hello"` returns the text `echo hello` instead of executing the command and returning `hello`.

### Root Cause

The exec handler falls through to the synthetic driver because no agent is connected. In simulator mode with callback URL, the backend waits for agent callback which times out, and any subsequent `docker exec` uses the synthetic driver which echoes the command text.

### Impact

Same as BUG-001 — doesn't affect CI runner workflows, but breaks interactive use in simulator mode.

---

## BUG-047: `gofmt` violations in cleanup.go and project.go

**Severity**: Low (formatting only)
**Component**: `cmd/sockerless-admin/cleanup.go`, `cmd/sockerless-admin/project.go`
**Status**: Fixed — Sprint 7: Ran `gofmt -w` on both files

### Details

- `cleanup.go:146-158`: Wrong indentation inside `if c.State == "exited" || c.State == "dead"` block — code compiles correctly (Go uses braces) but violates `gofmt` formatting
- `project.go:62-68`: `ProjectConnection` struct has extra padding on field tags (`DockerHost        string` instead of `DockerHost       string`)

---

## BUG-048: GCF backend uses non-cloud-native `X-Sockerless-Command` header

**Severity**: Medium
**Component**: `backends/cloudrun-functions/containers.go`, `simulators/gcp/cloudfunctions.go`
**Status**: Fixed — command now set at create time via `SOCKERLESS_CMD` environment variable

### Details

GCF backend sent `X-Sockerless-Command` header at invoke time to pass the command to the function runtime. Real Cloud Functions don't support custom headers for command passing. Command is now set at function create time via `SOCKERLESS_CMD` env var (base64-encoded JSON), matching the Lambda pattern of setting command at creation rather than invocation.

---

## BUG-049: AZF backend uses non-cloud-native `X-Sockerless-Command` header

**Severity**: Medium
**Component**: `backends/azure-functions/containers.go`, `simulators/azure/functions.go`
**Status**: Fixed — command now set at create time via `SOCKERLESS_CMD` app setting

### Details

Same issue as BUG-048. AZF backend already used `AppCommandLine` for agent callback mode but not for short-lived commands. Command is now set at create time via `SOCKERLESS_CMD` app setting (base64-encoded JSON), and the function is invoked with a plain HTTP POST.

---

## BUG-050: Lambda simulator sets unused `X-Sockerless-Exit-Code` header

**Severity**: Low
**Component**: `simulators/aws/lambda.go`
**Status**: Fixed — removed vestigial header

### Details

Lambda simulator set `X-Sockerless-Exit-Code` response header, but the Lambda backend never reads it (uses `FunctionError` / `X-Amz-Function-Error` from the SDK). Vestigial from a previous naming round.

---

## BUG-051: `SimCommand` field on simulator types is non-cloud-native

**Severity**: Low
**Component**: `simulators/gcp/cloudfunctions.go`, `simulators/azure/functions.go`
**Status**: Fixed — backends now use `SOCKERLESS_CMD` env var/app setting; `SimCommand` retained as backward-compat fallback for SDK tests

### Details

`SimCommand` is explicitly "simulator-only" on types that mirror real cloud APIs. After this fix, backends use `SOCKERLESS_CMD` environment variable (GCF) or app setting (AZF) instead. Simulators read `SOCKERLESS_CMD` first, falling back to `SimCommand` for backward compatibility with SDK tests that set the field directly.
