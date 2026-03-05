# Bug Sprint 39 — BUG-489 → BUG-501

**Date:** 2026-03-05
**Bugs fixed:** 13

## Bugs

| ID | Sev | Component | Fix |
|----|-----|-----------|-----|
| BUG-489 | Med | Core | Added `expose` filter case to `MatchContainerFilters` — checks `Config.ExposedPorts` with `/tcp` fallback |
| BUG-490 | Med | Core | Added `before` filter to `handleImageList` — compares image `Created` timestamps |
| BUG-491 | Med | Core | Added `since` filter to `handleImageList` — compares image `Created` timestamps |
| BUG-492 | Low | Frontend | `handleImageLoad` now forwards `quiet` query param to backend |
| BUG-493 | Low | Core | `handleImageLoad` reads `quiet` param — suppresses stream output when `quiet=1` or `quiet=true` |
| BUG-494 | Med | Core | `handleImagePush` reads `auth` query param — decodes base64 JSON and stores credentials |
| BUG-495 | Med | Core | `handleContainerResize` reads `h`/`w` query params and stores on `HostConfig.ConsoleSize` |
| BUG-496 | Med | Core | `handleExecResize` reads `h`/`w` query params and stores on exec's container `HostConfig.ConsoleSize` |
| BUG-497 | Low | Core | `handleContainerTop` synthetic fallback uses `c.State.Pid` instead of hardcoded `"1"` |
| BUG-498 | Med | Frontend | `handleImageCreate` with `fromSrc` now tags loaded image with user's `repo`/`tag` params |
| BUG-499 | Med | Core | `handleImageTag` validates empty `repo` — returns 400 "repository name must have at least one component" |
| BUG-500 | Med | Core | Added `dangling` filter to `handleVolumeList` — builds in-use set from container mounts/binds |
| BUG-501 | Med | Frontend | `handleExecStart` uses `application/vnd.docker.raw-stream` when TTY is enabled |

## Files Modified

| File | Bugs |
|------|------|
| `backends/core/filters.go` | 489 |
| `backends/core/handle_images.go` | 490, 491, 493, 494, 499 |
| `backends/core/handle_extended.go` | 495, 496, 497 |
| `backends/core/handle_volumes.go` | 500 |
| `frontends/docker/images.go` | 492, 498 |
| `frontends/docker/exec.go` | 501 |

## Verification

- `backends/core`: 302 PASS
- `frontends/docker`: 7 PASS (TLS + mux)
- All 8 backends: build OK with `-tags noui`
