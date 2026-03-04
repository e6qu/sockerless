# Bug Sprint 18 — BUG-131→138

**Date**: 2026-03-04
**Bugs fixed**: 8 (BUG-131→138)
**Open bugs added**: 1 (OB-031)
**Net open bugs**: 23 (was 30, fixed 8, added 1)

## Bugs Fixed

| Bug | File | Fix |
|-----|------|-----|
| BUG-131 | `core/handle_containers.go` | Restart now calls `StopHealthCheck(id)` before stopping process |
| BUG-132 | `core/handle_containers.go` | Restart emits `die` event after stop and `start` event after re-start |
| BUG-133 | `core/handle_containers.go` | Re-fetch container after state update so env/binds reflect current state |
| BUG-134 | `core/handle_containers_query.go` | Container list uses `ResolveImage()` for real ImageID instead of `GenerateID()` |
| BUG-135 | `core/handle_images.go` | Image remove deletes docker.io/library/ and docker.io/ short aliases |
| BUG-136 | `ecs/containers.go`, `cloudrun/containers.go`, `aca/containers.go` | Error paths after `AgentRegistry.Prepare()` now call `AgentRegistry.Remove()` |
| BUG-137 | `lambda/extended.go`, `cloudrun-functions/extended.go`, `azure-functions/extended.go` | FaaS restart now stops + re-dispatches to start (was no-op for running containers) |
| BUG-138 | `docker/containers.go` | Container list forwards `before`, `since`, and `size` query params to Docker SDK |

## Open Bug Added

| Bug | File | Issue |
|-----|------|-------|
| OB-031 | `docker/containers.go:109-122` | Docker mount mapping drops VolumeOptions and TmpfsOptions |

## Open Bugs Closed

OB-007, OB-008, OB-009, OB-011, OB-012, OB-017, OB-019, OB-021

## Verification

- `cd backends/core && go test -race -count=1 ./...` — 302 PASS
- All 8 backend modules build clean
- `make lint` — 0 issues
