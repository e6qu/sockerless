# Bug Sprint 33 — BUG-409 → BUG-422

**Status:** Complete
**Date:** 2026-03-05
**Bugs fixed:** 14

## Summary

Core backend parity fixes: resize endpoints, stats response completeness, default network initialization, missing event emissions, space reclaimed calculations, deterministic image IDs, paused container handling, health check exec cleanup, and logs stream filtering.

## Bugs Fixed

| Bug | File | Fix |
|-----|------|-----|
| BUG-409 | handle_extended.go, server.go | Added `handleContainerResize` and `handleExecResize` no-op handlers + route registration |
| BUG-410 | handle_extended.go | Added `precpu_stats` field to `buildStatsEntry` with zero values (matches Docker first-read behavior) |
| BUG-411 | handle_extended.go | `buildStatsEntry` now accepts `memLimit` param; uses `HostConfig.Memory` when > 0, else 1 GiB |
| BUG-412 | server.go | `InitDefaultNetwork` now creates `host` (driver "host") and `none` (driver "null") networks |
| BUG-413 | handle_commit.go | Added `container` `commit` event emission before response |
| BUG-414 | handle_extended.go | Added `container` `update` event emission after Store update |
| BUG-415 | handle_images.go | Added `image` `load` event emission after store put |
| BUG-416 | handle_extended.go | Container prune calculates `SpaceReclaimed` from `Drivers.Filesystem.RootPath` + `DirSize` |
| BUG-417 | handle_volumes.go | Volume prune calculates `SpaceReclaimed` from `VolumeDirs` + `DirSize` before `os.RemoveAll` |
| BUG-418 | handle_containers_query.go | Deterministic `ImageID` via `sha256.Sum256([]byte(c.Config.Image))` instead of `GenerateID()` |
| BUG-419 | filters.go | Added `case "paused"` to `FormatStatus` — returns "Up X (Paused)" with duration |
| BUG-420 | health.go | Health check exec instances deleted via `Store.Execs.Delete(execID)` after completion |
| BUG-421 | server.go | `handleInfo` counts paused containers separately; paused also counted as running (Docker behavior) |
| BUG-422 | handle_containers_query.go | `handleContainerLogs` parses `stdout`/`stderr` params; suppresses output when `stdout=false` |

## Verification

- `cd backends/core && go test -race -count=1 ./...` — PASS
- All 8 backends build (`go build -tags noui ./...`) — PASS
- Frontend builds (`go build -tags noui ./...`) — PASS
