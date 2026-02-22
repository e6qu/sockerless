# Docker API Mapping: Memory Backend

The memory backend executes container workloads entirely in-process using a WASM sandbox (wazero + mvdan.cc/sh + go-busybox). No cloud resources or Docker daemon required.

## Overview

The memory backend uses `core.BaseServer` with **no route overrides**. Instead, it injects a `ProcessFactory` that activates the WASM driver chain. All Docker API endpoints are handled by core's default handlers, which dispatch through the driver chain:

```
Agent → WASM (Process) → Synthetic
```

In normal mode, the WASM layer handles execution. In synthetic mode (`SOCKERLESS_SYNTHETIC=1`), everything falls through to the synthetic no-op layer.

## Container Lifecycle

### `POST /containers/create` — Create Container

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Creates a container from an image on the local daemon | Creates container metadata in memory |
| Image | Must exist locally (real layers) | Synthetic image (metadata only, no layers) |
| Entrypoint/Cmd | Stored as container config | Stored; used to spawn WASM process on start |
| Environment | Stored as container config | Stored; passed to WASM shell interpreter |
| Network | Joins real Docker network | Synthetic IP assigned from virtual bridge (172.17.0.x) |
| Filesystem | Writable layer on top of image | Temp directory per container as rootfs |
| Build context | From `docker build` layers | COPY files loaded from `BuildContexts` into `StagingDirs` |

### `POST /containers/{id}/start` — Start Container

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Starts the container process via runc | Calls `ProcessFactory.NewProcess()` to create WASM sandbox process |
| Process | Real Linux process (namespaces, cgroups) | wazero WASM runtime + mvdan.cc/sh shell interpreter |
| Shell | Real `/bin/sh` (bash/dash) | WASM-compiled shell (mvdan.cc/sh v3.12.0) |
| Utilities | Full Linux userland | Busybox applets compiled to WASM via TinyGo |
| Filesystem | Union mount (image layers + writable) | Temp directory with symlinks for volumes + WASI DirMounts |
| Volumes | Real bind mounts / Docker volumes | Symlinks in rootDir + WASI directory mounts |
| Staging | N/A | `mergeStagingDir()` copies pre-start archive files into container root |
| PID | Real process PID | PID 42 (synthetic, fixed) |
| HOSTNAME | Set in UTS namespace | Injected as env var |
| Health checks | Docker daemon runs checks | Core health check module runs checks via exec |

**WASM sandbox capabilities:**
- Shell builtins: cd, pwd, echo, printf, test, [, cat, ls, mkdir, cp, mv, rm, chmod, touch, head, tail, wc, sort, tr, env, which, dirname, basename, sleep
- PATH-aware command resolution: checks PATH dirs for scripts before busybox applets
- `sh -c 'cmd'`, `sh -e script.sh`, `sh -e -c 'cmd'` patterns supported
- `tail -f /dev/null` keepalive: blocks on context (WASI has no inotify)

### `POST /containers/{id}/stop` — Stop Container

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Sends SIGTERM then SIGKILL | Calls `ProcessLifecycleDriver.Stop()` which cancels the WASM context |
| Grace period | Configurable timeout | Immediate (context cancellation) |

### `POST /containers/{id}/kill` — Kill Container

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Sends specified signal | Calls `ProcessLifecycleDriver.Kill()` — cancels WASM context |
| Signal support | Full POSIX signals | SIGKILL → exit code 137; others → exit code 0 |

### `DELETE /containers/{id}` — Remove Container

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Removes container + filesystem | Calls `ProcessLifecycleDriver.Cleanup()` — removes temp directory, releases resources |

### `POST /containers/{id}/restart` — Restart Container

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Stops then starts | Stops old process, cleans up, creates new WASM process |

## Exec

### `POST /containers/{id}/exec` + `POST /exec/{id}/start`

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| Execution | nsenter into container namespaces | `process.RunExec(ctx, cmd, env, workDir, stdin, stdout, stderr)` |
| Shell | Real shell in container | mvdan.cc/sh interpreter |
| Working directory | Resolved in container | `cd <workdir> && <cmd>` wrapper via shell |
| Environment | Inherited from container + exec overrides | Container env + exec env merged (exec overrides) |
| TTY | Real PTY allocation | Handled by sandbox |
| Interactive | stdin pipe to process | `process.RunInteractiveShell()` for bare `sh` |
| Exit code | From process exit | From shell interpreter exit |

## Images

### `POST /images/create` — Pull Image

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Downloads image layers | Creates synthetic image (metadata only) |
| Image config | Real manifest | Optional real config via `SOCKERLESS_FETCH_IMAGE_CONFIG=true` |
| Layers | Stored on disk | None (no real layers) |

### `POST /images/load` — Load Image

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Imports image from tar | Discards tar body, creates synthetic image with tag `loaded:latest` |

### `POST /images/build` — Build Image

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Builds image from Dockerfile | Core Dockerfile parser: supports FROM, COPY, ADD, ENV, CMD, ENTRYPOINT, WORKDIR, ARG, LABEL, EXPOSE, USER |
| RUN instructions | Executed in build container | **No-op** (echoed in output but not executed) |
| COPY/ADD | Copies from build context | Files staged via `prepareBuildContext()` → loaded into container via `BuildContexts` |
| Multi-stage | Full support | Only final stage kept |

## Logs

### `GET /containers/{id}/logs` — Container Logs

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| Source | Container stdout/stderr from daemon | `Drivers.Stream.LogBytes(id)` from WASM process output capture |
| Real-time | Streaming from daemon | Captured output bytes returned |
| Follow | Continuous streaming | Process output subscription |
| Timestamps | From Docker daemon | Synthetic |
| Format | Docker multiplexed stream | Docker multiplexed stream (8-byte header) |

## Attach

### `POST /containers/{id}/attach`

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Attaches to container stdin/stdout/stderr | `Drivers.Stream.Attach()` — connects to WASM process I/O |
| Protocol | HTTP hijack + multiplexed stream | Same protocol (HTTP 101 upgrade) |
| TTY | Raw stream | Raw stream |
| Non-TTY | Multiplexed stream (8-byte headers) | Multiplexed stream |

## Networks

All handled by core. Entirely synthetic.

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| Network creation | Creates real Docker network (bridge, overlay, etc.) | In-memory tracking with synthetic IPAM |
| IP allocation | Real IPs from IPAM | Sequential IPs (172.17.0.x, 172.18.0.x, etc.) |
| DNS | Docker DNS server for container name resolution | **Not available** |
| Inter-container networking | Real TCP/UDP between containers | **Not available** (containers are isolated WASM processes) |
| Connect/disconnect | Real network attachment | State tracking only |

## Volumes

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| Volume creation | Creates real Docker volume (local driver, etc.) | Creates temp directory on host, tracked in `VolumeDirs` |
| Bind mounts | Real host → container path binding | Symlinks in container rootDir + WASI DirMounts |
| Named volumes | Persistent Docker volumes | Temp directories shared between containers |
| Volume data | Persists across container restarts | Persists within session (temp dirs deleted on cleanup) |
| Overlapping mounts | Docker handles via union FS | Sorted shortest-first, sub-paths use symlinks |

## Archive (Copy)

### `PUT /containers/{id}/archive` — Copy to Container

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Extracts tar into container filesystem | Extracts tar into container root temp directory |
| Before start | Writes to container layer | Staged in `StagingDirs`, merged on start via `mergeStagingDir()` |

### `GET /containers/{id}/archive` — Copy from Container

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Creates tar from container filesystem | Creates tar from container root temp directory |

### `HEAD /containers/{id}/archive` — Stat Path

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| What happens | Returns file stat info | Stats file in container root directory |

## System

| Aspect | Vanilla Docker | Memory Backend |
|--------|---------------|----------------|
| Info | Real daemon info | Static: Driver=memory, OS=WASM Sandbox |
| Disk usage | Real disk usage | Calculated from container root dirs + volume dirs |
| Events | Real Docker events | Empty stream (kept open) |
| Stats | Real cgroup stats | WASM runtime stats (memory, CPU nanos, PIDs) |
| Top | Real `ps` output | Process list from WASM runtime |

## CLI Command Mapping

| `docker` CLI command | Vanilla Docker | Memory Backend |
|---------------------|---------------|----------------|
| `docker create <image>` | Creates container | Creates container metadata (WASM process created on start) |
| `docker start <id>` | Starts via runc | Spawns WASM sandbox process |
| `docker stop <id>` | SIGTERM + SIGKILL | Context cancellation |
| `docker kill <id>` | Send signal | Context cancellation (SIGKILL → 137) |
| `docker rm <id>` | Remove container + fs | Remove metadata + temp directory |
| `docker logs <id>` | Read from daemon | Read captured WASM output |
| `docker exec <id> <cmd>` | nsenter | Shell interpreter exec |
| `docker cp <src> <id>:<dst>` | Write to container layer | Write to container temp directory |
| `docker pull <image>` | Download layers | Synthetic (metadata only) |
| `docker build .` | Build from Dockerfile | Core parser (RUN is no-op, COPY files staged) |
| `docker load` | Import from tar | Creates synthetic image |
| `docker network create` | Create real network | In-memory tracking |
| `docker volume create` | Create real volume | Create temp directory |
| `docker pause <id>` | Freeze cgroups | State flag only (process continues) |
| `docker stats <id>` | Real cgroup stats | WASM runtime stats |
| `docker top <id>` | Real `ps` | WASM process list |
| `docker attach <id>` | Attach to container I/O | Attach to WASM process I/O |

## Synthetic Mode (`SOCKERLESS_SYNTHETIC=1`)

When synthetic mode is enabled, the WASM sandbox is disabled. All operations fall through to the synthetic driver:

| Operation | Synthetic behavior |
|-----------|-------------------|
| Container start | Logs synthetic message, auto-exits after 50ms (or 200ms if OpenStdin) |
| Exec | Returns empty output with exit code 0 |
| Logs | Returns buffered synthetic log message |
| Archive put | Stored in staging dirs |
| Archive get | Returns empty tar |
| Stats | Returns zero stats |

This mode is required for `gitlab-runner` which uses helper binaries (`gitlab-runner-helper`, `gitlab-runner-build`) that cannot run in WASM.

## Summary: What's Not Supported

| Feature | Reason |
|---------|--------|
| Real image layers | No image filesystem (uses temp dirs instead) |
| Real networking | WASM processes are isolated; no TCP/UDP between containers |
| DNS resolution | No DNS server for container name → IP resolution |
| Signal delivery (POSIX) | WASM has no signal mechanism; stop/kill cancel context |
| Dockerfile RUN | Shell commands in Dockerfile not executed during build |
| fork/exec | WASI Preview 1 has no process spawning |
| inotify / filesystem watching | WASI has no inotify; `tail -f` blocks on context |
| Real cgroups | No resource isolation (all runs in single process) |
| Pause/unpause | State flag only; WASM process cannot be frozen |
| Native binaries | Only busybox applets and shell scripts; no ELF/Mach-O execution |
