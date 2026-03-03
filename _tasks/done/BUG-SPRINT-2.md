# Bug Sprint 2 — Admin Module Bugs (BUG-003 through BUG-016)

**Completed**: 2026-03-03

## Summary

Fixed 14 admin bugs across Go backend (`cmd/sockerless-admin/`) and admin UI (`ui/packages/admin/`, `ui/packages/core/`).

## Bugs Fixed

### Go Backend (6 bugs)
- **BUG-007/008**: Per-project operation lock (`opLock`) prevents concurrent Start/Stop/Delete
- **BUG-009**: `stopIfRunning` tolerates "not running" errors (TOCTOU fix)
- **BUG-010**: Port leak on Reserve failure — release auto-allocated ports
- **BUG-011**: Graceful shutdown via `http.Server.Shutdown()` instead of `os.Exit(0)`
- **BUG-012**: `sockerlessDir()` falls back to `os.TempDir()` when HOME is unset
- **BUG-016**: `PollLoop` accepts `done` channel for cancellation

### UI (8 bugs)
- **BUG-003**: LogViewer XSS fix — HTML escape before ANSI replacement + multi-code support
- **BUG-004**: DataTable `onRowClick` prop replaces DOM scraping in ComponentsPage
- **BUG-005**: Error states on ProjectDetailPage and ProjectLogsPage
- **BUG-006**: Confirmation dialogs on CleanupPage destructive buttons
- **BUG-013**: Invalidate connection query on start/stop
- **BUG-014**: Client-side project name validation with regex + form semantics
- **BUG-015**: StatusBadge color map for warning/starting/stopping/stopped

## Files Changed
- `cmd/sockerless-admin/project_manager.go` — opLock, stopProcesses, stopIfRunning, port release
- `cmd/sockerless-admin/config.go` — TempDir fallback
- `cmd/sockerless-admin/registry.go` — PollLoop done channel
- `cmd/sockerless-admin/main.go` — http.Server graceful shutdown
- `cmd/sockerless-admin/project_test.go` — 3 new tests
- `cmd/sockerless-admin/config_test.go` — 2 new tests (new file)
- `cmd/sockerless-admin/registry_test.go` — 1 new test (new file)
- `ui/packages/core/src/components/LogViewer.tsx` — escapeHtml + multi-code ANSI
- `ui/packages/core/src/components/StatusBadge.tsx` — 4 new color mappings
- `ui/packages/core/src/components/DataTable.tsx` — onRowClick prop
- `ui/packages/core/src/__tests__/LogViewer.test.tsx` — 2 new tests
- `ui/packages/core/src/__tests__/DataTable.test.tsx` — 1 new test
- `ui/packages/admin/src/pages/ProjectDetailPage.tsx` — error state + connection invalidation
- `ui/packages/admin/src/pages/ProjectLogsPage.tsx` — error state
- `ui/packages/admin/src/pages/CleanupPage.tsx` — confirmation dialogs
- `ui/packages/admin/src/pages/ComponentsPage.tsx` — onRowClick
- `ui/packages/admin/src/pages/ProjectCreatePage.tsx` — name validation + form

## Test Results
- Go admin: 83 PASS (was 77)
- UI Vitest: 89 PASS (was 86)
