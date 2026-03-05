# Bug Sprint 31 — BUG-378 → BUG-394

**Date:** 2026-03-05
**Bugs fixed:** 17
**Total bugs fixed:** 368 (31 sprints)

## Bugs Fixed

| Bug | Sev | Component | Fix |
|-----|-----|-----------|-----|
| BUG-378 | High | Core | `handlePodStart` now calls `ProcessLifecycle.Start()` |
| BUG-379 | High | Core | `handlePodStart` now calls `StartHealthCheck()` |
| BUG-380 | Med | Core | `handlePodRemove` non-force: added `StopHealthCheck(cid)` |
| BUG-381 | Med | Core | `handlePodRemove` non-force: added `ProcessLifecycle.Cleanup(cid)` |
| BUG-382 | Med | Core | `handlePodRemove` non-force: added "destroy" events |
| BUG-383 | Med | Core | `handlePodRemove` non-force: added `Network.Disconnect` loop |
| BUG-384 | Med | Core | `handleContainerWait` now reads `condition` param (not-running/next-exit/removed) |
| BUG-385 | Med | Core | `MatchContainerFilters` added `exited` filter |
| BUG-386 | Med | All 6 cloud | `handleContainerCreate` added `?pod=` validation + `Pods.AddContainer()` |
| BUG-387 | Med | All 6 cloud | `handleContainerRestart` uses httptest.ResponseRecorder, only emits on success |
| BUG-388 | Med | Frontend | `handlePodKill` forwards `signal` query param |
| BUG-389 | Med | Frontend | `handlePodStop` forwards `t` query param |
| BUG-390 | Low | Core | `handleContainerLogs` handles `details` param (prepends labels) |
| BUG-391 | Low | Core | `MatchContainerFilters` added `publish` filter |
| BUG-392 | Low | Core | `MatchContainerFilters` added `volume` filter |
| BUG-393 | Low | Core | `MatchContainerFilters` added `is-task` filter |
| BUG-394 | Low | Core | `handleContainerList` handles `size` param (populates SizeRootFs) |

## Files Modified

- `backends/core/handle_pods.go` (BUG-378→383)
- `backends/core/handle_containers.go` (BUG-384)
- `backends/core/handle_containers_query.go` (BUG-390, 394)
- `backends/core/filters.go` (BUG-385, 391→393)
- `backends/{ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}/containers.go` (BUG-386)
- `backends/{ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}/extended.go` (BUG-387)
- `frontends/docker/pods.go` (BUG-388, 389)

## Verification

- `cd backends/core && go test -race -count=1 ./...` — 302 PASS
- All 8 backends build with `-tags noui`
- Frontend builds with `-tags noui`
