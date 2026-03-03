# Bug Sprint 10 — API & Backends Audit (BUG-069→074)

**Date**: 2026-03-04
**PR**: Sprint 10

## Bugs Fixed

### BUG-069: `handleImageRemove` doesn't delete tag aliases from store
- **File**: `backends/core/handle_images.go`
- **Fix**: Delete all RepoTags and name-without-tag aliases after deleting by ID

### BUG-070: ECS `handleContainerPrune` doesn't clean up cloud resources
- **File**: `backends/ecs/extended.go`
- **Fix**: Added task definition deregistration and `MarkCleanedUp` to prune loop

### BUG-071: FaaS `handleContainerKill` doesn't update container state
- **Files**: `backends/lambda/containers.go`, `backends/cloudrun-functions/containers.go`, `backends/azure-functions/containers.go`
- **Fix**: Added signal parsing, state transition to "exited", and WaitChs close

### BUG-072: FaaS `handleContainerPrune` doesn't clean up cloud resources
- **Files**: `backends/lambda/extended.go`, `backends/cloudrun-functions/extended.go`, `backends/azure-functions/extended.go`
- **Fix**: Added cloud function deletion and resource registry cleanup

### BUG-073: FaaS prune and remove don't clean up LogBuffers
- **Files**: 6 locations across 3 FaaS backends (prune + remove handlers)
- **Fix**: Added `LogBuffers.Delete(c.ID)` to all 6 locations

### BUG-074: Docker backend `mapContainerFromDocker` doesn't populate Mounts
- **File**: `backends/docker/containers.go`
- **Fix**: Map `info.Mounts` to `api.MountPoint` slice

## Verification

- `go build ./...` — all 6 modified backends compile
- `go test -race -count=1 ./...` (backends/core) — 286 PASS
- `make lint` — 0 issues across 19 modules
- `make sim-test-all` — 75 PASS
