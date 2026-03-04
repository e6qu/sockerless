# Bug Sprint 16 — BUG-115 through BUG-122

**Date**: 2026-03-04
**Scope**: api/, backends/core/, backends/docker/, all 6 cloud backends

## Bugs Fixed (8)

| Bug | Severity | Summary |
|-----|----------|---------|
| BUG-115 | High | `extractTar` path traversal — files could be written outside destination |
| BUG-116 | Medium | Container prune missing network disconnect cleanup |
| BUG-117 | Medium | Container restart not restarting health checks |
| BUG-118 | High | 6 cloud stop handlers missing AgentRegistry.Remove + using StopContainer |
| BUG-119 | High | 3 cloud restart handlers missing AgentRegistry.Remove + using StopContainer |
| BUG-120 | High | Docker system events drops since/until/filters params |
| BUG-121 | High | Docker system df drops container SizeRw/SizeRootFs |
| BUG-122 | Medium | 6 cloud backends' remove/prune missing StagingDirs/Execs cleanup |

## Files Modified (17)

- `api/types.go` — added `SizeRootFs` field to `ContainerSummary`
- `backends/core/handle_containers_archive.go` — path traversal fix
- `backends/core/handle_extended.go` — prune network disconnect
- `backends/core/handle_containers.go` — restart health check
- `backends/ecs/containers.go` — stop agent+force, remove cleanup
- `backends/ecs/extended.go` — restart agent+force, prune cleanup
- `backends/cloudrun/containers.go` — stop agent+force, remove cleanup
- `backends/cloudrun/extended.go` — restart agent+force, prune cleanup
- `backends/aca/containers.go` — stop agent+force, remove cleanup
- `backends/aca/extended.go` — restart agent+force, prune cleanup
- `backends/lambda/containers.go` — stop agent+force, remove cleanup
- `backends/lambda/extended.go` — prune cleanup
- `backends/cloudrun-functions/containers.go` — stop agent+force, remove cleanup
- `backends/cloudrun-functions/extended.go` — prune cleanup
- `backends/azure-functions/containers.go` — stop agent+force, remove cleanup
- `backends/azure-functions/extended.go` — prune cleanup
- `backends/docker/extended.go` — events filters, df sizes

## Verification

- `cd backends/core && go test -race -count=1 ./...` — 286 PASS
- All 8 backends compile clean
- `make lint` — 0 issues across 19 modules
