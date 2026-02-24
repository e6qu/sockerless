# api

Shared type definitions and interfaces for the Sockerless project. This module defines the contract between frontends, backends, and drivers — with zero external dependencies.

## Overview

All Sockerless components import this module for common types. It contains no logic, only struct definitions, interface declarations, and typed errors. Every type uses JSON struct tags for REST API serialization.

## Interfaces

| Interface | File | Purpose |
|-----------|------|---------|
| `Backend` | `backend.go` | ~40 methods covering containers, images, exec, networks, volumes, and system |
| `VolumeDriver` | `drivers.go` | Pluggable volume storage (create, inspect, list, remove, mount spec) |
| `NetworkDriver` | `drivers.go` | Container networking (create, inspect, connect, disconnect, prune) |
| `LogDriver` | `drivers.go` | Log storage (write, read) |
| `StatusCoder` | `errors.go` | Implemented by errors that carry an HTTP status code |

## Error types

All implement `error` and `StatusCoder`:

| Type | HTTP Status | Fields |
|------|-------------|--------|
| `NotFoundError` | 404 | Resource, ID |
| `ConflictError` | 409 | Message |
| `InvalidParameterError` | 400 | Message |
| `NotImplementedError` | 501 | Message |
| `NotModifiedError` | 304 | — |
| `ServerError` | 500 | Message |

`ErrorResponse` wraps a `Message` field for JSON error bodies.

## Type categories

### Containers
`Container`, `ContainerState`, `ContainerConfig`, `HostConfig`, `ContainerSummary`, `ContainerCreateRequest`/`Response`, `ContainerListOptions`, `ContainerLogsOptions`, `ContainerWaitResponse`, `ContainerAttachOptions`, `ContainerTopResponse`, `ContainerPruneResponse`, `ContainerUpdateRequest`/`Response`, `ContainerChangeItem`, `ContainerCommitResponse`

### Exec
`ExecInstance`, `ExecProcessConfig`, `ExecCreateRequest`/`Response`, `ExecStartRequest`

### Images
`Image`, `ImageSummary`, `ImagePullRequest`, `ImageDeleteResponse`, `ImageHistoryEntry`, `ImagePruneResponse`, `ImageListOptions`, `RootFS`, `ImageMetadata`

### Networks
`Network`, `IPAM`, `IPAMConfig`, `EndpointSettings`, `EndpointResource`, `NetworkCreateRequest`/`Response`, `NetworkConnectRequest`, `NetworkDisconnectRequest`, `NetworkPruneResponse`, `NetworkingConfig`

### Volumes
`Volume`, `VolumeCreateRequest`, `VolumeListResponse`, `VolumePruneResponse`

### System
`BackendInfo`, `AuthRequest`/`Response`, `Event`, `EventActor`, `EventsOptions`, `DiskUsageResponse`

### Drivers
`MountSpec` (returned by `VolumeDriver.MountSpec`)

## Project structure

```
api/
├── backend.go    Backend interface (~40 methods)
├── drivers.go    VolumeDriver, NetworkDriver, LogDriver interfaces
├── errors.go     Typed HTTP errors (404, 409, 400, 501, 304, 500)
├── types.go      All request/response structs
└── go.mod        Zero dependencies
```

## Usage

```go
import "github.com/sockerless/api"

func handleCreate(b api.Backend, req api.ContainerCreateRequest) {
    resp, err := b.ContainerCreate(ctx, req)
    if err != nil {
        var nf *api.NotFoundError
        if errors.As(err, &nf) {
            // 404
        }
    }
}
```
