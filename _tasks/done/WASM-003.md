# WASM-003: Wire WASM exec into core container handlers

**Component:** Core (Shared backend library)
**Phase:** 15
**Depends on:** WASM-005
**Estimated effort:** L
**Status:** PENDING

---

## Description
Modify handle_containers.go, store.go, and server.go to use WasmProcess for start/stop/kill/remove/logs/attach/restart when WasmExec is enabled in BackendDescriptor.

## Key Changes

### server.go
- Add `WasmExec bool` to `BackendDescriptor`
- Add `WasmRuntime *WasmRuntime` to `BaseServer`
- In `NewBaseServer`: if `desc.WasmExec`, init `WasmRuntime`

### store.go
- Add `Processes sync.Map` to `Store` (containerID → *WasmProcess)

### handle_containers.go
- **handleContainerStart**: When `s.WasmRuntime != nil` and container has cmd, spawn `NewWasmProcess`, store in `s.Store.Processes`, goroutine waits for exit then calls `StopContainer`. Skip 50ms auto-stop.
- **handleContainerStop**: When WASM, call `wp.Signal()`
- **handleContainerKill**: When WASM, call `wp.Signal()`
- **handleContainerRemove**: When WASM, call `wp.Close()`, delete from Processes map
- **handleContainerLogs**: When WASM, read from `wp.LogBytes()`
- **handleContainerAttach**: When WASM, subscribe to output fan-out, stream with mux frames, wait for exit
- **handleContainerRestart**: When WASM, stop old WasmProcess, spawn new one

## Acceptance Criteria
1. `BackendDescriptor` has `WasmExec bool` field
2. `BaseServer` conditionally initializes `WasmRuntime` based on `desc.WasmExec`
3. Container start spawns a WasmProcess when WASM is enabled
4. Container stop/kill signals the WasmProcess
5. Container remove cleans up temp dir and process
6. Container logs returns real output from WASM execution
7. Container attach streams real output
8. Container restart stops and re-spawns WasmProcess
9. Non-WASM backends are completely unaffected (synthetic fallback remains)
10. `go build ./backends/core/...` passes

## Suggested File Paths
- `backends/core/server.go` (modified)
- `backends/core/store.go` (modified)
- `backends/core/handle_containers.go` (modified)
