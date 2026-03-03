# Bug Fix Sprint — BUGS.md Triage & Fixes

**Completed**: 2026-03-03

## Summary

Triaged all 20 bugs (BUG-001 through BUG-020). Removed 3 invalid/fixed, deferred 2 architectural, fixed 15.

## Bugs Fixed

| Bug | File(s) | Fix |
|-----|---------|-----|
| BUG-004 | `backends/core/server.go` | Added LoggingMiddleware (method, path, status, duration) |
| BUG-005 | `cmd/sockerless-admin/project_manager.go` | Local vars for BackendPort/FrontendMgmtPort before unlock |
| BUG-007 | `cmd/sockerless-admin/project_manager.go` | Reserve all non-zero ports regardless of auto count |
| BUG-008 | `cmd/sockerless-admin/project_manager.go` | Log warning on port reservation failure in LoadProject |
| BUG-009 | `cmd/sockerless-admin/cleanup.go` | Length guard before ID[:12] |
| BUG-011 | `cmd/sockerless-admin/ring_buffer.go` | Partial line carry-over buffer |
| BUG-012 | `cmd/sockerless-admin/project.go` | isValidProjectName regex validation |
| BUG-013 | `cmd/sockerless-admin/project_manager.go` | Propagate SaveProject error with rollback |
| BUG-014 | `cmd/sockerless-admin/cleanup.go` | Check ESRCH specifically |
| BUG-015 | `cmd/sockerless-admin/project_manager.go` | errors.Join instead of errs[0] |
| BUG-016 | `cmd/sockerless-admin/api_projects.go` | 404 for not found, 500 for server errors |
| BUG-017 | 8 admin UI pages | Error state with red error box |
| BUG-018 | ComponentDetailPage, ProcessDetailPage | "Not found" message for missing items |
| BUG-019 | ProcessesPage, ProjectsPage | Per-row pending state via mutation.variables |
| BUG-020 | CleanupPage | Error display for all 4 mutations |

## Tests

- Admin Go: 77 PASS (was 70, +7 new)
- Admin Vitest: 33 PASS (unchanged)
- Lint: 0 issues
