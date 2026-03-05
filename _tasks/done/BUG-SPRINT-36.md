# Bug Sprint 36 — BUG-450 → BUG-462

**Date:** 2026-03-05
**Bugs fixed:** 13

## Summary

System df response field gaps (images and containers), container list SizeRw parity, image commit/build GraphDriver gaps, and Container.Image set to sha256 ID instead of reference name.

## Bugs

| ID | Component | Fix |
|----|-----------|-----|
| BUG-450 | Core | Added `VirtualSize` to system df ImageSummary |
| BUG-451 | Core | Added `Labels` (from `img.Config.Labels`) to system df ImageSummary |
| BUG-452 | Core | Added `SizeRw` from `DirSize(rootPath)` when `size=true` in container list |
| BUG-453 | Core | Added synthetic `RootFS.Layers` to committed images |
| BUG-454 | Core | Added `GraphDriver` (overlay2) to committed images |
| BUG-455 | Core | Added `GraphDriver` (overlay2) to built images |
| BUG-456 | Core | Added `ImageID` to system df ContainerSummary via `ResolveImage` |
| BUG-457 | Core | Added `Command` (Path + Args) to system df ContainerSummary |
| BUG-458 | Core | Added `Status` via `FormatStatus()` to system df ContainerSummary |
| BUG-459 | Core | Added `Labels` and `Ports` to system df ContainerSummary |
| BUG-460 | Core | Added `Mounts`, `NetworkSettings`, `HostConfig` to system df ContainerSummary |
| BUG-461 | Core | Added `SizeRootFs` from image size to system df ContainerSummary |
| BUG-462 | Core | `buildContainerFromConfig` now sets `Container.Image` to sha256 image ID |

## Files Modified

| File | Bugs |
|------|------|
| `backends/core/handle_extended.go` | 450, 451, 456–461 |
| `backends/core/handle_containers_query.go` | 452 |
| `backends/core/handle_commit.go` | 453, 454 |
| `backends/core/build.go` | 455 |
| `backends/core/handle_containers_archive.go` | 462 |

## Verification

- `cd backends/core && go test -race -count=1 ./...` — PASS
- All 8 backends build with `-tags noui` — PASS
