# Bug Sprint 32 — BUG-395 → BUG-408

**14 bugs fixed**

## Bugs Fixed

| Bug | Sev | Component | Fix |
|-----|-----|-----------|-----|
| BUG-395 | High | All 6 cloud | `handleContainerKill`: moved WaitChs close AFTER EmitEvent calls so event watchers see events first |
| BUG-396 | Med | All 6 cloud | `handleContainerRemove`: added `TmpfsDirs.LoadAndDelete` + `os.RemoveAll` cleanup after `StagingDirs.Delete` |
| BUG-397 | Med | All 6 cloud | `handleContainerPrune`: added `TmpfsDirs.LoadAndDelete` + `os.RemoveAll` cleanup in prune loop |
| BUG-398 | Med | Frontend | `handleContainerList`: forward `size` query parameter to backend |
| BUG-399 | Med | Core + Frontend | `handleImagePush`: replaced hardcoded stub with real handler in core + frontend proxy |
| BUG-400 | Med | Core | `handlePodList`: added `filters` query parameter support (name, id, label, status) |
| BUG-401 | Med | Frontend | `handlePodList`: forward `filters` query parameter to backend |
| BUG-402 | Low | Frontend | `handleContainerStop`: forward `signal` query parameter |
| BUG-403 | Low | Frontend | `handleImageCreate`: forward `platform` query parameter (added to ImagePullRequest) |
| BUG-404 | Low | Frontend | `handleImageList`: forward `shared-size` and `digests` query parameters |
| BUG-405 | Low | Frontend | `handleContainerPutArchive`: forward `noOverwriteDirNonDir` query parameter |
| BUG-406 | Low | Frontend | `handleNetworkRemove`: forward `force` query parameter (switched from `delete` to `deleteWithQuery`) |
| BUG-407 | Low | Core + Frontend | Added `GET /images/get` and `GET /images/{name}/get` for `docker save` (tar with manifest.json) |
| BUG-408 | Low | Core + Frontend | Added `GET /images/search` — searches local image store by term, replacing NotImplemented stub |

## Files Modified

- `backends/{ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}/containers.go` — BUG-395, 396
- `backends/{ecs,lambda,cloudrun,cloudrun-functions,aca,azure-functions}/extended.go` — BUG-397
- `backends/core/handle_images.go` — BUG-399, 407, 408
- `backends/core/handle_pods.go` — BUG-400
- `backends/core/server.go` — BUG-399, 407, 408
- `frontends/docker/containers.go` — BUG-398, 402
- `frontends/docker/images.go` — BUG-399, 403, 404, 407
- `frontends/docker/pods.go` — BUG-401
- `frontends/docker/containers_stream.go` — BUG-405
- `frontends/docker/networks.go` — BUG-406
- `frontends/docker/server.go` — BUG-407, 408
- `api/types.go` — BUG-403 (Platform field)

## Verification

- `cd backends/core && go test -race -count=1 ./...` — 302 PASS
- All 8 backends: `go build -tags noui ./...` — clean
- `cd frontends/docker && go build -tags noui ./...` — clean
