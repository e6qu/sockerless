# backend-memory

In-memory backend that executes container workloads in a WASM sandbox. No cloud resources or Docker daemon required — everything runs in a single process.

## Overview

The memory backend uses `core.BaseServer` with no route overrides. Instead, it injects a `ProcessFactory` that creates WASM-based container processes using the [sandbox](../../sandbox/) module. This activates the WASM driver chain in core, giving containers a real filesystem, shell interpreter, and busybox utilities — all running in WebAssembly via wazero.

When `SOCKERLESS_SYNTHETIC=1` is set, the WASM sandbox is disabled and all operations fall through to the synthetic (no-op) drivers. This is used for CI runners like gitlab-runner that require their own helper binaries which cannot run in WASM.

## Building

```sh
cd backends/memory
go build -o sockerless-backend-memory ./cmd/main.go
```

## Usage

```sh
# Normal mode (WASM sandbox enabled)
sockerless-backend-memory

# Synthetic mode (for gitlab-runner compatibility)
SOCKERLESS_SYNTHETIC=1 sockerless-backend-memory
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:9100` | Listen address |
| `-log-level` | `info` | Log level: debug, info, warn, error |

## Environment variables

| Variable | Description |
|----------|-------------|
| `SOCKERLESS_SYNTHETIC` | Set to `1` to disable WASM sandbox (synthetic mode) |

## How it works

1. `NewServer()` initializes a `core.BaseServer` with default handlers
2. A `sandboxFactory` is created wrapping `sandbox.NewRuntime()`
3. The factory is set as `s.ProcessFactory` and `s.InitDrivers()` rebuilds the driver chain
4. Container start calls `ProcessFactory.NewProcess()` which creates a WASM process with:
   - A shell interpreter (mvdan.cc/sh)
   - Busybox applets (compiled to WASM via TinyGo)
   - An isolated virtual filesystem per container
   - Volume bind mounts between containers

The `sandboxProcess` adapter in `adapter.go` bridges the `sandbox.Process` interface to core's `ContainerProcess` interface.

## Project structure

```
memory/
├── cmd/
│   └── main.go      CLI entrypoint
├── server.go        NewServer, BaseServer setup, ProcessFactory injection
├── adapter.go       sandboxFactory + sandboxProcess wrappers
└── go.mod           Dependencies (backend-core, sandbox, wazero)
```

## Docker API mapping

For a detailed breakdown of how each Docker REST API endpoint and CLI command maps to WASM sandbox operations — including what's supported, what's not, synthetic mode behavior, and how it compares to vanilla Docker — see [docs/docker_api_mapping.md](docs/docker_api_mapping.md).

## Testing

The memory backend is exercised by the E2E test suites:

```sh
# Upstream act tests (GitHub Actions)
make upstream-test-act

# Upstream gitlab-ci-local tests
make upstream-test-gitlab-ci-local

# Sandbox unit tests
cd sandbox && go test -v ./...
```
