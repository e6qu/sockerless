# CORE-004: Refactor ECS backend to use core

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** CORE-003
**Estimated effort:** M
**Status:** DONE

---

## Description
Refactored ECS backend to embed *core.BaseServer with 14 RouteOverrides for cloud-specific behavior. Deleted store.go (StateStore implementation), networks.go, and volumes.go. Kept cloud-specific files (containers.go, images.go, agent_inject.go, etc.) to handle AWS ECS API integration.

## Acceptance Criteria
1. ECS backend builds using core.BaseServer
2. Deleted 3 files of duplicated code (store.go, networks.go, volumes.go)
3. All 102 tests pass unchanged
4. 14 RouteOverrides implement ECS-specific behavior
5. Cloud-specific functionality preserved

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
