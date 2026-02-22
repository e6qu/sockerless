# CORE-002: Extract common HTTP handlers into BaseServer

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** CORE-001
**Estimated effort:** L
**Status:** DONE

---

## Description
Created BaseServer struct with RouteOverrides pattern to enable selective handler customization. Extracted ~40 common HTTP handlers from backend implementations into handle_containers.go, handle_networks.go, handle_volumes.go, handle_images.go, handle_exec.go, and handle_extended.go. Also created agent.go with BuildAgentEntrypoint for agent injection logic.

## Acceptance Criteria
1. BaseServer registers ~47 routes with nil-override fallback mechanism
2. RouteOverrides allows backends to selectively override specific handlers
3. InitDefaultNetwork creates "bridge" network on startup
4. ListenAndServe starts HTTP server with proper routing
5. All common Docker API endpoints implemented (containers, networks, volumes, images, exec, extended)

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
