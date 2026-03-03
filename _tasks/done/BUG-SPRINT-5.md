# Bug Sprint 5 — BUG-034 through BUG-042

**Date**: 2026-03-03
**Status**: Complete

## Summary

Fixed 9 bugs (2 Go backend, 7 admin UI) discovered during audit round 5 of `cmd/sockerless-admin/` and `ui/packages/admin/`.

## Bugs Fixed

### Go Backend (2)
- **BUG-034**: `ProcessManager.Stop` race condition — added generation check using `doneCh` identity to prevent clobbering re-started process PID/cancel
- **BUG-035**: `handleOverview` counts "unknown" health as "down" — changed to only count explicitly "down"

### Admin UI (7)
- **BUG-036**: ProjectDetailPage Start button enabled during "stopping"
- **BUG-037**: ProjectsPage Start button enabled during "stopping"
- **BUG-038**: ProjectDetailPage Delete button enabled during starting/stopping
- **BUG-039**: ComponentDetailPage uptime only shows minutes (inconsistent with ComponentsPage)
- **BUG-040**: ProjectCreatePage double-submission via rapid Enter
- **BUG-041**: Error display `||` hides concurrent errors (ProcessesPage, ProjectsPage, ProjectDetailPage)
- **BUG-042**: App.tsx no catch-all 404 route

## Files Modified

### Go
- `cmd/sockerless-admin/process.go` — generation check in Stop()
- `cmd/sockerless-admin/api_overview.go` — health counting fix
- `cmd/sockerless-admin/api_processes_test.go` — TestProcessStopThenStartRace

### UI
- `ui/packages/admin/src/pages/ProjectDetailPage.tsx` — BUG-036, 038, 041
- `ui/packages/admin/src/pages/ProjectsPage.tsx` — BUG-037, 041
- `ui/packages/admin/src/pages/ComponentDetailPage.tsx` — BUG-039
- `ui/packages/admin/src/pages/ProjectCreatePage.tsx` — BUG-040
- `ui/packages/admin/src/pages/ProcessesPage.tsx` — BUG-041
- `ui/packages/admin/src/App.tsx` — BUG-042

## Test Results
- Admin Go: 88 PASS (was 87, +1 Stop/Start race test)
- UI Vitest: 92 PASS (unchanged)
- go vet: 0 issues
