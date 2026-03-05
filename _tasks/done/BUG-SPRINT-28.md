# Bug Sprint 28 — BUG-337 → BUG-344

**Date**: 2026-03-05
**Bugs fixed**: 8

## Bugs

| Bug | Component | Fix |
|-----|-----------|-----|
| BUG-337 | ECS | `handleContainerRemove` — use ClusterARN fallback instead of hardcoded `s.config.Cluster` |
| BUG-338 | ECS | `handleContainerRestart` — use ClusterARN fallback instead of hardcoded `s.config.Cluster` |
| BUG-339 | ECS | `handleContainerRestart` — deregister old task definition (matching remove/prune) |
| BUG-340 | CloudRun | `handleContainerRestart` — call `deleteJob()` before `MarkCleanedUp` (matching remove/prune) |
| BUG-341 | ACA | `handleContainerRestart` — call `deleteJob()` before `MarkCleanedUp` (matching remove/prune) |
| BUG-342 | Core | `handlePodKill` — respect `signal` query parameter via `signalToExitCode()` instead of hardcoded 137 |
| BUG-343 | Core | `handleImageTag` — check for duplicate `RepoTags` before appending |
| BUG-344 | Frontend | `handleContainerStats` — forward `one-shot` query parameter to backend |

## Files Modified

- `backends/ecs/containers.go` — BUG-337
- `backends/ecs/extended.go` — BUG-338, BUG-339
- `backends/cloudrun/extended.go` — BUG-340
- `backends/aca/extended.go` — BUG-341
- `backends/core/handle_pods.go` — BUG-342
- `backends/core/handle_images.go` — BUG-343
- `frontends/docker/containers_stream.go` — BUG-344

## Verification

- `backends/core` tests: 302 PASS
- All 8 backends: build OK (`go build -tags noui ./...`)
- Frontend: build OK (`go build -tags noui ./...`)
