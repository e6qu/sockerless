# Bug Sprint 30 — BUG-359 → BUG-377

**Date**: 2026-03-05
**Bugs fixed**: 18

## Summary

Remaining lifecycle gaps in cloud backends (StopHealthCheck, create event, signalToExitCode, force-remove events, Network.Disconnect) and core (prune events, pod lifecycle, FormatStatus, Event.Scope, restart event).

## Bugs Fixed

| ID | Component | Fix |
|----|-----------|-----|
| BUG-359 | All 6 cloud | Added `StopHealthCheck(id)` in `handleContainerStop` |
| BUG-360 | All 6 cloud | Added `StopHealthCheck(id)` in `handleContainerKill` |
| BUG-361 | All 6 cloud | Added `StopHealthCheck(id)` in `handleContainerRemove` |
| BUG-362 | All 6 cloud | Added "create" event in `handleContainerCreate` |
| BUG-363 | All 6 cloud | Added full `signalToExitCode` with 8 signals (24 aliases) |
| BUG-364 | All 6 cloud | Added "kill"+"die" events in force-remove path |
| BUG-365 | All 6 cloud | Added `Network.Disconnect` loop in `handleContainerRemove` |
| BUG-366 | All 6 cloud | Added `Network.Disconnect` loop + `StopHealthCheck` in `handleContainerPrune` |
| BUG-367 | Core | Added `BuildContexts` cleanup in `handleImagePrune` |
| BUG-368 | Core | Added untag/delete events in `handleImagePrune` |
| BUG-369 | Core | Added destroy events in `handleVolumePrune` |
| BUG-370 | Core | Added destroy events in `handleNetworkPrune` |
| BUG-371 | Core | Added `ProcessLifecycle.Cleanup(cid)` in `handlePodKill` |
| BUG-372 | Core | Added "destroy" events per container in `handlePodRemove` force path |
| BUG-373 | Core | Added `FinishedAt`/`ExitCode` reset in `handlePodStart` |
| BUG-374 | Core | Replaced hardcoded FormatStatus with computed uptime |
| BUG-375 | Core | Added `Scope: "local"` to Event construction in `emitEvent` |
| BUG-376 | Core + All 6 cloud | Added "restart" event in `handleContainerRestart` |

## Files Modified

- `backends/ecs/containers.go` — BUG-359, 360, 361, 362, 363, 364, 365
- `backends/ecs/extended.go` — BUG-366, 376
- `backends/lambda/containers.go` — BUG-359, 360, 361, 362, 363, 364, 365
- `backends/lambda/extended.go` — BUG-366, 376
- `backends/cloudrun/containers.go` — BUG-359, 360, 361, 362, 363, 364, 365
- `backends/cloudrun/extended.go` — BUG-366, 376
- `backends/cloudrun-functions/containers.go` — BUG-359, 360, 361, 362, 363, 364, 365
- `backends/cloudrun-functions/extended.go` — BUG-366, 376
- `backends/aca/containers.go` — BUG-359, 360, 361, 362, 363, 364, 365
- `backends/aca/extended.go` — BUG-366, 376
- `backends/azure-functions/containers.go` — BUG-359, 360, 361, 362, 363, 364, 365
- `backends/azure-functions/extended.go` — BUG-366, 376
- `backends/core/handle_images.go` — BUG-367, 368
- `backends/core/handle_volumes.go` — BUG-369
- `backends/core/handle_networks.go` — BUG-370
- `backends/core/handle_pods.go` — BUG-371, 372, 373
- `backends/core/filters.go` — BUG-374
- `backends/core/event_bus.go` — BUG-375
- `backends/core/handle_containers.go` — BUG-376

## Verification

- `cd backends/core && go test -race -count=1 ./...` — 302 PASS
- All 8 backends: `go build -tags noui ./...` — PASS
- `cd frontends/docker && go build -tags noui ./...` — PASS
