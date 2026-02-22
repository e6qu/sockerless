# CORE-007: Refactor Lambda backend to use core

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** CORE-003
**Estimated effort:** M
**Status:** DONE

---

## Description
Refactored AWS Lambda backend following same pattern as other cloud backends. Embedded *core.BaseServer with RouteOverrides for Lambda-specific behavior. Deleted networks.go and volumes.go, keeping Lambda-specific function handling.

## Acceptance Criteria
1. Lambda backend builds using core.BaseServer
2. Deleted 2 files (networks.go, volumes.go)
3. All 102 tests pass unchanged
4. RouteOverrides implement Lambda-specific behavior
5. AWS Lambda API integration preserved

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
