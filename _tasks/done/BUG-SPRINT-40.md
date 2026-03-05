# Bug Sprint 40 — BUG-502 → BUG-514

**Status:** Complete
**Bugs fixed:** 13

## Summary

Fixed 13 bugs across frontend, core, and Docker backend:

### Frontend fixes
- **BUG-502**: `handleContainerAttach` — inspect container TTY setting, use `raw-stream` content-type for TTY containers
- **BUG-506**: `handleContainerCreate` — forward `platform` query param to backend
- **BUG-507**: `handleContainerRemove` — forward `link` query param to backend
- **BUG-514**: `handleContainerStart` — forward `detachKeys` query param to backend

### Core fixes
- **BUG-503**: `handleExecCreate` — validate empty `Cmd`, return 400 "No exec command specified"
- **BUG-504**: `handleContainerTop` — read `ps_args` query param for API parity
- **BUG-505**: `handleContainerStop` — accept `signal` query param, apply signal-based exit code via `signalToExitCode()`
- **BUG-513**: `handleImageSearch` — sort results by relevance (exact match first, then alphabetical)

### Docker backend fixes
- **BUG-508**: Register `POST /internal/v1/images/{name}/push` — proxy to Docker SDK `ImagePush`
- **BUG-509**: Register `GET /internal/v1/images/get` and `GET /internal/v1/images/{name}/get` — proxy to Docker SDK `ImageSave`
- **BUG-510**: Register `GET /internal/v1/images/search` — proxy to Docker SDK `ImageSearch`
- **BUG-511**: Register `POST /internal/v1/images/build` — proxy to Docker SDK `ImageBuild`
- **BUG-512**: Register `PUT/HEAD/GET /internal/v1/containers/{id}/archive` — proxy to Docker SDK `CopyToContainer`, `ContainerStatPath`, `CopyFromContainer`

## Files Modified
- `frontends/docker/containers_stream.go` — BUG-502
- `frontends/docker/containers.go` — BUG-506, BUG-507, BUG-514
- `backends/core/handle_exec.go` — BUG-503
- `backends/core/handle_extended.go` — BUG-504
- `backends/core/handle_containers.go` — BUG-505
- `backends/core/handle_images.go` — BUG-513
- `backends/docker/server.go` — BUG-508, BUG-509, BUG-510, BUG-511, BUG-512
- `backends/docker/extended.go` — BUG-508, BUG-509, BUG-510, BUG-511, BUG-512

## Verification
- `backends/core`: 302 PASS (go test -race)
- `frontends/docker`: 7 PASS (go test -race)
- All 8 backends: build clean with `-tags noui`
