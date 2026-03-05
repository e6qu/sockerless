# Bug Sprint 44 — BUG-554 → BUG-574

**Date:** 2026-03-05
**Bugs Fixed:** 21
**Total Fixed:** 574 (BUG-001 → BUG-574)

## Summary

Docker backend inspect/list field mapping, cloud restart state management, KQL datetime syntax, and frontend route gaps.

## Bugs

| ID | Sev | Component | Fix |
|----|-----|-----------|-----|
| BUG-554 | Med | Docker | `handleContainerLogs` — inspect container for TTY mode, set Content-Type accordingly |
| BUG-555 | Med | Docker | `mapContainerFromDocker` — map all top-level NetworkSettings scalars (Bridge, SandboxID, HairpinMode, etc.) |
| BUG-556 | Med | Docker | Container inspect EndpointSettings — add IPv6Gateway, GlobalIPv6Address, GlobalIPv6PrefixLen, IPAMConfig, DriverOpts |
| BUG-557 | Med | Docker | Container list EndpointSettings — same missing fields as BUG-556 |
| BUG-558 | Med | Docker | ContainerSummary — set HostConfig with NetworkMode |
| BUG-559 | High | Docker | `handleImagePush` — read auth from X-Registry-Auth header, not query param |
| BUG-560 | Med | Docker | `handleContainerCommit` — use `r.URL.Query()["changes"]` for multi-value params |
| BUG-561 | Low | Docker | Container/exec resize — return 204 No Content instead of 200 |
| BUG-562 | Med | Docker | `handleExecInspect` — extract OpenStdin/OpenStdout/OpenStderr/DetachKeys from raw JSON |
| BUG-563 | Med | Docker | Network create/inspect — map IPAM AuxiliaryAddresses ↔ AuxAddress |
| BUG-564 | Med | Docker | Image inspect healthcheck — add StartInterval field |
| BUG-565 | Low | Docker | NetworkSettings.Ports — initialize to empty map instead of nil |
| BUG-566 | Med | ECS | `handleContainerRestart` — clear TaskDefARN/TaskARN/ClusterARN after deregister |
| BUG-567 | Med | CloudRun | `handleContainerRestart` — clear JobName/ExecutionName after job delete |
| BUG-568 | High | AZF+Core | KQL datetime — add quotes: `datetime("%s")` (3 locations) |
| BUG-569 | Low | ACA | `waitForExecutionComplete`/`pollExecutionExit` — guard `executionName != ""` |
| BUG-570 | Low | Frontend | `handleContainerList` — forward before/since query params |
| BUG-571 | Low | Frontend | `handleContainerRestart` — forward signal query param |
| BUG-572 | Low | Frontend | Add `POST /_ping` route |
| BUG-573 | Low | Frontend | Add `POST /build/prune` route with stub response |
| BUG-574 | Low | Docker | `handleExecStart` — guard stdin copy goroutine with OpenStdin check |

## Files Modified

| File | Bugs |
|------|------|
| `backends/docker/containers.go` | 554, 555, 556, 557, 558, 564, 565 |
| `backends/docker/extended.go` | 559, 560, 561 |
| `backends/docker/exec.go` | 562, 574 |
| `backends/docker/networks.go` | 563 |
| `backends/docker/images.go` | 564 |
| `backends/ecs/extended.go` | 566 |
| `backends/cloudrun/extended.go` | 567 |
| `backends/azure-functions/logs.go` | 568 |
| `backends/core/log_cloud.go` | 568 |
| `backends/aca/containers.go` | 569 |
| `frontends/docker/containers.go` | 570, 571 |
| `frontends/docker/server.go` | 572, 573 |
| `frontends/docker/images.go` | 573 |
| `FEATURE_MATRIX.md` | Updated |
| `BUGS.md` | Sprint 44 added |
| `STATUS.md` | Bug count updated |
| `WHAT_WE_DID.md` | Sprint 44 summary added |

## Verification

- All 6 modified modules compile: `go build ./...` ✅
- Core tests: 302+ PASS ✅
- Frontend tests: 7 PASS ✅
- Lint (19 modules): 0 issues ✅
