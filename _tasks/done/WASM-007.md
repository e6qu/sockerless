# WASM-007: Container archive with virtual filesystem

**Component:** Core (Shared backend library)
**Phase:** 15
**Depends on:** WASM-003
**Estimated effort:** M
**Status:** PENDING

---

## Description
Modify archive handlers (put/get/head) to use real filesystem operations on WasmProcess rootDir when WASM is enabled.

## Key Changes

### handle_containers.go

**handlePutArchive** (lines 504-513):
- When `s.WasmRuntime != nil`: extract tar to `wp.rootDir` + path prefix

**handleGetArchive** (lines 539-562):
- When `s.WasmRuntime != nil`: tar up files from `wp.rootDir` + path prefix

**handleHeadArchive** (lines 516-536):
- When `s.WasmRuntime != nil`: stat real file in `wp.rootDir`

## Acceptance Criteria
1. `docker cp file.txt container:/tmp/` copies file into container's virtual filesystem
2. `docker exec container cat /tmp/file.txt` reads the copied file
3. `docker cp container:/tmp/file.txt .` extracts file from container's virtual filesystem
4. HEAD archive returns real file stat information
5. Non-WASM backends continue to use synthetic (discard) behavior
6. `go build ./backends/core/...` passes

## Suggested File Paths
- `backends/core/handle_containers.go` (modified)
