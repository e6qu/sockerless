# Docker API Mapping: Docker Backend

The Docker backend is a direct passthrough to a local Docker daemon via the Docker Go SDK. Every API call maps 1:1 to the real Docker API.

## Overview

Unlike the cloud backends which use `core.BaseServer`, the Docker backend implements all route handlers directly using `github.com/docker/docker/client`. No agent, driver chain, or synthetic fallback is involved — operations go straight to the Docker daemon.

## Container Lifecycle

### `POST /containers/create` — Create Container

| Aspect | Vanilla Docker | Docker Backend |
|--------|---------------|----------------|
| What happens | Creates a container | Translates to `ContainerCreate` via Docker SDK |
| Image | Must exist locally | **Auto-pulls** if not found locally (`ImageInspectWithRaw` check → `ImagePull` fallback) |
| Config mapped | All fields | Image, Cmd, Env, Labels, Tty, OpenStdin, AttachStdin/Stdout/Stderr, WorkingDir, Entrypoint, User, Hostname, StopSignal |
| HostConfig mapped | All fields | NetworkMode, Binds, AutoRemove |
| Difference from vanilla | None | Auto-pull behavior is additive (vanilla requires explicit pull or `--pull=always`) |

### `POST /containers/{id}/start` — Start Container

Direct passthrough to `ContainerStart`. No differences from vanilla Docker.

### `POST /containers/{id}/stop` — Stop Container

| Aspect | Vanilla Docker | Docker Backend |
|--------|---------------|----------------|
| Timeout | Default or specified via `-t` | `t` query parameter mapped to stop timeout |
| Behavior | SIGTERM → SIGKILL | Identical |

### `POST /containers/{id}/kill` — Kill Container

| Aspect | Vanilla Docker | Docker Backend |
|--------|---------------|----------------|
| Signal | Specified via `--signal` | `signal` query parameter (defaults to SIGKILL) |
| Behavior | Delivers signal | Identical |

### `DELETE /containers/{id}` — Remove Container

| Aspect | Vanilla Docker | Docker Backend |
|--------|---------------|----------------|
| Force | `--force` flag | `force` query parameter |
| Volumes | `--volumes` flag | `v` query parameter |
| Behavior | Removes container + optional volumes | Identical |

### `POST /containers/{id}/restart` — Restart Container

Direct passthrough. Supports `t` query parameter for stop timeout.

### Other Container Operations

All direct passthroughs:

| Operation | Docker SDK Method | Notes |
|-----------|-------------------|-------|
| `GET /containers` (list) | `ContainerList` | Supports `all` query param |
| `GET /containers/{id}` (inspect) | `ContainerInspect` | Full container state returned |
| `POST /containers/{id}/wait` | `ContainerWait` | Supports `condition` query param |
| `POST /containers/{id}/attach` | `ContainerAttach` | HTTP hijack, multiplexed stream |
| `GET /containers/{id}/logs` | `ContainerLogs` | Supports stdout, stderr, follow, timestamps, tail |
| `GET /containers/{id}/top` | `ContainerTop` | Supports `ps_args` query param |
| `GET /containers/{id}/stats` | `ContainerStatsOneShot` or streaming | Supports `stream` query param |
| `POST /containers/{id}/rename` | `ContainerRename` | `name` query param required |
| `POST /containers/{id}/pause` | `ContainerPause` | Full support |
| `POST /containers/{id}/unpause` | `ContainerUnpause` | Full support |
| `POST /containers/prune` | `ContainersPrune` | Returns deleted IDs + space reclaimed |

## Exec

All direct passthroughs:

| Operation | Docker SDK Method | Notes |
|-----------|-------------------|-------|
| `POST /containers/{id}/exec` | `ContainerExecCreate` | Full ExecOptions: AttachStdin/Out/Err, Tty, Cmd, Env, WorkingDir, User, Privileged |
| `GET /exec/{id}` | `ContainerExecInspect` | Returns running state, exit code, PID |
| `POST /exec/{id}/start` | `ContainerExecAttach` | HTTP hijack, multiplexed stream, Detach/Tty support |

## Images

| Operation | Docker SDK Method | Notes |
|-----------|-------------------|-------|
| `POST /images/create` (pull) | `ImagePull` | Streams real pull progress |
| `GET /images/inspect` | `ImageInspectWithRaw` | Full image metadata |
| `POST /images/load` | `ImageLoad` | Real tar loading, streams response |
| `POST /images/tag` | `ImageTag` | `name`, `repo`, `tag` query params |
| `GET /images` (list) | `ImageList` | Supports `all` query param |
| `DELETE /images/{name}` | `ImageRemove` | Supports `force`, `noprune` |
| `GET /images/{name}/history` | `ImageHistory` | Full layer history |
| `POST /images/prune` | `ImagesPrune` | Returns deleted + space reclaimed |

## Auth

| Operation | Docker SDK Method | Notes |
|-----------|-------------------|-------|
| `POST /auth` | `RegistryLogin` | Returns status + identity token |

## Networks

| Operation | Docker SDK Method | Notes |
|-----------|-------------------|-------|
| `POST /networks` | `NetworkCreate` | Full options: Driver, Internal, Attachable, IPAM |
| `GET /networks` | `NetworkList` | All networks |
| `GET /networks/{id}` | `NetworkInspect` | Full network state |
| `POST /networks/{id}/connect` | `NetworkConnect` | Optional EndpointConfig |
| `POST /networks/{id}/disconnect` | `NetworkDisconnect` | Supports Force |
| `DELETE /networks/{id}` | `NetworkRemove` | Direct removal |
| `POST /networks/prune` | `NetworksPrune` | Returns deleted network names |

## Volumes

| Operation | Docker SDK Method | Notes |
|-----------|-------------------|-------|
| `POST /volumes` | `VolumeCreate` | Name, Driver, DriverOpts, Labels |
| `GET /volumes` | `VolumeList` | All volumes |
| `GET /volumes/{name}` | `VolumeInspect` | Full volume state |
| `DELETE /volumes/{name}` | `VolumeRemove` | Supports `force` |
| `POST /volumes/prune` | `VolumesPrune` | Returns deleted + space reclaimed |

## System

| Operation | Docker SDK Method | Notes |
|-----------|-------------------|-------|
| `GET /events` | `Events` | Real-time event streaming |
| `GET /system/df` | `DiskUsage` | Real disk usage with images, containers, volumes |

## CLI Command Mapping

| `docker` CLI command | Docker Backend behavior |
|---------------------|------------------------|
| `docker create <image>` | `ContainerCreate` (auto-pulls if needed) |
| `docker start <id>` | `ContainerStart` |
| `docker stop <id>` | `ContainerStop` with timeout |
| `docker kill <id>` | `ContainerKill` with signal |
| `docker rm <id>` | `ContainerRemove` with force/volume options |
| `docker logs <id>` | `ContainerLogs` with full option support |
| `docker exec <id> <cmd>` | `ContainerExecCreate` + `ContainerExecAttach` |
| `docker cp <src> <id>:<dst>` | **Not implemented** (no archive endpoints) |
| `docker pull <image>` | `ImagePull` (real download) |
| `docker build .` | **Not implemented** (no build endpoint) |
| `docker load` | `ImageLoad` (real tar loading) |
| `docker tag` | `ImageTag` |
| `docker network create` | `NetworkCreate` |
| `docker volume create` | `VolumeCreate` |
| `docker pause <id>` | `ContainerPause` |
| `docker unpause <id>` | `ContainerUnpause` |
| `docker stats <id>` | `ContainerStatsOneShot` or streaming |
| `docker system df` | `DiskUsage` |

## What's Not Implemented

| Feature | Reason |
|---------|--------|
| `docker cp` (archive put/head/get) | Archive endpoints not registered; would need Docker SDK `CopyToContainer`/`CopyFromContainer` |
| `docker build` | Build endpoint not registered |
| Health checks | Not explicitly handled (Docker daemon manages natively) |
| Container archive endpoints | Not part of this backend's route set |

## Differences from Vanilla Docker

The Docker backend is almost identical to vanilla Docker with these minor differences:

1. **Auto-pull on create** — if an image doesn't exist locally when `docker create` is called, the backend automatically pulls it. Vanilla Docker requires an explicit `docker pull` first (or `--pull=always`).
2. **API translation layer** — requests go through Sockerless internal API format before being translated to Docker SDK calls. Some fields may not be mapped (e.g., advanced HostConfig options beyond NetworkMode, Binds, AutoRemove).
3. **No archive/build endpoints** — `docker cp` and `docker build` are not available through this backend.
