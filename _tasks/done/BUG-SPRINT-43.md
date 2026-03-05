# Bug Sprint 43 — BUG-541 → BUG-553

**Date**: 2026-03-05
**Bugs fixed**: 13
**Total bugs fixed**: 553 (43 sprints)

## Bugs Fixed

| Bug | Sev | Component | Fix |
|-----|-----|-----------|-----|
| BUG-541 | High | Lambda/GCF/AZF | ENV merge changed from all-or-nothing to key-based `core.MergeEnvByKey` |
| BUG-542 | Med | Lambda/GCF/AZF | Cmd/Entrypoint: clear image Cmd when only Entrypoint overridden |
| BUG-543 | Med | Docker | `handleImageTag` returns 200 OK (was 201 Created) |
| BUG-544 | Med | Docker | `handleContainerCreate` Healthcheck mapping: add `StartInterval` |
| BUG-545 | Med | Docker | `mapContainerFromDocker` Healthcheck inspect: add `StartInterval` |
| BUG-546 | Low | Docker | `handleContainerCreate` ContainerConfig: add `ArgsEscaped`, `NetworkDisabled`, `OnBuild` |
| BUG-547 | Low | Docker | `mapContainerFromDocker` inspect Config: add `ArgsEscaped`, `NetworkDisabled`, `OnBuild` |
| BUG-548 | Med | Docker | `mapContainerFromDocker` Mount inspect: add `VolumeOptions`, `TmpfsOptions` |
| BUG-549 | Low | Core | Replace hardcoded `Pid=42` with `Store.NextPID()` atomic counter |
| BUG-550 | Low | Core | Replace hardcoded exec `Pid=43` with `Store.NextPID()` |
| BUG-551 | Low | Core | Replace hardcoded image size 7654321 with deterministic FNV hash |
| BUG-552 | Med | Docker | `handleContainerCreate` ContainerConfig: add `MacAddress` |
| BUG-553 | Med | Docker | `mapContainerFromDocker` inspect Config: add `MacAddress` |

## Files Modified

| File | Bugs |
|------|------|
| `backends/lambda/containers.go` | 541, 542 |
| `backends/cloudrun-functions/containers.go` | 541, 542 |
| `backends/azure-functions/containers.go` | 541, 542 |
| `backends/docker/images.go` | 543 |
| `backends/docker/containers.go` | 544, 545, 546, 547, 548, 552, 553 |
| `backends/core/store.go` | 549 (NextPID counter) |
| `backends/core/handle_containers.go` | 541 (export MergeEnvByKey), 549 |
| `backends/core/restart_policy.go` | 549 |
| `backends/core/handle_pods.go` | 549 |
| `backends/core/handle_exec.go` | 550 |
| `backends/core/handle_images.go` | 551 |
| `backends/core/compose_lifecycle_test.go` | 549 (test update) |

## Verification

- `cd backends/core && go test -race -count=1 ./...` — 302 PASS
- `cd frontends/docker && go test -race -count=1 ./...` — 7 PASS
- All 5 modified backends build clean
- `make lint` — 0 issues across all modules
