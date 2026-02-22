# sandbox

A pure-Go container process sandbox that executes shell commands inside a WASM runtime, without requiring Docker or any external container runtime.

It combines three components:

- **[wazero](https://github.com/tetratelabs/wazero)** (v1.11.0) — a zero-dependency WebAssembly runtime for Go
- **[go-busybox](https://github.com/rcarmo/go-busybox)** — 41 BusyBox applets compiled to WASM via TinyGo
- **[mvdan.cc/sh](https://github.com/mvdan/sh)** (v3.12.0) — a POSIX shell interpreter in Go

Each container gets an isolated temporary directory with an Alpine-like filesystem layout. Shell commands are parsed by the Go shell interpreter and dispatched to either Go-native builtins or the WASM busybox module.

## Architecture

```
┌──────────────────────────────────────────────────┐
│  Process (per container)                         │
│  ┌────────────────────────────────────────────┐  │
│  │  mvdan.cc/sh — shell interpreter           │  │
│  │  pipes, &&, ||, redirects, variables,      │  │
│  │  command substitution, globbing             │  │
│  └──────┬───────────────┬─────────────────────┘  │
│         │               │                        │
│   Go builtins     wazero WASM runtime            │
│   (21 commands)   (busybox.wasm, 41 applets)     │
│         │               │                        │
│  ┌──────┴───────────────┴─────────────────────┐  │
│  │  /tmp/sandbox-*/                           │  │
│  │  ├── bin/ etc/ usr/ var/ dev/ tmp/ ...     │  │
│  │  └── <volume symlinks>                     │  │
│  └────────────────────────────────────────────┘  │
└──────────────────────────────────────────────────┘
```

### Go-native builtins

These are implemented directly in the exec handler and bypass WASM entirely:

`base64` `basename` `chmod` `chown` `date` `dirname` `env` `hostname`
`id` `ln` `md5sum` `mktemp` `pwd` `readlink` `seq` `sha256sum` `stat`
`tee` `touch` `uname` `which`

### WASM busybox applets

These run as WASM module instantiations via wazero:

`ash` `awk` `cat` `cp` `cut` `diff` `dig` `echo` `false` `find` `free`
`grep` `gunzip` `gzip` `head` `ionice` `kill` `killall` `logname` `ls`
`mkdir` `mv` `nc` `nice` `nohup` `nproc` `pgrep` `pidof` `pkill`
`printf` `ps` `pwd` `renice` `rm` `rmdir` `sed` `setsid` `sh` `sleep`
`sort` `ss` `start-stop-daemon` `tail` `tar` `taskset` `test` `time`
`timeout` `top` `tr` `true` `uniq` `uptime` `users` `w` `watch` `wc`
`wget` `who` `whoami` `xargs`

## Building busybox.wasm

The embedded `busybox.wasm` binary is built from go-busybox using TinyGo inside Docker. A patch script fixes WASI compatibility issues (missing `syscall.Stat_t` fields under WASI Preview 1).

```sh
# From the repository root:
docker build -f .build/busybox.Dockerfile -o . .build/
cp busybox.wasm sandbox/busybox.wasm
```

This runs the Dockerfile at `.build/busybox.Dockerfile`, which:

1. Starts from `tinygo/tinygo:0.39.0`
2. Clones the go-busybox repository
3. Applies WASI compatibility patches (`.build/patch-busybox.sh`) — extracts platform-specific code into build-tagged files for `ls`, `cp`, etc.
4. Builds with `tinygo build -target=wasip1 -opt=z -no-debug -o /busybox.wasm ./cmd/busybox`

The resulting `busybox.wasm` (~2MB) is committed to the repository and embedded into the Go binary at compile time via `//go:embed`.

## Usage

### Basic: run a single command

```go
ctx := context.Background()

rt, _ := sandbox.NewRuntime(ctx)
defer rt.Close(ctx)

rootDir, _ := os.MkdirTemp("", "sandbox-*")
defer os.RemoveAll(rootDir)
sandbox.PopulateRootfs(rootDir)

var stdout, stderr bytes.Buffer
exitCode, _ := rt.RunCommand(ctx, []string{"echo", "hello"}, nil, rootDir, nil, nil, &stdout, &stderr)
fmt.Println(stdout.String()) // hello
```

### Shell commands with pipes and redirects

```go
exitCode, _ := rt.RunShell(ctx,
    `echo "hello world" | grep -o hello > /tmp/out.txt && cat /tmp/out.txt`,
    []string{"PATH=/bin:/usr/bin"},
    rootDir, nil, nil, &stdout, &stderr,
)
```

### Volume mounts

```go
mounts := []sandbox.DirMount{
    {HostPath: "/host/workspace", ContainerPath: "/workspace"},
}
exitCode, _ := rt.RunCommand(ctx, []string{"ls", "/workspace"}, nil, rootDir, mounts, nil, &stdout, &stderr)
```

### Container process lifecycle

```go
// Start a long-running process
proc, _ := sandbox.NewProcess(rt, []string{"tail", "-f", "/dev/null"}, env, binds)

// Run additional commands inside the same container
exitCode := proc.RunExec(ctx, []string{"sh", "-c", "echo hi"}, env, "/workspace", nil, &stdout, &stderr)

// Get stats and process listing
stats := proc.Stats()
top := proc.Top()

// Attach for live output
ch := proc.Subscribe("my-listener")
defer proc.Unsubscribe("my-listener")

// Stop and clean up
proc.Close()
```

## Running tests

```sh
cd sandbox && go test -v -timeout 1m
```

Or from the repository root:

```sh
make test  # runs sandbox + integration tests
```

## Limitations

- **No network access** from within the WASM sandbox (WASI Preview 1 limitation)
- **No fork/exec** — WASI has no process spawning; all commands are interpreted or run as WASM module instantiations
- **No bash/node/python/git** — only busybox applets, Go builtins, and POSIX shell (via `mvdan.cc/sh`)
- **No file permissions model** — `chmod` and `chown` are no-ops
- **No inotify** — `tail -f /dev/null` is detected and handled as a context-blocking keepalive pattern
- **Shell only, not bash** — the interpreter is POSIX sh; bash-specific syntax (arrays, `[[ ]]`, process substitution) is not supported

## Use cases

- **Docker API simulation** — the Sockerless memory backend uses this module to provide real command execution behind a Docker-compatible API, without needing a real Docker daemon
- **CI/CD runner testing** — run GitHub Actions (`act`) and GitLab CI pipelines against an in-memory backend with actual shell execution for most workflow steps
- **Lightweight integration tests** — test Docker-based workflows without Docker, useful in environments where Docker-in-Docker is unavailable or impractical
- **Sandboxed script execution** — execute untrusted shell scripts in an isolated WASM environment with controlled filesystem access and no network
- **Embedded container runtime** — embed a minimal container-like runtime into any Go application, with per-process isolated filesystems, volume mounts, and output capture
