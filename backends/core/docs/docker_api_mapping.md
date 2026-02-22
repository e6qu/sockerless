# Docker API Mapping: Core (Shared Library)

The core module provides the shared Docker-compatible API surface that all non-Docker backends inherit. It implements 50+ Docker API endpoints with in-memory state management and a pluggable driver architecture.

## Architecture

Cloud backends embed `core.BaseServer` and override only the handlers that need cloud-specific logic. Everything else is handled by core's default implementations.

### Driver Chain

All execution dispatches through a chain-of-responsibility pattern:

```
Agent → WASM (Process) → Synthetic
```

| Driver Layer | When Active | Purpose |
|-------------|-------------|---------|
| Agent | Container has `AgentAddress` (forward or reverse) | Forwards operations to agent running inside cloud resource |
| WASM | `ProcessFactory` is set (memory backend) | Executes via in-process WASM sandbox |
| Synthetic | Fallback (all cloud backends) | No-op responses for operations without real backend |

### Driver Interfaces

| Interface | Methods | Purpose |
|-----------|---------|---------|
| `ExecDriver` | `Exec(ctx, containerID, execID, cmd, env, workDir, tty, conn)` | Run commands in containers |
| `FilesystemDriver` | `PutArchive`, `GetArchive`, `StatPath`, `RootPath` | Container filesystem operations |
| `StreamDriver` | `Attach(ctx, containerID, tty, conn)`, `LogBytes(containerID)` | Bidirectional I/O and log retrieval |
| `ProcessLifecycleDriver` | `Start`, `Stop`, `Kill`, `Cleanup`, `WaitCh`, `Top`, `Stats`, `IsSynthetic` | Process lifecycle management |

## Container Endpoints (Default Handlers)

These handlers are used when a backend does not provide an override.

### `POST /containers/create` — Create Container

| Aspect | Vanilla Docker | Core Default |
|--------|---------------|--------------|
| What happens | Creates container via Docker daemon | Creates container metadata in `Store.Containers` |
| Image config | Resolved from local image | Merged from `Store.Images` (Env, Cmd, Entrypoint, WorkingDir) |
| Build context | From `docker build` cache | Loads `BuildContexts` files into `StagingDirs` for COPY support |
| Network | Real Docker network | Assigns synthetic IP from bridge IPAM (172.17.0.x) |
| State | `created` | `created` (identical) |

### `POST /containers/{id}/start` — Start Container

| Aspect | Vanilla Docker | Core Default |
|--------|---------------|--------------|
| What happens | Starts process via runc | Calls `ProcessLifecycleDriver.Start()` |
| If process starts | Process runs | Merges staging dir, returns 204 |
| If synthetic | N/A | Logs synthetic message, auto-exits after 50ms (200ms if OpenStdin) |
| Env injection | From container config | Adds `HOSTNAME`, optionally `SOCKERLESS_UID`/`SOCKERLESS_GID` |
| Health check | Docker daemon | Core health check module via exec |

### `POST /containers/{id}/stop` — Stop Container

Calls `ProcessLifecycleDriver.Stop()`, then `Store.StopContainer(id, 0)`.

### `POST /containers/{id}/kill` — Kill Container

Calls `ProcessLifecycleDriver.Kill()`. SIGKILL → exit code 137; other signals → exit code 0.

### `DELETE /containers/{id}` — Remove Container

Force-stops if running, calls `ProcessLifecycleDriver.Cleanup()`, deletes all associated state.

### `POST /containers/{id}/restart` — Restart Container

Stops process, cleans up, re-creates exit channel, re-spawns via `ProcessLifecycleDriver.Start()`. Increments `RestartCount`.

### `POST /containers/{id}/wait` — Wait for Container Exit

Blocks on `WaitChs` channel. Returns immediately if already exited. For synthetic containers with OpenStdin: spawns 2-second auto-stop if no execs remain.

## Container Query Endpoints (Not Overridable)

These are always handled by core, regardless of backend.

### `GET /containers/{id}` — Inspect Container

Returns full `Container` object from `Store.Containers`.

### `GET /containers` — List Containers

Supports `all` query param and filter expressions (label, status).

### `GET /containers/{id}/logs` — Container Logs (Overridable)

| Aspect | Vanilla Docker | Core Default |
|--------|---------------|--------------|
| Source | Docker daemon | `Drivers.Stream.LogBytes(id)` from driver chain |
| Format | Docker multiplexed stream | Docker multiplexed stream (8-byte header: `[streamType, 0, 0, 0, size_BE4]`) |
| Timestamps | From daemon | Prepends RFC3339Nano if requested |
| TTY vs non-TTY | raw-stream vs multiplexed | Same behavior |

### `POST /containers/{id}/attach` — Attach (Overridable)

HTTP hijacks connection. Calls `Drivers.Stream.Attach(ctx, id, tty, conn)`. Returns `101 UPGRADED` with appropriate content type (raw-stream for TTY, multiplexed-stream for non-TTY).

## Exec Endpoints

### `POST /containers/{id}/exec` — Create Exec

Creates `ExecInstance` with command, env, working directory. Allows exec on exited containers if synthetic.

### `POST /exec/{id}/start` — Start Exec (Overridable)

| Aspect | Vanilla Docker | Core Default |
|--------|---------------|--------------|
| Protocol | HTTP hijack | HTTP hijack (101 UPGRADED) |
| Dispatch | nsenter into container | `Drivers.Exec.Exec()` through driver chain |
| Env | Exec-specific env | Container env + exec env merged (exec overrides) |
| Working dir | Exec workdir or container default | Same |
| Auto-stop | N/A | Synthetic containers: 500ms grace → auto-stop if all execs finished |

## Image Endpoints

### `POST /images/create` — Pull Image (Overridable)

| Aspect | Vanilla Docker | Core Default |
|--------|---------------|--------------|
| Download | Real image layers | Synthetic (hash of reference as ID) |
| Config | From registry manifest | Optional real config via `SOCKERLESS_FETCH_IMAGE_CONFIG=true` |
| Pull guard | N/A | Skips if image already exists (prevents overwriting built images) |
| Aliases | By reference | Stored under: imageID, full ref, short name, docker.io/library/ variants |

### `POST /images/load` — Load Image (Overridable)

Discards tar body, creates synthetic image tagged `loaded:latest`.

### `POST /images/build` — Build Image

| Aspect | Vanilla Docker | Core Default |
|--------|---------------|--------------|
| Dockerfile | Full support | Parses: FROM, COPY, ADD, ENV, CMD, ENTRYPOINT, WORKDIR, ARG, LABEL, EXPOSE, USER |
| RUN | Executed | **No-op** (echoed in build output) |
| Multi-stage | Full support | Only final stage kept |
| COPY files | From build context | Staged via `prepareBuildContext()` → injected into container via `BuildContexts` |
| Output | Image layers | Synthetic image with config from Dockerfile |

### Other Image Operations

`handleImageInspect`, `handleImageTag`, `handleImageList`, `handleImageRemove`, `handleImageHistory`, `handleImagePrune` — all operate on `Store.Images` in memory.

## Network Endpoints (Not Overridable)

| Operation | Vanilla Docker | Core Default |
|-----------|---------------|--------------|
| Create | Real network (bridge, overlay, etc.) | In-memory with auto IPAM: `172.{18+n}.0.0/16` |
| List/Inspect | From Docker daemon | From `Store.Networks` |
| Connect | Real network attachment | Allocates IP from IPAM, adds to Container.NetworkSettings |
| Disconnect | Detaches from network | **No-op** (doesn't remove from maps) |
| Remove | Deletes real network | Removes from store (pre-defined networks protected) |
| Prune | Removes unused | Removes networks with empty Containers map |

## Volume Endpoints

| Operation | Vanilla Docker | Core Default |
|-----------|---------------|--------------|
| Create | Real volume (local driver) | In-memory; if WASM active, creates temp directory in `VolumeDirs` |
| List/Inspect | From Docker daemon | From `Store.Volumes` |
| Remove (overridable) | Deletes real volume | Removes from store + cleans up temp directory |
| Prune (overridable) | Removes unused | Removes volumes not mounted by any container |

## Archive Endpoints (Not Overridable)

| Operation | Vanilla Docker | Core Default |
|-----------|---------------|--------------|
| PUT (copy to) | Extracts tar into container | `Drivers.Filesystem.PutArchive(id, path, body)` |
| HEAD (stat) | Returns file info | `Drivers.Filesystem.StatPath(id, path)` → `X-Docker-Container-Path-Stat` header |
| GET (copy from) | Creates tar from container | `Drivers.Filesystem.GetArchive(id, path, w)` → `application/x-tar` |

## Extended Endpoints

| Operation | Vanilla Docker | Core Default |
|-----------|---------------|--------------|
| Top | `ps` inside container | `ProcessLifecycleDriver.Top(id)` or synthetic (Path + Args) |
| Stats | Real cgroup stats | `ProcessLifecycleDriver.Stats(id)` → Docker stats JSON format |
| Rename | Updates container name | Updates `ContainerNames` mapping |
| Pause (overridable) | Freezes cgroups | Sets `Paused=true` (state flag only) |
| Unpause (overridable) | Unfreezes cgroups | Sets `Paused=false` |
| Events | Real Docker events | Empty stream (kept open) |
| Disk Usage | Real disk usage | Calculates from `RootPath` + `VolumeDirs` |
| Prune (overridable) | Removes exited containers | Deletes exited/dead + calls `Cleanup()` |

## Auth

### `POST /auth`

Stores credentials in `Store.Creds`. Always returns success (`"Login Succeeded"`).

## System

### `GET /info`

Returns `BackendDescriptor` with backend-specific fields (name, driver, OS, CPU, memory count).

## CLI Command Mapping (Core Defaults)

| `docker` CLI command | Core default behavior |
|---------------------|---------------------|
| `docker create` | Store container metadata, merge image config |
| `docker start` | `ProcessLifecycleDriver.Start()` or synthetic auto-exit |
| `docker stop` | `ProcessLifecycleDriver.Stop()` |
| `docker kill` | `ProcessLifecycleDriver.Kill()` |
| `docker rm` | Cleanup + remove all state |
| `docker restart` | Stop + cleanup + re-start |
| `docker logs` | `StreamDriver.LogBytes()` |
| `docker attach` | `StreamDriver.Attach()` |
| `docker exec` | `ExecDriver.Exec()` |
| `docker cp to` | `FilesystemDriver.PutArchive()` |
| `docker cp from` | `FilesystemDriver.GetArchive()` |
| `docker pull` | Synthetic image creation |
| `docker build` | Dockerfile parser (RUN no-op) |
| `docker load` | Synthetic image tagged `loaded:latest` |
| `docker network *` | In-memory with synthetic IPAM |
| `docker volume *` | In-memory with optional temp dirs |
| `docker wait` | Block on WaitChs channel |
| `docker top` | `ProcessLifecycleDriver.Top()` |
| `docker stats` | `ProcessLifecycleDriver.Stats()` |
| `docker pause` | State flag only |
| `docker system df` | Calculate from root paths |

## What's Not Supported by Core Defaults

| Feature | Reason |
|---------|--------|
| Real image pulling | Synthetic (backends that need real pulls override ImagePull) |
| Dockerfile RUN | Shell commands not executed during build |
| Real networking | All networks are in-memory with fake IPs |
| Network disconnect | No-op (state not updated) |
| Real events | Empty event stream |
| Signal delivery | WASM: context cancel; Synthetic: no-op |
