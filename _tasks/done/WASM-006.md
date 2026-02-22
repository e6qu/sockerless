# WASM-006: Enable WASM exec in memory backend

**Component:** Memory Backend
**Phase:** 15
**Depends on:** WASM-003, WASM-004
**Estimated effort:** S
**Status:** PENDING

---

## Description
Set `WasmExec: true` in the memory backend's BackendDescriptor to enable WASM execution for all memory backend containers.

## Key Changes

### backends/memory/server.go
- Add `WasmExec: true` to the `core.BackendDescriptor` passed to `core.NewBaseServer`

## Acceptance Criteria
1. Memory backend descriptor has `WasmExec: true`
2. `docker run alpine echo hello` prints "hello" (real execution via WASM)
3. `docker exec <container> ls /` shows Alpine rootfs contents
4. All existing tests still pass (cloud backends unaffected)
5. `go build ./backends/memory/...` passes

## Suggested File Paths
- `backends/memory/server.go` (modified)
