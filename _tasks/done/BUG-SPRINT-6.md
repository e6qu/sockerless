# Bug Sprint 6 — BUG-043 through BUG-046

**Completed**: 2026-03-03
**Branch**: bugs-admin-triage

## Bugs Fixed (4)

### BUG-043: `buildStatus` doesn't detect "stopping" state
- **File**: `cmd/sockerless-admin/project_manager.go`
- **Fix**: Added "stopping" check loop after existing "starting" check

### BUG-044: ProcessDetailPage error display uses `||`, hiding concurrent errors
- **File**: `ui/packages/admin/src/pages/ProcessDetailPage.tsx`
- **Fix**: Replaced `||` with array filter+map pattern (same as BUG-041 fix applied to missed page)

### BUG-045: Health badge shows "error" for "unknown" health
- **Files**: `ComponentsPage.tsx`, `ComponentDetailPage.tsx`, `DashboardPage.tsx`
- **Fix**: Mapped "unknown" health to "warning" StatusBadge instead of "error"

### BUG-046: ComponentDetailPage reload doesn't invalidate provider cache
- **File**: `ui/packages/admin/src/pages/ComponentDetailPage.tsx`
- **Fix**: Added `["component-provider", name]` invalidation to reload onSuccess callback

## BUGS.md Cleanup
- Removed 9 fixed Sprint-5 bugs (BUG-034 through BUG-042)
- Added 4 new bugs (BUG-043 through BUG-046) as fixed

## Test Results
- Admin Go: 88 PASS (unchanged)
- Admin UI Vitest: 33 PASS (unchanged)
- Total UI Vitest: 92 PASS (unchanged)
- go vet: 0 issues
