# Bug Sprint 37 — BUG-463 → BUG-475

**Date:** 2026-03-05
**Bugs fixed:** 13

## Summary

Image history/save correctness, stats networks field, container size pointer fields, container inspect size param, frontend version KernelVersion, container update PidsLimit/OomKillDisable, image push auth forwarding, image LastTagTime on mutations, and image search limit.

## Bugs

| ID | Component | Fix |
|----|-----------|-----|
| BUG-463 | Core | `handleImageHistory` returns per-layer entries instead of single hardcoded entry |
| BUG-464 | Core | `handleImageSave` manifest uses actual `RootFS.Layers` instead of empty array |
| BUG-465 | Core | `buildStatsEntry` populates `networks` field from container `NetworkSettings` |
| BUG-466 | API | Container struct gets `SizeRw`/`SizeRootFs` pointer fields |
| BUG-467 | Core | `handleContainerInspect` respects `size` query parameter |
| BUG-468 | Frontend | `handleContainerInspect` forwards `size` query param to backend |
| BUG-469 | Frontend | `handleVersion` uses `info.KernelVersion` instead of empty string |
| BUG-470 | API+Core | `ContainerUpdateRequest` gets `PidsLimit` field |
| BUG-471 | API+Core | `ContainerUpdateRequest` gets `OomKillDisable` field |
| BUG-472 | Frontend | `handleImagePush` forwards `X-Registry-Auth` header as query param |
| BUG-473 | Core | `handleImagePush` accepts auth query param |
| BUG-474 | Core | Image `Metadata.LastTagTime` set on pull/load/build/commit/tag operations |
| BUG-475 | Core | `handleImageSearch` respects `limit` query parameter |

## Files Modified

| File | Bugs |
|------|------|
| `api/types.go` | 466, 470, 471 |
| `backends/core/handle_images.go` | 463, 464, 473, 474, 475 |
| `backends/core/handle_extended.go` | 465 |
| `backends/core/handle_containers_query.go` | 467 |
| `backends/core/handle_commit.go` | 474 |
| `backends/core/build.go` | 474 |
| `frontends/docker/containers.go` | 468 |
| `frontends/docker/system.go` | 469 |
| `frontends/docker/images.go` | 472 |

## Verification

- `cd backends/core && go test -race -count=1 ./...` — PASS
- All 8 backends build with `-tags noui` — PASS
