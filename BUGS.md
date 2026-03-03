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

## BUG-003: LogViewer XSS via unsanitized HTML in `dangerouslySetInnerHTML`

**Severity**: Medium
**Component**: `ui/packages/core/src/components/LogViewer.tsx`
**Status**: Open

### Description

The `ansiToHtml` function replaces ANSI escape codes with `<span>` tags but does not HTML-escape the non-ANSI portions of each line. The result is passed to `dangerouslySetInnerHTML` (line 51). If log content contains raw HTML (e.g., `<script>alert(1)</script>` or `<img onerror=...>`), it will be injected into the DOM.

### Additional Issues

1. Multi-code ANSI sequences like `\x1b[1;32m` (bold+green) only process the first code — the `for` loop returns on the first match, so `1` produces a bold span but the green `32` is never applied.
2. An opening `\x1b[32m` without a closing `\x1b[0m` produces an unclosed `<span>`, potentially bleeding color into subsequent lines.

### Fix

HTML-escape the text before ANSI replacement (escape `<`, `>`, `&`, `"`). Fix multi-code sequences by building a combined style string instead of returning on first match.

---

## BUG-004: ComponentsPage uses fragile DOM scraping for row navigation

**Severity**: Medium
**Component**: `ui/packages/admin/src/pages/ComponentsPage.tsx:47-51`
**Status**: Open

### Description

Row click navigation works by walking the DOM: `closest("tr")` → `querySelector("td")` → `textContent`. This has several issues:

1. `nameCell.textContent` is used without a null check — if the first `<td>` has no text content, navigation goes to `/ui/components/null`.
2. The approach is fragile — if column order changes or cells gain child elements, the wrong text could be extracted.
3. The entire wrapping `<div>` receives `cursor-pointer`, making even the filter input look clickable.

### Fix

Use TanStack Table's row model to get the row data directly (e.g., `row.original.name`), or use `data-*` attributes on each `<tr>`.

---

## BUG-005: ProjectDetailPage and ProjectLogsPage missing error state handling

**Severity**: Medium
**Component**: `ui/packages/admin/src/pages/ProjectDetailPage.tsx:32`, `ProjectLogsPage.tsx:20`
**Status**: Open

### Description

Both pages destructure only `{ data, isLoading }` from `useQuery`, ignoring `isError` and `error`.

- **ProjectDetailPage**: If the API returns an error, `isLoading` becomes false but `project` stays undefined, so the check `if (isLoading || !project) return <Spinner />` shows an infinite spinner with no error feedback.
- **ProjectLogsPage**: On API failure, `logs` is undefined, so `logs ?? []` renders an empty `<LogViewer>` showing "No log output" — misleading when the real problem is an API error.

### Fix

Destructure `isError` and `error`, render error states as done on other pages.

---

## BUG-006: CleanupPage destructive actions lack confirmation dialogs

**Severity**: Medium
**Component**: `ui/packages/admin/src/pages/CleanupPage.tsx:91,100,109`
**Status**: Open

### Description

The "Clean" (processes, tmp files) and "Prune" (containers) buttons immediately execute mutations on click with no `window.confirm()` or modal. These are destructive operations that kill processes, delete temp files, and prune containers. Compare with `ProjectDetailPage.tsx:63` which correctly uses `window.confirm()` before delete.

### Fix

Add `window.confirm()` guard before each destructive mutation call.

---

## BUG-007: `ProjectManager.Delete` has TOCTOU race allowing concurrent Start+Delete

**Severity**: Medium
**Component**: `cmd/sockerless-admin/project_manager.go:255-287`
**Status**: Open

### Description

`Delete` checks project existence under `m.mu` (line 256-262), releases the lock, calls `m.Stop(name)` (line 265), then re-acquires `m.mu` to `delete(m.projects, name)` (line 274-276). Between the existence check and the delete:

1. Two concurrent `Delete` calls can both pass the existence check — the second one will attempt to stop already-stopped processes and delete an already-deleted key.
2. A concurrent `Start` call can begin orchestration (starting simulator, backend, frontend) while `Delete` is tearing everything down.

### Fix

Use a per-project state field (e.g., `"deleting"`) set under lock before releasing, or hold the lock across the full operation (accepting that Stop is slow).

---

## BUG-008: `ProjectManager.Start` releases lock during long orchestration

**Severity**: Medium
**Component**: `cmd/sockerless-admin/project_manager.go:161-222`
**Status**: Open
**Related**: BUG-007

### Description

`Start` reads config under `m.mu` (lines 162-174) then releases the lock for the entire orchestration sequence (start sim → wait health → bootstrap → start backend → wait health → start frontend → wait health). This can take 35+ seconds. During this time, `Delete` or another `Start` can proceed on the same project.

### Fix

Same approach as BUG-007 — add per-project state tracking (e.g., `"starting"`) that prevents concurrent operations on the same project.

---

## BUG-009: `stopIfRunning` TOCTOU race causes spurious errors

**Severity**: Medium
**Component**: `cmd/sockerless-admin/project_manager.go:487-496`
**Status**: Open

### Description

`stopIfRunning` calls `m.pm.Get(name)` which reads status under `pm.mu`, then calls `m.pm.Stop(name)` which acquires `pm.mu` again. Between the two calls, the process may exit naturally (status transitions from "running" to "failed"). `Stop` then returns an error like `process "foo" is not running (status: failed)`, which propagates to the caller's error list.

### Fix

Have `Stop` (or a new `StopIfRunning` on `ProcessManager`) tolerate the process already being stopped, or catch and ignore "not running" errors.

---

## BUG-010: Port leak when explicit port reservation fails after auto-allocation

**Severity**: Medium
**Component**: `cmd/sockerless-admin/project_manager.go:94-105`
**Status**: Open

### Description

In `Create`, if some ports are auto-allocated (lines 71-92) and then `Reserve` fails for an explicit port (line 102), the function returns the error (line 103) without releasing the auto-allocated ports. Those ports remain in `pa.taken` forever, preventing future projects from using them.

### Fix

Add `m.ports.Release(cfg.Name)` before `return err` on the Reserve error path.

---

## BUG-011: Graceful shutdown uses `os.Exit(0)`, drops in-flight HTTP requests

**Severity**: Medium
**Component**: `cmd/sockerless-admin/main.go:107-115`
**Status**: Open

### Description

The signal handler calls `projectMgr.StopAll()`, `procMgr.StopAll()`, then `os.Exit(0)`. This abruptly terminates the process — in-flight HTTP requests get no response, deferred functions don't run, and there is no opportunity for graceful HTTP connection draining.

### Fix

Use `http.Server` with `srv.Shutdown(ctx)` for graceful HTTP drain before exiting.

---

## BUG-012: `sockerlessDir()` silently falls back to relative path when `HOME` is unset

**Severity**: Low
**Component**: `cmd/sockerless-admin/config.go:45-46`
**Status**: Open

### Description

`os.UserHomeDir()` error is ignored (`home, _ := ...`). If `HOME` is unset (common in minimal containers), `home` is `""` and the function returns `.sockerless` — a relative path in the current working directory. This could cause the admin to read/write project configs, cleanup data, and context files in the wrong location.

### Fix

Return an error or log a warning when `os.UserHomeDir()` fails.

---

## BUG-013: ProjectDetailPage connection query not invalidated on start/stop

**Severity**: Low
**Component**: `ui/packages/admin/src/pages/ProjectDetailPage.tsx:39-53`
**Status**: Open

### Description

The `start` and `stop` mutations (lines 45-53) only invalidate `["project", name]` on success. They do not invalidate `["project-connection", name]`. After starting a stopped project, the connection info (ports, docker_host, etc.) shows stale cached values until the global 5s refetch interval triggers. If the project is stopped, the connection endpoint may return an error that is silently ignored (no `isError` on the connection query either).

### Fix

Invalidate `["project-connection", name]` in start/stop `onSuccess`. Add error handling to the connection query.

---

## BUG-014: ProjectCreatePage has no client-side project name validation

**Severity**: Low
**Component**: `ui/packages/admin/src/pages/ProjectCreatePage.tsx:61-72`
**Status**: Open

### Description

The only client-side check is that `name` is non-empty (line 62). The Go backend validates names against `^[a-z0-9][a-z0-9_-]*$`, but the UI provides no proactive feedback. Users can type spaces, uppercase, or special characters and only see the error after a failed API call. There is also no `maxLength` on the input, no `pattern` attribute, and no form semantics (Enter key doesn't submit).

### Fix

Add inline validation matching the backend regex, show feedback below the input, wrap in `<form>` with `onSubmit`.

---

## BUG-015: StatusBadge receives unmapped status strings, all rendering as gray

**Severity**: Low
**Component**: Multiple admin pages → `ui/packages/core/src/components/StatusBadge.tsx`
**Status**: Open

### Description

The `StatusBadge` color map only covers: `running`, `created`, `exited`, `ok`, `error`. Admin pages pass status strings that are not in the map:

- `"stopped"` (ProcessDetailPage, ProjectDetailPage) — renders gray
- `"starting"` / `"stopping"` (ProcessesPage, ProjectsPage) — renders gray
- `"warning"` (ProjectDetailPage for "partial" status, ProjectsPage) — renders gray

Multiple distinct states all look identical (gray badge), making it hard to distinguish stopped from starting from warning.

### Fix

Add `stopped`, `starting`, `stopping`, `warning` to the StatusBadge color map with appropriate colors (e.g., yellow for warning/starting, gray for stopped).

---

## BUG-016: `PollLoop` goroutine has no cancellation mechanism

**Severity**: Low
**Component**: `cmd/sockerless-admin/registry.go:98-104`
**Status**: Open

### Description

`PollLoop` runs an infinite `for` loop with no context, stop channel, or other cancellation mechanism. It is only stopped by `os.Exit(0)` in the signal handler. This means there is no way to gracefully stop health polling without killing the entire process — problematic for testing.

### Fix

Accept a `context.Context` parameter and select on `ctx.Done()` between iterations.
