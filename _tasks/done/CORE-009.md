# CORE-009: Refactor Docker backend to use core helpers

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** CORE-001
**Estimated effort:** S
**Status:** DONE

---

## Description
Docker backend is special as it proxies to Docker SDK rather than implementing handlers. Only replaced local writeJSON/writeError/readJSON with core.WriteJSON/WriteError/ReadJSON via package-level var aliases. Added backend-core dependency. Did not embed BaseServer as Docker backend keeps its own mux and routing.

## Acceptance Criteria
1. Docker backend uses core helper functions (WriteJSON, WriteError, ReadJSON)
2. No BaseServer embedding (keeps own mux and routing)
3. All tests pass unchanged
4. Package-level aliases maintain compatibility with existing code
5. Added backends/core dependency to go.mod

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
