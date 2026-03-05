# Bug Sprint 35 — BUG-437 → BUG-449

**Status**: Complete
**Bugs Fixed**: 13

## Summary

Container inspect path fields, NetworkSettings synthetic fields, KernelVersion, exec CanRemove, frontend df type param, image GraphDriver/RootFS, volume UsageData.

## Bugs

| ID | Component | Fix |
|----|-----------|-----|
| BUG-437 | Core | Set `LogPath` in `buildContainerFromConfig` |
| BUG-438 | Core | Set `ResolvConfPath` in `buildContainerFromConfig` |
| BUG-439 | Core | Set `HostnamePath` in `buildContainerFromConfig` |
| BUG-440 | Core | Set `HostsPath` in `buildContainerFromConfig` |
| BUG-441 | Core | Set `NetworkSettings.SandboxID` to container ID |
| BUG-442 | Core | Set `NetworkSettings.SandboxKey` to `/var/run/docker/netns/<id[:12]>` |
| BUG-443 | Core | Set `NetworkSettings.Bridge` to `docker0` for bridge network |
| BUG-444 | Core | Set `KernelVersion` to `5.15.0-sockerless` in `handleInfo` |
| BUG-445 | Core | Set `CanRemove = true` after exec completes or errors |
| BUG-446 | Frontend | Forward `type` query param in `handleSystemDf` |
| BUG-447 | Core | Set `RootFS.Layers` with synthetic layer in `handleImageLoad` |
| BUG-448 | Core | Set `GraphDriver` with overlay2 metadata in both pull and load |
| BUG-449 | Core | Set `UsageData` with RefCount and Size in `handleSystemDf` volumes |

## Files Modified

- `backends/core/handle_containers_archive.go` (BUG-437–443)
- `backends/core/server.go` (BUG-444)
- `backends/core/handle_exec.go` (BUG-445)
- `frontends/docker/system.go` (BUG-446)
- `backends/core/handle_images.go` (BUG-447, 448)
- `backends/core/handle_extended.go` (BUG-449)

## Verification

- `backends/core`: 302 PASS
- All 9 backends: build clean (`-tags noui`)
- `frontends/docker`: build clean (`-tags noui`)
