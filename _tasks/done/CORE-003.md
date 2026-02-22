# CORE-003: Refactor memory backend to use core

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** CORE-002
**Estimated effort:** M
**Status:** DONE

---

## Description
Refactored memory backend to embed *core.BaseServer, eliminating duplicate code. Deleted store.go and most handler files (networks.go, volumes.go, exec.go, images.go, extended.go). Server struct reduced from ~1200 lines to ~300 lines while maintaining full functionality and test compatibility.

## Acceptance Criteria
1. Memory backend builds with no local StateStore implementation
2. All 102 tests pass unchanged
3. Server embeds *core.BaseServer
4. Deleted files: store.go, networks.go, volumes.go, exec.go, images.go, extended.go
5. Memory backend serves as reference implementation for core usage

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
