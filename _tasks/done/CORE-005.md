# CORE-005: Refactor Cloud Run backend to use core

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** CORE-003
**Estimated effort:** M
**Status:** DONE

---

## Description
Refactored Cloud Run backend following same pattern as ECS. Embedded *core.BaseServer with 15 RouteOverrides for GCP Cloud Run specific behavior. Deleted networks.go and volumes.go, keeping cloud-specific container and image handling.

## Acceptance Criteria
1. Cloud Run backend builds using core.BaseServer
2. Deleted 2 files (networks.go, volumes.go)
3. All 102 tests pass unchanged
4. 15 RouteOverrides implement Cloud Run-specific behavior
5. GCP Cloud Run API integration preserved

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
