# Bug Sprint 38 — BUG-476 → BUG-488

**Date:** 2026-03-05
**Bugs Fixed:** 13

## Summary

Stop/restart timeout parameter gaps, container prune SpaceReclaimed across 6 cloud backends, volume prune SpaceReclaimed across 3 backends, container wait "removed" condition, image save config JSON.

## Bugs

| Bug | Component | Fix |
|-----|-----------|-----|
| BUG-476 | Core | `handleContainerStop` reads `t` query param |
| BUG-477 | Core | `handleContainerRestart` reads `t` query param |
| BUG-478 | ECS | `handleContainerPrune` sums image sizes for SpaceReclaimed |
| BUG-479 | CloudRun | `handleContainerPrune` sums image sizes for SpaceReclaimed |
| BUG-480 | ACA | `handleContainerPrune` sums image sizes for SpaceReclaimed |
| BUG-481 | Lambda | `handleContainerPrune` sums image sizes for SpaceReclaimed |
| BUG-482 | GCF | `handleContainerPrune` sums image sizes for SpaceReclaimed |
| BUG-483 | AZF | `handleContainerPrune` sums image sizes for SpaceReclaimed |
| BUG-484 | ECS | `handleVolumePrune` sums volume dir sizes for SpaceReclaimed |
| BUG-485 | CloudRun | `handleVolumePrune` sums volume dir sizes for SpaceReclaimed |
| BUG-486 | ACA | `handleVolumePrune` sums volume dir sizes for SpaceReclaimed |
| BUG-487 | Core | `handleContainerWait` handles "removed" condition |
| BUG-488 | Core | `handleImageSave` writes config JSON entries |

## Files Modified

- `backends/core/handle_containers.go` — BUG-476, 477, 487
- `backends/core/handle_images.go` — BUG-488
- `backends/ecs/extended.go` — BUG-478, 484
- `backends/cloudrun/extended.go` — BUG-479, 485
- `backends/aca/extended.go` — BUG-480, 486
- `backends/lambda/extended.go` — BUG-481
- `backends/cloudrun-functions/extended.go` — BUG-482
- `backends/azure-functions/extended.go` — BUG-483

## Tests

- Core: 302 PASS
- Frontend: 7 PASS (TLS + mux)
- All 8 backends build clean
