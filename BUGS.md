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

## BUG-034: `ProcessManager.Stop` clobbers PID/cancel of re-started process (race condition)

**Severity**: Medium
**Component**: `cmd/sockerless-admin/process.go`
**Status**: Fixed — Sprint 5: Added generation check using `doneCh` identity; only cleans up if process hasn't been re-started

---

## BUG-035: `handleOverview` counts "unknown" health as "down"

**Severity**: Low
**Component**: `cmd/sockerless-admin/api_overview.go`
**Status**: Fixed — Sprint 5: Changed to only count explicitly "down" as down

---

## BUG-036: ProjectDetailPage Start button enabled during "stopping" status

**Severity**: Medium
**Component**: `ui/packages/admin/src/pages/ProjectDetailPage.tsx`
**Status**: Fixed — Sprint 5: Added `|| project.status === "stopping"` to disabled prop

---

## BUG-037: ProjectsPage Start button enabled during "stopping" status

**Severity**: Medium
**Component**: `ui/packages/admin/src/pages/ProjectsPage.tsx`
**Status**: Fixed — Sprint 5: Added `|| proj.status === "stopping"` to disabled prop

---

## BUG-038: ProjectDetailPage Delete button enabled during starting/stopping

**Severity**: Medium
**Component**: `ui/packages/admin/src/pages/ProjectDetailPage.tsx`
**Status**: Fixed — Sprint 5: Added `|| project.status === "starting" || project.status === "stopping"` to disabled prop

---

## BUG-039: ComponentDetailPage uptime only shows minutes, inconsistent with ComponentsPage

**Severity**: Low
**Component**: `ui/packages/admin/src/pages/ComponentDetailPage.tsx`
**Status**: Fixed — Sprint 5: Uses same `Xh Ym` format as ComponentsPage for uptimes >= 3600s

---

## BUG-040: ProjectCreatePage `handleSubmit` allows double-submission via rapid Enter

**Severity**: Low
**Component**: `ui/packages/admin/src/pages/ProjectCreatePage.tsx`
**Status**: Fixed — Sprint 5: Added `if (create.isPending) return;` guard at top of handleSubmit

---

## BUG-041: Error display uses `||`, hiding concurrent start+stop errors

**Severity**: Low
**Component**: `ui/packages/admin/src/pages/ProcessesPage.tsx`, `ProjectsPage.tsx`, `ProjectDetailPage.tsx`
**Status**: Fixed — Sprint 5: Changed to array filter+map pattern showing all errors

---

## BUG-042: App.tsx has no catch-all 404 route

**Severity**: Low
**Component**: `ui/packages/admin/src/App.tsx`
**Status**: Fixed — Sprint 5: Added `<Route path="*">` fallback with "Page not found" message
