# Bug Sprint 14 — API & Backends Audit (BUG-099→106)

**Completed**: 2026-03-04

## Summary

Fixed 8 bugs found in Sprint 14 audit of `api/` and all `backends/`:

| Bug | Severity | Component | Fix |
|-----|----------|-----------|-----|
| BUG-099 | High | lambda, gcf, azf containers.go | Added `StopContainer(id, 0)` in FaaS stop handlers |
| BUG-100 | High | ecs extended.go, server.go | Added `handleContainerRestart` override matching CloudRun/ACA pattern |
| BUG-101 | Medium | core handle_exec.go | Added `Privileged: &req.Privileged` to ExecProcessConfig |
| BUG-102 | Medium | docker containers.go | Added `Since`/`Until` to ContainerLogs LogsOptions |
| BUG-103 | Medium | docker extended.go | Added `IPPrefixLen` to NetworkConnect EndpointSettings |
| BUG-104 | Medium | docker volumes.go | Added `Status: vol.Status` to all 3 volume handlers |
| BUG-105 | Medium | cloudrun, aca extended.go | Replaced no-op VolumePrune with ECS's mount-usage-check logic |
| BUG-106 | Medium | docker containers.go | Added `limit`/`filters` query param parsing to ContainerList |

## Verification

- All 8 modules build clean
- 286 core tests PASS (`go test -race -count=1 ./...`)
- 0 lint issues across 19 modules (`make lint`)
