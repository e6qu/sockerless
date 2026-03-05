# Bug Sprint 42 — Cloud ENV/Cmd, Error Mapping, Docker HostConfig, Commit Params (BUG-528→540)

**Date**: 2026-03-05
**Scope**: `backends/ecs`, `backends/cloudrun`, `backends/aca`, `backends/docker`, `backends/core`, `backends/cloudrun-functions`, `api/`

## Bugs Fixed

| Bug | Severity | File(s) | Fix |
|-----|----------|---------|-----|
| BUG-528 | High | `backends/{ecs,cloudrun,aca}/containers.go` | ENV merge changed from all-or-nothing to key-based `mergeEnvByKey` |
| BUG-529 | Med | `backends/{ecs,cloudrun,aca}/containers.go` | Clear inherited Cmd when Entrypoint overridden |
| BUG-530 | High | `backends/ecs/errors.go`, `containers.go` | Added `mapAWSError` + wrapped 6 error sites |
| BUG-531 | High | `backends/cloudrun/errors.go`, `containers.go` | Added `mapGCPError` + wrapped 10 error sites |
| BUG-532 | High | `backends/aca/errors.go`, `containers.go` | Added `mapAzureError` + wrapped 10 error sites |
| BUG-533 | High | `backends/docker/containers.go` | 26 HostConfig fields added to create mapping |
| BUG-534 | High | `backends/docker/containers.go` | 26 HostConfig fields added to inspect mapping |
| BUG-535 | Low | `backends/docker/extended.go` | `handleNetworkConnect` returns 204 not 200 |
| BUG-536 | Med | `backends/core/handle_commit.go` | `changes` query param with Dockerfile instructions |
| BUG-537 | Med | `backends/core/handle_commit.go` | `pause` query param accepted (no-op for synthetic) |
| BUG-538 | Med | `backends/cloudrun-functions/containers.go` | `http.Post` → `gcfHTTPClient` with 10min timeout |
| BUG-539 | Med | `backends/docker/extended.go` | Docker commit passes `changes` and `pause` to SDK |
| BUG-540 | Med | `backends/docker/images.go` | Image pull passes `Platform` field |

## Additional Changes

- Added `NanoCpus int64` to `api.HostConfig` struct
- Created `FEATURE_MATRIX.md` — Docker API compatibility matrix across all 9 backends

## Verification

- `backends/core`: 302 PASS (go test -race)
- `frontends/docker`: 7 PASS (go test -race)
- All 10 modules: `go build -tags noui ./...` — clean
- All 7 modified backends: `go vet ./...` — clean
