# Bug Sprint 9 — API & Backends Audit (BUG-063→068)

**Date**: 2026-03-04
**Scope**: `api/` and all 8 `backends/`
**Focus**: Docker-to-cloud translation fidelity — resource lifecycle, state consistency, error handling

## Bugs Fixed

| Bug | Severity | File(s) | Fix |
|-----|----------|---------|-----|
| BUG-063 | Low | `api/types.go` | `ExecProcessConfig.Privileged` → `*bool` with `omitempty` |
| BUG-064 | High | `backends/core/store.go`, `backends/{ecs,aca,cloudrun}/containers.go` | `Store.RevertToCreated()` on all cloud failure paths |
| BUG-065 | Medium | `backends/aca/containers.go` | `s.deleteJob()` on `PollUntilDone` failure (single + multi) |
| BUG-066 | Medium | `backends/cloudrun/containers.go` | `s.deleteJob()` on `createOp.Wait` failure (single + multi) |
| BUG-067 | Medium | `backends/cloudrun-functions/containers.go` | Best-effort `DeleteFunction` on `op.Wait` failure |
| BUG-068 | Medium | `backends/azure-functions/containers.go` | Best-effort `WebApps.Delete` on `PollUntilDone` failure |

## False Positives Eliminated

- `handleImageLoad` io.Copy to Discard — intentional body drain
- `handleContainerWait` type assertion — only `chan struct{}` ever stored
- `forceStop` double-close — `LoadAndDelete` is atomic
- `buildEndpointForNetwork` Update return — concurrent deletion negligible
- Dockerfile VOLUME JSON parsing — intentional fallthrough

## Verification

- All 7 modified modules compile
- 276 core tests PASS (with race detector)
- 75 sim-backend tests PASS
- 0 lint issues across 19 modules
