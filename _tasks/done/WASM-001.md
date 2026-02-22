# WASM-001: Wazero runtime + go-busybox embed + dependencies

**Component:** Core (Shared backend library)
**Phase:** 15
**Depends on:** —
**Estimated effort:** M
**Status:** PENDING

---

## Description
Add wazero and mvdan.cc/sh/v3 dependencies to backends/core/go.mod. Build busybox.wasm from go-busybox via TinyGo and commit as binary asset. Create wasm_runtime.go with singleton WasmRuntime that embeds busybox.wasm via `//go:embed` and compiles it once via wazero.

## Key Details
- `github.com/tetratelabs/wazero` — Pure Go WASM runtime, WASI Preview 1, no CGo
- `mvdan.cc/sh/v3` — Go shell interpreter for pipes, &&, ||, etc.
- `github.com/nicholasgasior/go-busybox` — 41 BusyBox applets as WASM
- busybox.wasm built via: `tinygo build -o busybox.wasm -target=wasi -no-debug -scheduler=none ./cmd/busybox`
- Embedded via `//go:embed busybox.wasm` in wasm_runtime.go

## Acceptance Criteria
1. `backends/core/go.mod` includes `github.com/tetratelabs/wazero` and `mvdan.cc/sh/v3`
2. `backends/core/busybox.wasm` exists as committed binary (~2MB)
3. `backends/core/wasm_runtime.go` exports `WasmRuntime` struct with `NewWasmRuntime(ctx)` and `Close(ctx)` methods
4. `NewWasmRuntime` calls `wazero.NewRuntime`, instantiates WASI preview1, and compiles busybox module once
5. `go build ./backends/core/...` passes

## Suggested File Paths
- `backends/core/wasm_runtime.go`
- `backends/core/busybox.wasm`
- `backends/core/go.mod` (modified)

## Notes
- busybox.wasm uses multi-call pattern: argv[0] determines the applet
- Module compiled once, instantiated per command (wazero supports concurrent instantiation)
