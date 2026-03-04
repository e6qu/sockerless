# Bug Sprint 26 — BUG-295 → BUG-319 (25 bugs)

## Summary

Deep audit of `api/types.go`, all `backends/core/` handlers, and all 6 cloud backend handlers. Fixed WaitCh leaks, HTTP status codes, symlink path traversal, cloud event gaps, and API type additions.

## Bugs Fixed

| Bug | Sev | Component | Fix |
|-----|-----|-----------|-----|
| BUG-295 | Med | Core | `handleContainerPrune` closes WaitCh channel before delete (prevents waiter hangs) |
| BUG-296 | Med | Core | `handlePodRemove` cleans up TmpfsDirs on disk |
| BUG-297 | Med | Core | `handleContainerStats` stream=true on stopped container returns stats then closes |
| BUG-298 | Med | Core | `handleVolumePrune` now respects `label` and `until` filters |
| BUG-299 | Med | Core | `handlePutArchive` returns 204 instead of 200 |
| BUG-300 | Med | Core | `handleNetworkDisconnect` returns 204 instead of 200 |
| BUG-301 | Med | Core | `handleNetworkConnect` returns 204 instead of 200 |
| BUG-302 | Med | Core | `extractTar` symlink validation prevents path traversal |
| BUG-303 | Med | Core | `handlePodStart` creates WaitChs and emits start events |
| BUG-304 | Low | Core | `handleContainerKill` emits events before closing WaitCh |
| BUG-305 | Med | Cloud | All 6 cloud backends emit `start` event in container start |
| BUG-306 | Med | Cloud | All 6 cloud backends use actual exit code in `die` event |
| BUG-307 | Low | ACA | `handleContainerRestart` guards empty JobName in MarkCleanedUp |
| BUG-308 | Low | Cloud | Lambda/GCF/AZF populate `NetworkSettings.Networks` on create |
| BUG-309 | Low | API | `ContainerConfig.Labels` nil→`{}` |
| BUG-310 | Low | API | `ContainerSummary.Labels` nil→`{}` |
| BUG-311 | Low | API | `ContainerSummary.Mounts` nil→`[]` |
| BUG-312 | Low | API | `NetworkSettings.Ports` removed `omitempty` |
| BUG-313 | Low | API | `Network.Containers/Options/Labels` nil→`{}` |
| BUG-314 | Low | API | `Volume.Labels` nil→`{}`, `Volume.Options` removed `omitempty` |
| BUG-315 | Low | API | `VolumeListResponse.Warnings` nil→`[]` |
| BUG-316 | Low | API | `Port.PublicPort` removed `omitempty` |
| BUG-317 | Low | API | `HostConfig` added `PublishAllPorts`, `CgroupnsMode`, `ConsoleSize` |
| BUG-318 | Low | API | `NetworkSettings` added IPv6 fields and `EndpointID` |
| BUG-319 | Low | API | `EndpointSettings` added `DNSNames []string` |

## Files Modified (31)

- `api/types.go`
- `backends/core/handle_extended.go`, `handle_pods.go`, `handle_containers.go`
- `backends/core/handle_containers_archive.go`, `handle_networks.go`, `handle_volumes.go`
- `backends/core/handle_containers_query.go`, `handle_images.go`
- `backends/core/archive_bugfix_test.go`, `network_disconnect_test.go`, `pod_test.go`
- `backends/ecs/{containers,extended,server}.go`
- `backends/cloudrun/{containers,extended,server}.go`
- `backends/aca/{containers,extended,server}.go`
- `backends/lambda/{containers,extended,server}.go`
- `backends/cloudrun-functions/{containers,extended,server}.go`
- `backends/azure-functions/{containers,extended,server}.go`
- `backends/docker/containers.go`

## Verification

- `cd backends/core && go test -race -count=1 ./...` — PASS
- All 8 backend modules build clean with `-tags noui`
