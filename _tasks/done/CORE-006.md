# CORE-006: Refactor ACA backend to use core

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** CORE-003
**Estimated effort:** M
**Status:** DONE

---

## Description
Refactored Azure Container Apps backend following same pattern as ECS and Cloud Run. Embedded *core.BaseServer with 15 RouteOverrides for Azure-specific behavior. Deleted networks.go and volumes.go, keeping cloud-specific container and image handling.

## Acceptance Criteria
1. ACA backend builds using core.BaseServer
2. Deleted 2 files (networks.go, volumes.go)
3. All 102 tests pass unchanged
4. 15 RouteOverrides implement ACA-specific behavior
5. Azure Container Apps API integration preserved

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
