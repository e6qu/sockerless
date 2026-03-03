# Bug Sprint 3 — BUG-003 through BUG-023

**Completed**: 2026-03-03

## Summary

Fixed 21 bugs across admin Go backend and admin UI:
- 8 Go backend bugs (BUG-003 through BUG-010)
- 4 UI core component bugs (BUG-014, BUG-020, BUG-022, BUG-023)
- 10 admin UI page bugs (BUG-011 through BUG-013, BUG-015 through BUG-019, BUG-021)

## Test Results

- Admin Go: 86 PASS (was 83: +2 RingBuffer Reset + 1 bootstrap error body)
- UI Vitest: 92 PASS (was 89: +2 LogViewer + 1 DataTable a11y)
- go vet: 0 issues

## Files Modified

### Go (7 files)
- `cmd/sockerless-admin/project_manager.go` — BUG-003 (StopAll bypass opLock), BUG-007 (drain health body)
- `cmd/sockerless-admin/process.go` — BUG-004 (cancel context), BUG-008 (reset logs)
- `cmd/sockerless-admin/api_processes.go` — BUG-005 (404 not 400)
- `cmd/sockerless-admin/bootstrap.go` — BUG-006 (include error body)
- `cmd/sockerless-admin/ring_buffer.go` — BUG-008 (Reset method)
- `cmd/sockerless-admin/spa.go` — BUG-009 (safe type assertion)
- `cmd/sockerless-admin/registry.go` — BUG-010 (nil check)

### Go Tests (3 files)
- `api_processes_test.go` — updated 400→404
- `bootstrap_test.go` — new TestBootstrapSimulatorErrorBody
- `ring_buffer_test.go` — new TestRingBufferReset, TestRingBufferResetClearsPartial

### UI Core (3 files)
- `LogViewer.tsx` — BUG-014/020 (ANSI rewrite)
- `DataTable.tsx` — BUG-022 (aria-label)
- `ErrorBoundary.tsx` — BUG-023 (reload button)

### UI Core Tests (2 files)
- `LogViewer.test.tsx` — +2 tests
- `DataTable.test.tsx` — +1 test

### Admin UI (12 files)
- `api.ts` — BUG-011 (encodeURIComponent)
- `ProcessesPage.tsx` — BUG-012, BUG-013
- `ProjectsPage.tsx` — BUG-012, BUG-013
- `ProcessDetailPage.tsx` — BUG-013
- `ProjectDetailPage.tsx` — BUG-015, BUG-016, BUG-019, BUG-013
- `ProjectLogsPage.tsx` — BUG-015
- `ComponentDetailPage.tsx` — BUG-013, BUG-017
- `ProjectCreatePage.tsx` — BUG-018
- `MetricsPage.tsx` — BUG-021
- `ContainersPage.tsx` — BUG-021
- `ResourcesPage.tsx` — BUG-021
- `ContextsPage.tsx` — BUG-021
- `DashboardPage.tsx` — BUG-021
