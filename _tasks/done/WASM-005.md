# WASM-005: Shell interpreter + WasmProcess manager

**Component:** Core (Shared backend library)
**Phase:** 15
**Depends on:** WASM-002
**Estimated effort:** L
**Status:** PENDING

---

## Description
Create wasm_shell.go with shell interpreter integration via mvdan.cc/sh/v3 and wasm_process.go with per-container process manager including output fan-out.

## Key Details

### wasm_shell.go (~150 lines)
- `RunShell(ctx, command, env, rootDir, stdin, stdout, stderr) (exitCode int, err error)` — non-interactive
- `RunInteractiveShell(ctx, env, rootDir, stdin, stdout, stderr) error` — REPL mode
- Uses `syntax.NewParser().Parse(src, command)` to parse shell command string
- Creates `interp.Runner` with `interp.ExecHandlers(wasmExecHandler)`
- `wasmExecHandler` intercepts command execution and dispatches to `wr.RunCommand`
- For unknown commands: return `interp.NewExitStatus(127)` ("command not found")
- Interactive: REPL loop reading lines from stdin, writing prompt to stdout

### wasm_process.go (~200 lines)
- `WasmProcess` struct: runtime, rootDir, cancel, logBuf, listeners, exitCode, done
- `NewWasmProcess(wr, cmd, env, rootDir)` — creates temp dir, populates rootfs, spawns goroutine
- Output fan-out: custom `io.Writer` appending to logBuf and broadcasting to listeners
- `Wait() int` — blocks until main process exits
- `Signal()` — cancels context (kills WASM via WithCloseOnContextDone)
- `LogBytes() []byte` — returns accumulated log output
- `Subscribe(id) chan []byte` / `Unsubscribe(id)`
- `Done() <-chan struct{}`
- `Close()` — cancel + wait + remove temp dir
- `RunExec(ctx, cmd, env, tty, stdin, stdout, stderr) int` — exec in same rootDir
- `RunInteractiveShell(ctx, env, tty, stdin, stdout, stderr) int` — interactive shell

## Acceptance Criteria
1. Shell commands with pipes work: `echo hello | tr a-z A-Z` → "HELLO"
2. `&&` and `||` operators work correctly
3. Variable expansion works: `X=hello; echo $X` → "hello"
4. Redirects work: `echo hello > /tmp/test; cat /tmp/test` → "hello"
5. Command substitution works: `echo $(echo hello)` → "hello"
6. Interactive shell reads from stdin and writes prompt to stdout
7. WasmProcess fan-out delivers output to multiple subscribers
8. WasmProcess.Close() cleans up temp directory
9. `go build ./backends/core/...` passes

## Suggested File Paths
- `backends/core/wasm_shell.go`
- `backends/core/wasm_process.go`
