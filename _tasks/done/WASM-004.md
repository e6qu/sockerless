# WASM-004: Wire WASM exec into core exec handler

**Component:** Core (Shared backend library)
**Phase:** 15
**Depends on:** WASM-005
**Estimated effort:** M
**Status:** PENDING

---

## Description
Modify handle_exec.go to dispatch exec commands to WasmProcess when WasmExec is enabled. Handle interactive shell detection for `docker exec -it <container> sh`.

## Key Changes

### handle_exec.go
- After agent bridge checks, before synthetic fallback:
  ```
  } else if s.WasmRuntime != nil {
      if wp, ok := s.Store.Processes.Load(exec.ContainerID); ok {
          proc := wp.(*WasmProcess)
          cmd := append([]string{exec.ProcessConfig.Entrypoint}, exec.ProcessConfig.Arguments...)
          if isShellCommand(cmd) && tty {
              exitCode = proc.RunInteractiveShell(r.Context(), c.Config.Env, tty, conn, conn, conn)
          } else {
              exitCode = proc.RunExec(r.Context(), cmd, c.Config.Env, tty, conn, conn, conn)
          }
      }
  }
  ```
- Add `isShellCommand` helper: checks if cmd[0] is "sh", "bash", "ash", "/bin/sh", etc.
- No auto-stop scheduling for WASM exec — main process exit drives lifecycle

## Acceptance Criteria
1. `docker exec <container> echo hello` runs echo via WASM and returns "hello"
2. `docker exec -it <container> sh` starts interactive shell via mvdan.cc/sh REPL
3. `docker exec <container> sh -c "echo hello | tr a-z A-Z"` works (non-interactive shell)
4. Exit codes from WASM commands propagate correctly
5. Non-WASM backends unaffected
6. `go build ./backends/core/...` passes

## Suggested File Paths
- `backends/core/handle_exec.go` (modified)
