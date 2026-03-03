# Bug Sprint 4 — BUG-024 through BUG-033

**Completed**: 2026-03-03
**Branch**: bugs-admin-triage

## Summary

Audited admin Go backend and UI for remaining bugs after Sprint 3. Found and fixed 10 bugs (6 Go, 4 UI).

## Bugs Fixed

### Go Backend (6)

| Bug | Description | Fix |
|-----|-------------|-----|
| BUG-024 | `handleProjectCreate` returns 400 for all errors | Added `createErrorStatus()`: 409 for "already exists", 500 for port/persist, 400 for validation |
| BUG-025 | `projectErrorStatus` doesn't map "is busy" to 409 | Added "is busy" → 409 Conflict |
| BUG-026 | `processErrorStatus` doesn't map conflict errors | "already"/"is not running" → 409 instead of 400 |
| BUG-027 | `ScanStoppedContainers` missing `Created` field | Added `Created` to struct, compute Age from RFC3339 |
| BUG-028 | `handleProjectLogs` returns 404 for invalid component | "invalid component" → 400 instead of 404 |
| BUG-029 | `RingBuffer.Lines` panics on negative `n` | Added `n <= 0` guard, new test |

### Admin UI (4)

| Bug | Description | Fix |
|-----|-------------|-----|
| BUG-030 | `ComponentMetricsPanel` swallows errors | Destructure `isError`/`error`, display error |
| BUG-031 | ProcessesPage empty state below empty table | Conditional render: empty state OR table |
| BUG-032 | ComponentDetailPage no auto-refresh | Added `refetchInterval: 5000` to status/metrics |
| BUG-033 | ProjectCreatePage navigate no encoding | Added `encodeURIComponent(data.name)` |

## Files Modified

- `cmd/sockerless-admin/api_projects.go` — BUG-024, BUG-025, BUG-028
- `cmd/sockerless-admin/api_processes.go` — BUG-026
- `cmd/sockerless-admin/cleanup.go` — BUG-027
- `cmd/sockerless-admin/ring_buffer.go` — BUG-029
- `cmd/sockerless-admin/ring_buffer_test.go` — BUG-029 test
- `ui/packages/admin/src/pages/MetricsPage.tsx` — BUG-030
- `ui/packages/admin/src/pages/ProcessesPage.tsx` — BUG-031
- `ui/packages/admin/src/pages/ComponentDetailPage.tsx` — BUG-032
- `ui/packages/admin/src/pages/ProjectCreatePage.tsx` — BUG-033
- `BUGS.md` — Removed BUG-003→023, added BUG-024→033

## Test Results

- Admin Go: 87 PASS (was 86, +1 TestRingBufferLinesNegative)
- UI Vitest: 92 PASS (unchanged)
- go vet: 0 issues
