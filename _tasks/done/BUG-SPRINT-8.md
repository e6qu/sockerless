# Bug Sprint 8 — api, backends, frontends audit (BUG-052→062)

**Completed**: 2026-03-03

## Bugs Fixed

| Bug | Severity | File | Summary |
|-----|----------|------|---------|
| BUG-052 | High | `backends/core/handle_containers_archive.go` | `extractTar` ignores `io.Copy` error — silent file corruption |
| BUG-053 | High | `backends/core/handle_containers_archive.go` | `handlePutArchive` swallows driver error — returns 200 on failure |
| BUG-054 | Medium | `backends/core/handle_containers_archive.go` | `mergeStagingDir` silently ignores all file copy errors |
| BUG-055 | Medium | `backends/core/handle_containers_archive.go` | `createTar` ignores `tw.WriteHeader` and `io.Copy` errors |
| BUG-058 | High | `frontends/docker/networks.go` | `handleNetworkPrune` doesn't forward `filters` query parameter |
| BUG-059 | Medium | `backends/core/handle_commit.go` | `handleContainerCommit` ignores JSON decode error on request body |
| BUG-060 | Medium | `backends/core/build.go` | `handleImageBuild` ignores `buildargs` JSON unmarshal error |
| BUG-061 | Low | `backends/core/drivers_agent.go` | Agent drivers ignore container-not-found from Store.Get |
| BUG-062 | Medium | `backends/ecs/containers.go` | ECS `startMultiContainerTask` leaks task definition on `runECSTask` failure |

## Files Modified

- `backends/core/handle_containers_archive.go` — BUG-052, 053, 054, 055
- `backends/core/handle_containers_export.go` — BUG-055 caller update
- `backends/core/drivers_agent.go` — BUG-055 caller + BUG-061
- `backends/core/drivers_process.go` — BUG-055 caller
- `backends/core/drivers_synthetic.go` — BUG-055 caller
- `backends/core/handle_commit.go` — BUG-059
- `backends/core/build.go` — BUG-060
- `frontends/docker/networks.go` — BUG-058
- `backends/ecs/containers.go` — BUG-062
- `backends/core/archive_bugfix_test.go` — 10 new tests
- `BUGS.md` — added BUG-052→062

## Test Results

- Core: 286 PASS (276 existing + 10 new)
- Frontend: 7 PASS
- ECS: compiles, no unit tests
- Lint: 0 issues across 19 modules
