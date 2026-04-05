# API Surface Specification

The `api.Backend` interface defines 65 methods. Every backend implements this interface — `BaseServer` provides defaults, cloud backends override subsets, Docker overrides all.

Source: `api/backend_gen.go` (generated from `api/gen/main.go` + `api/openapi.yaml`)

## System (2)

| Method | Signature | Description |
|--------|-----------|-------------|
| `Info` | `() (*BackendInfo, error)` | Backend metadata (driver, OS, containers, images) |
| `SystemDf` | `() (*DiskUsageResponse, error)` | Disk usage for images, containers, volumes, build cache |

## Container Lifecycle (11)

| Method | Signature | Description |
|--------|-----------|-------------|
| `ContainerCreate` | `(req *ContainerCreateRequest) (*ContainerCreateResponse, error)` | Create container from image + config |
| `ContainerStart` | `(id string) error` | Start a created container |
| `ContainerStop` | `(id string, timeout *int) error` | Graceful stop (SIGTERM + timeout + SIGKILL) |
| `ContainerKill` | `(id string, signal string) error` | Send signal to container |
| `ContainerRemove` | `(id string, force bool) error` | Remove container (force stops if running) |
| `ContainerRestart` | `(id string, timeout *int) error` | Stop + Start |
| `ContainerPause` | `(id string) error` | Pause (freeze) container |
| `ContainerUnpause` | `(id string) error` | Resume paused container |
| `ContainerWait` | `(id string, condition string) (*ContainerWaitResponse, error)` | Block until container reaches condition |
| `ContainerPrune` | `(filters map[string][]string) (*ContainerPruneResponse, error)` | Remove all stopped containers |
| `ContainerRename` | `(id string, newName string) error` | Rename container |

## Container Inspection (5)

| Method | Signature | Description |
|--------|-----------|-------------|
| `ContainerInspect` | `(id string) (*Container, error)` | Full container details |
| `ContainerList` | `(opts *ContainerListOptions) ([]*ContainerSummary, error)` | List containers with filters |
| `ContainerTop` | `(id string, psArgs string) (*ContainerTopResponse, error)` | Process list inside container |
| `ContainerStats` | `(id string, stream bool) (io.ReadCloser, error)` | CPU/memory/IO metrics |
| `ContainerChanges` | `(id string) ([]ContainerChangeItem, error)` | Filesystem diff since creation |

## Container I/O (3)

| Method | Signature | Description |
|--------|-----------|-------------|
| `ContainerLogs` | `(id string, opts ContainerLogOptions) (io.ReadCloser, error)` | Stdout/stderr stream |
| `ContainerAttach` | `(id string, opts ContainerAttachOptions) (io.ReadWriteCloser, error)` | Bidirectional attach |
| `ContainerExport` | `(id string) (io.ReadCloser, error)` | Export filesystem as tar |

## Container Configuration (2)

| Method | Signature | Description |
|--------|-----------|-------------|
| `ContainerUpdate` | `(id string, req *ContainerUpdateRequest) (*ContainerUpdateResponse, error)` | Update resource limits |
| `ContainerResize` | `(id string, h int, w int) error` | Resize TTY |

## Container Archive (3)

| Method | Signature | Description |
|--------|-----------|-------------|
| `ContainerPutArchive` | `(id string, path string, noOverwriteDirNonDir bool, body io.Reader) error` | Extract tar into container |
| `ContainerStatPath` | `(id string, path string) (*ContainerPathStat, error)` | Stat path inside container |
| `ContainerGetArchive` | `(id string, path string) (*ContainerArchiveResponse, error)` | Tar archive of path |

## Exec (4)

| Method | Signature | Description |
|--------|-----------|-------------|
| `ExecCreate` | `(containerID string, req *ExecCreateRequest) (*ExecCreateResponse, error)` | Create exec instance |
| `ExecStart` | `(id string, opts ExecStartRequest) (io.ReadWriteCloser, error)` | Start exec (streams I/O) |
| `ExecInspect` | `(id string) (*ExecInstance, error)` | Inspect exec instance |
| `ExecResize` | `(id string, h int, w int) error` | Resize exec TTY |

## Image Lifecycle (6)

| Method | Signature | Description |
|--------|-----------|-------------|
| `ImagePull` | `(ref string, auth string) (io.ReadCloser, error)` | Pull image from registry |
| `ImageLoad` | `(r io.Reader) (io.ReadCloser, error)` | Load image from tar |
| `ImageTag` | `(source string, repo string, tag string) error` | Tag an image |
| `ImageRemove` | `(name string, force bool, prune bool) ([]*ImageDeleteResponse, error)` | Remove image |
| `ImagePrune` | `(filters map[string][]string) (*ImagePruneResponse, error)` | Remove dangling images |
| `ImageBuild` | `(opts ImageBuildOptions, context io.Reader) (io.ReadCloser, error)` | Build from Dockerfile |

## Image Operations (6)

| Method | Signature | Description |
|--------|-----------|-------------|
| `ImageInspect` | `(name string) (*Image, error)` | Full image details |
| `ImageList` | `(opts *ImageListOptions) ([]*ImageSummary, error)` | List images with filters |
| `ImageHistory` | `(name string) ([]*ImageHistoryEntry, error)` | Layer history |
| `ImagePush` | `(name string, tag string, auth string) (io.ReadCloser, error)` | Push to registry |
| `ImageSave` | `(names []string) (io.ReadCloser, error)` | Export as tar |
| `ImageSearch` | `(term string, limit int, filters map[string][]string) ([]*ImageSearchResult, error)` | Search registries |

## Container Commit (1)

| Method | Signature | Description |
|--------|-----------|-------------|
| `ContainerCommit` | `(req *ContainerCommitRequest) (*ContainerCommitResponse, error)` | Create image from container |

## Authentication (1)

| Method | Signature | Description |
|--------|-----------|-------------|
| `AuthLogin` | `(req *AuthLoginRequest) (*AuthResponse, error)` | Registry login |

## Network (7)

| Method | Signature | Description |
|--------|-----------|-------------|
| `NetworkCreate` | `(req *NetworkCreateRequest) (*NetworkCreateResponse, error)` | Create network |
| `NetworkList` | `(filters map[string][]string) ([]*Network, error)` | List networks |
| `NetworkInspect` | `(id string) (*Network, error)` | Inspect network |
| `NetworkConnect` | `(id string, req *NetworkConnectRequest) error` | Connect container to network |
| `NetworkDisconnect` | `(id string, req *NetworkDisconnectRequest) error` | Disconnect from network |
| `NetworkRemove` | `(id string) error` | Remove network |
| `NetworkPrune` | `(filters map[string][]string) (*NetworkPruneResponse, error)` | Remove unused networks |

## Volume (5)

| Method | Signature | Description |
|--------|-----------|-------------|
| `VolumeCreate` | `(req *VolumeCreateRequest) (*Volume, error)` | Create volume |
| `VolumeList` | `(filters map[string][]string) (*VolumeListResponse, error)` | List volumes |
| `VolumeInspect` | `(name string) (*Volume, error)` | Inspect volume |
| `VolumeRemove` | `(name string, force bool) error` | Remove volume |
| `VolumePrune` | `(filters map[string][]string) (*VolumePruneResponse, error)` | Remove unused volumes |

## Pod (8)

Podman-compatible pod operations via the Libpod API (`/libpod/pods/*`).

| Method | Signature | Description |
|--------|-----------|-------------|
| `PodCreate` | `(req *PodCreateRequest) (*PodCreateResponse, error)` | Create pod |
| `PodList` | `(opts *PodListOptions) ([]*PodListEntry, error)` | List pods |
| `PodInspect` | `(name string) (*PodInspectResponse, error)` | Inspect pod |
| `PodExists` | `(name string) (bool, error)` | Check pod exists |
| `PodStart` | `(name string) (*PodActionResponse, error)` | Start pod (launches multi-container task) |
| `PodStop` | `(name string, timeout *int) (*PodActionResponse, error)` | Stop pod |
| `PodKill` | `(name string, signal string) (*PodActionResponse, error)` | Kill pod |
| `PodRemove` | `(name string, force bool) error` | Remove pod |

## HTTP Route Registration

Source: `backends/core/handle_docker_api.go:registerDockerAPIRoutes()`

Docker-compatible routes registered on the mux with API version prefix stripping (handles `/v1.44/containers/json`, etc.). Libpod routes registered in `handle_libpod.go:registerLibpodRoutes()`.
