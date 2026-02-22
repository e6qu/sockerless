# CORE-008: Refactor GCF and AZF backends to use core

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** CORE-003
**Estimated effort:** M
**Status:** DONE

---

## Description
Refactored both cloudrun-functions (GCF) and azure-functions (AZF) backends to use core.BaseServer. Deleted networks.go and volumes.go from each backend. Both backends now follow the same pattern as other cloud backends with RouteOverrides for function-specific behavior.

## Acceptance Criteria
1. Both cloudrun-functions and azure-functions backends build using core.BaseServer
2. Deleted 4 files total (networks.go and volumes.go from each backend)
3. All 102 tests pass unchanged
4. RouteOverrides implement function-specific behavior for both clouds
5. GCF and Azure Functions API integration preserved

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
