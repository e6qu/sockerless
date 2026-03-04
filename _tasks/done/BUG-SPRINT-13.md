# Bug Sprint 13 — API & Backends Audit (BUG-091→098)

**Status**: Complete
**Date**: 2026-03-04

## Bugs Fixed

| Bug | Severity | Component | Fix |
|-----|----------|-----------|-----|
| BUG-091 | High | docker/containers.go | Map NetworkingConfig to Docker SDK (was nil) |
| BUG-092 | High | ecs,cloudrun,aca | Add LogBuffers.Delete in prune+remove (6 locations) |
| BUG-093 | Medium | cloudrun,aca extended.go | Add Registry.MarkCleanedUp in prune |
| BUG-094 | Medium | docker/containers.go | Map RestartCount, ExecIDs in inspect |
| BUG-095 | Medium | docker/containers.go | Map Platform, LogPath, ResolvConfPath, HostnamePath, HostsPath |
| BUG-096 | Medium | docker/networks.go | Forward IPAM.Options in network create |
| BUG-097 | Medium | docker/images.go | Map Parent, Comment in image inspect |
| BUG-098 | Medium | docker/containers.go | Map Aliases in container list endpoint settings |

## Files Modified

- `backends/docker/containers.go` — BUG-091, BUG-094, BUG-095, BUG-098
- `backends/docker/networks.go` — BUG-096
- `backends/docker/images.go` — BUG-097
- `backends/ecs/extended.go` — BUG-092
- `backends/ecs/containers.go` — BUG-092
- `backends/cloudrun/extended.go` — BUG-092, BUG-093
- `backends/cloudrun/containers.go` — BUG-092
- `backends/aca/extended.go` — BUG-092, BUG-093
- `backends/aca/containers.go` — BUG-092
- `BUGS.md` — BUG-091→098

## Verification

- All 4 modified backends compile: docker, ecs, cloudrun, aca
- 286 core tests PASS (no regressions)
- 0 lint issues across 19 modules
