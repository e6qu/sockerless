# CORE-001: Create `backends/core/` Go module scaffold

**Component:** Core (Shared backend library)
**Phase:** 5
**Depends on:** —
**Estimated effort:** M
**Status:** DONE

---

## Description
Created backends/core/ as a new Go module with fundamental state management and helper utilities. The module provides StateStore[T] generic interface for type-safe state management, a composite Store struct aggregating all state stores, and essential helper functions for HTTP handlers, ID generation, and resource resolution.

## Acceptance Criteria
1. backends/core/ module exists and builds
2. StateStore[T] supports Get/Put/Delete/Update/Len/Range operations
3. Store composite struct aggregates all state stores (containers, networks, volumes, images, exec)
4. All helper functions exported (WriteJSON, WriteError, ReadJSON, ParseFilters, GenerateID, GenerateName, GenerateToken, ResolveContainer, ResolveContainerID, ResolveNetwork, ResolveImage)

## Definition of Done
- [x] `go build ./...` passes
- [x] `go vet ./...` passes
- [x] All 102 existing tests pass unchanged
