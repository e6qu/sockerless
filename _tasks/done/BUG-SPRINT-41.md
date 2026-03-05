# Bug Sprint 41 — BUG-515 → BUG-527

**Date:** 2026-03-05
**Bugs fixed:** 13 (BUG-515 through BUG-527)

## Summary

Container config merge parity, stats response gaps, HostConfig missing fields, cloud error mapping for forward-agent backends, Dockerfile parser bugs, volume status code, image save validation, and events since/until support.

## Bugs Fixed

| Bug | Component | Fix |
|-----|-----------|-----|
| BUG-515 | Core | ENV merge by key — image defaults, container overrides (not all-or-nothing) |
| BUG-516 | Core | Clear image Cmd when container overrides Entrypoint |
| BUG-517 | Core | Stats response now includes `id` and `name` fields |
| BUG-518 | Core | Stats `precpu_stats` tracks previous CPU reading per container via PrevCPUStats sync.Map |
| BUG-519 | Core | Volume create returns 200 for existing volume (was 201) |
| BUG-520 | Core | Events endpoint supports `since`/`until` query params with history replay |
| BUG-521 | ECS | Added `mapAWSError` for proper 404/409/400 error mapping |
| BUG-522 | CloudRun | Added `mapGCPError` for proper 404/409/400 error mapping |
| BUG-523 | ACA | Added `mapAzureError` for proper 404/409/400 error mapping |
| BUG-524 | Core | Image save validates all images exist before writing tar stream |
| BUG-525 | Core | parseLabels uses quote-aware tokenizer for `LABEL foo="bar baz"` |
| BUG-526 | Core | parseEnv handles multi-value `ENV k1=v1 k2=v2` |
| BUG-527 | API | Added `NanoCpus` field to HostConfig |

## Files Modified

- `backends/core/handle_containers.go` — BUG-515, BUG-516
- `backends/core/handle_extended.go` — BUG-517, BUG-518, BUG-520
- `backends/core/handle_volumes.go` — BUG-519
- `backends/core/handle_images.go` — BUG-524
- `backends/core/build.go` — BUG-525, BUG-526
- `backends/core/store.go` — BUG-518
- `backends/core/event_bus.go` — BUG-520
- `backends/ecs/errors.go` — BUG-521
- `backends/cloudrun/errors.go` — BUG-522
- `backends/aca/errors.go` — BUG-523
- `api/types.go` — BUG-527

## Test Results

- `backends/core`: PASS
- `frontends/docker`: PASS
- All 8 backends + frontend: build OK
