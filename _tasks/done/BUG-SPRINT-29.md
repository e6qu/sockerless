# Bug Sprint 29 — BUG-345 → BUG-358

**Date**: 2026-03-05
**Bugs fixed**: 14

## Summary

Systematic event emission gaps and cleanup inconsistencies across all 6 cloud backends and core pod handlers.

## Bugs Fixed

| ID | Component | Fix |
|----|-----------|-----|
| BUG-345 | ECS | Added "stop" event after "die" in `handleContainerRestart` |
| BUG-346 | Lambda | Added "stop" event after "die" in `handleContainerRestart` |
| BUG-347 | CloudRun | Added "stop" event after "die" in `handleContainerRestart` |
| BUG-348 | GCF | Added "stop" event after "die" in `handleContainerRestart` |
| BUG-349 | ACA | Added "stop" event after "die" in `handleContainerRestart` |
| BUG-350 | AZF | Added "stop" event after "die" in `handleContainerRestart` |
| BUG-351 | All 6 cloud | Added `RestartCount++` via `Store.Containers.Update` in restart handlers |
| BUG-352 | All 6 cloud | Added "destroy" event at end of `handleContainerRemove` |
| BUG-353 | All 6 cloud | Added "destroy" event in `handleContainerPrune` loop |
| BUG-354 | Core | Added "die" + "stop" events per container in `handlePodStop` |
| BUG-355 | Core | Added "kill" + "die" events per container in `handlePodKill` |
| BUG-356 | All 6 cloud | Added `Pods.RemoveContainer` in `handleContainerRemove` |
| BUG-357 | All 6 cloud | Added `Pods.RemoveContainer` in `handleContainerPrune` |
| BUG-358 | Core | Added non-force cleanup in `handlePodRemove` for exited containers |

## Files Modified

- `backends/ecs/extended.go` — BUG-345, 351, 353, 357
- `backends/ecs/containers.go` — BUG-352, 356
- `backends/lambda/extended.go` — BUG-346, 351, 353, 357
- `backends/lambda/containers.go` — BUG-352, 356
- `backends/cloudrun/extended.go` — BUG-347, 351, 353, 357
- `backends/cloudrun/containers.go` — BUG-352, 356
- `backends/cloudrun-functions/extended.go` — BUG-348, 351, 353, 357
- `backends/cloudrun-functions/containers.go` — BUG-352, 356
- `backends/aca/extended.go` — BUG-349, 351, 353, 357
- `backends/aca/containers.go` — BUG-352, 356
- `backends/azure-functions/extended.go` — BUG-350, 351, 353, 357
- `backends/azure-functions/containers.go` — BUG-352, 356
- `backends/core/handle_pods.go` — BUG-354, 355, 358

## Verification

- `cd backends/core && go test -race -count=1 ./...` — 302 PASS
- All 8 backends: `go build -tags noui ./...` — PASS
- `cd frontends/docker && go build -tags noui ./...` — PASS
