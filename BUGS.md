# Known Bugs

## BUG-001: `docker run` with simple commands produces no output against cloud backends in simulator mode

**Severity**: High
**Component**: ECS backend (and likely CloudRun, ACA backends too)
**Status**: Deferred — requires new simulator-direct execution mode (architectural change)
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
**Status**: Deferred — same root cause as BUG-001 Issue B
**Related**: BUG-001 Issue B

### Symptoms

After starting a container with `tail -f /dev/null` via `docker start`, running `docker exec <container> echo "hello"` returns the text `echo hello` instead of executing the command and returning `hello`.

### Root Cause

The exec handler falls through to the synthetic driver because no agent is connected. In simulator mode with callback URL, the backend waits for agent callback which times out, and any subsequent `docker exec` uses the synthetic driver which echoes the command text.

### Impact

Same as BUG-001 — doesn't affect CI runner workflows, but breaks interactive use in simulator mode.

---

## BUG-043: `buildStatus` doesn't detect "stopping" state

**Severity**: Low
**Component**: `cmd/sockerless-admin/project_manager.go`
**Status**: Fixed — Sprint 6: Added "stopping" check after "starting" check in `buildStatus`

---

## BUG-044: ProcessDetailPage error display uses `||`, hiding concurrent errors

**Severity**: Low
**Component**: `ui/packages/admin/src/pages/ProcessDetailPage.tsx`
**Status**: Fixed — Sprint 6: Replaced `||` with array filter+map pattern (same fix as BUG-041, missed page)

---

## BUG-045: Health badge shows "error" for "unknown" health

**Severity**: Low
**Component**: `ui/packages/admin/src/pages/ComponentsPage.tsx`, `ComponentDetailPage.tsx`, `DashboardPage.tsx`
**Status**: Fixed — Sprint 6: Map "unknown" health to "warning" StatusBadge instead of "error"

---

## BUG-046: ComponentDetailPage reload doesn't invalidate provider cache

**Severity**: Low
**Component**: `ui/packages/admin/src/pages/ComponentDetailPage.tsx`
**Status**: Fixed — Sprint 6: Added `["component-provider", name]` invalidation to reload onSuccess
