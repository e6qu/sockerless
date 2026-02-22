# backend-core

Shared library that implements the Docker-compatible API surface used by all Sockerless backends. Provides in-memory state management, HTTP routing, a pluggable driver architecture, and default handlers for 50+ endpoints.

## Overview

Cloud backends embed `core.BaseServer` and override only the handlers that need cloud-specific logic (typically container create/start/stop/kill/remove and logs). Everything else — exec, attach, archive copy, networks, volumes, images, health checks — is handled by core.

## Driver architecture

Core uses a chain-of-responsibility pattern with four driver interfaces:

| Driver | Responsibility |
|--------|---------------|
| `ExecDriver` | Run commands in containers |
| `FilesystemDriver` | Read/write files in container root (tar archives) |
| `StreamDriver` | Bidirectional I/O for attach and logs |
| `ProcessLifecycleDriver` | Start/stop/wait/signal processes |

Each driver has a `Fallback` field forming the chain:

```
Agent → WASM (Process) → Synthetic
```

- **Agent drivers** route to a forward or reverse `sockerless-agent` connection
- **WASM drivers** execute via the in-process sandbox (memory backend)
- **Synthetic drivers** provide no-op/echo fallbacks for operations without a real backend

## Key types

### BaseServer

The HTTP server that all non-Docker backends embed:

```go
s := core.NewBaseServer(store, descriptor, overrides, logger)
```

- `store` — shared in-memory state
- `descriptor` — static backend metadata (name, driver, OS, CPU, memory)
- `overrides` — `RouteOverrides{}` map of handler functions to replace defaults
- `logger` — zerolog instance

### Store

Generic thread-safe state with `StateStore[T]`:

- `Containers`, `Images`, `Networks`, `Volumes`, `Execs`, `Creds`
- `Processes` (sync.Map) — live `ContainerProcess` instances
- `StagingDirs` (sync.Map) — pre-start archive staging for `docker cp` before `docker start`
- `BuildContexts` (sync.Map) — COPY files from `docker build`
- `WaitChs` (sync.Map) — container exit notification channels
- `LogBuffers` (sync.Map) — buffered container output

### ProcessFactory / ContainerProcess

Interfaces that backends implement to provide real execution:

```go
type ProcessFactory interface {
    NewProcess(cmd, env []string, binds map[string]string) (ContainerProcess, error)
    IsShellCommand(cmd []string) bool
    Close(ctx context.Context) error
}
```

Set `s.ProcessFactory` and call `s.InitDrivers()` to wire up the WASM driver chain.

## API endpoints

Core registers handlers for:

- **Containers** — create, list, inspect, start, stop, kill, remove, restart, pause, unpause, rename, top, stats, wait, attach, logs, prune
- **Exec** — create, inspect, start
- **Images** — pull, inspect, list, load, build, tag, remove, history, prune
- **Networks** — create, list, inspect, connect, disconnect, remove, prune
- **Volumes** — create, list, inspect, remove, prune
- **Archive** — put, head, get (copy files to/from containers)
- **System** — info, events, disk usage
- **Agent** — WebSocket endpoint for reverse agent connections

## Project structure

```
core/
├── server.go                 BaseServer, route registration, InitDrivers
├── store.go                  StateStore[T], Store (all in-memory state)
├── drivers.go                Driver interfaces and DriverSet
├── drivers_synthetic.go      No-op fallback drivers
├── drivers_process.go        WASM sandbox drivers
├── drivers_agent.go          Forward/reverse agent drivers
├── process.go                ContainerProcess, ProcessFactory interfaces
├── handle_containers.go      Create, start, stop, kill, remove
├── handle_containers_query.go  Inspect, list, logs, wait, attach, stats
├── handle_containers_archive.go  Put/head/get archive, tar helpers
├── handle_exec.go            Exec create, inspect, start
├── handle_images.go          Pull, inspect, load, tag, build
├── handle_networks.go        Network CRUD + connect/disconnect
├── handle_volumes.go         Volume CRUD
├── handle_extended.go        Top, stats, rename, pause, events, df
├── agent_registry.go         Reverse agent connection management
├── agent.go                  Agent entrypoint builders
├── build.go                  Dockerfile parser + build handler
├── health.go                 Health check runner
├── registry.go               Docker v2 registry client (opt-in)
├── resolve.go                Container/network/image resolution
├── filters.go                Filter matching for list endpoints
└── helpers.go                JSON/error/ID utilities
```

## Docker API mapping

For a detailed breakdown of how each Docker REST API endpoint and CLI command is handled by the core default handlers — including the driver chain dispatch, what's overridable, and how it compares to vanilla Docker — see [docs/docker_api_mapping.md](docs/docker_api_mapping.md).

## Environment variables

| Variable | Description |
|----------|-------------|
| `SOCKERLESS_FETCH_IMAGE_CONFIG` | Set to `true` to fetch real image configs from Docker Hub / registries |

## Testing

```sh
cd backends/core
go test -v ./...
```

Tests cover the Dockerfile parser, health checks, registry client, and agent registry.
