# WASM-002: WASM command runner + Alpine rootfs skeleton

**Component:** Core (Shared backend library)
**Phase:** 15
**Depends on:** WASM-001
**Estimated effort:** S
**Status:** PENDING

---

## Description
Create wasm_exec.go with RunCommand method that instantiates the compiled busybox module with WASI config (args, env, stdin/stdout/stderr, dir mount). Create wasm_rootfs.go with PopulateRootfs function that creates Alpine-like directory structure and minimal config files in a temp directory.

## Key Details

### wasm_exec.go (~120 lines)
- `RunCommand(ctx, args, env, rootDir, stdin, stdout, stderr) (exitCode int, err error)`
- Instantiates compiled module with `WithName("")` (anonymous for concurrency)
- Configures WASI: `WithArgs(args...)`, `WithEnv(env...)`, `WithStdin`, `WithStdout`, `WithStderr`
- Mounts rootDir as `/` via `WithDirMount(rootDir, "/")`
- Extracts exit code from `*sys.ExitError`
- `WithCloseOnContextDone(true)` for cancellation

### wasm_rootfs.go (~80 lines)
- `PopulateRootfs(dir string) error`
- Creates: `/bin`, `/sbin`, `/usr/bin`, `/usr/sbin`, `/usr/local/bin`
- Creates: `/etc` with `passwd`, `group`, `hostname`, `hosts`, `resolv.conf`
- Creates: `/tmp`, `/var/tmp`, `/var/log`
- Creates: `/dev` (empty), `/home`, `/root`, `/proc`, `/sys` (empty dirs)

## Acceptance Criteria
1. `RunCommand` executes a busybox applet (e.g., `["echo", "hello"]`) and captures stdout
2. Exit code extracted correctly from WASM module exit
3. `PopulateRootfs` creates all expected directories and files
4. `go build ./backends/core/...` passes

## Suggested File Paths
- `backends/core/wasm_exec.go`
- `backends/core/wasm_rootfs.go`
