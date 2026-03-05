# Bug Sprint 45 — BUG-575 → BUG-583

**Date**: 2026-03-06
**Bugs fixed**: 8 (BUG-580 confirmed false positive)

## Bugs

| Bug | Component | Fix |
|-----|-----------|-----|
| BUG-575 | Docker | Map `info.GraphDriver` in `handleImageInspect` |
| BUG-576 | Docker | Add IPv6Gateway, GlobalIPv6Address, GlobalIPv6PrefixLen, DriverOpts, IPAMConfig to `handleSystemDf` EndpointSettings |
| BUG-577 | Docker | Add HostConfig with NetworkMode to `handleSystemDf` ContainerSummary |
| BUG-578 | Docker | Map UsageData in `handleSystemDf` volume section |
| BUG-579 | Docker | Map UsageData in `handleVolumeCreate`, `handleVolumeList`, `handleVolumeInspect` |
| BUG-580 | Docker | FALSE POSITIVE — Docker SDK `MountPoint` has no Consistency field |
| BUG-581 | Docker | Check `X-Registry-Auth` header first, fall back to query param `auth` in `handleImagePush` |
| BUG-582 | Docker | Return empty string for zero `LastUsedAt` in BuildCache |
| BUG-583 | Frontend | Forward all `changes` query params in `handleContainerCommit` |

## Files Modified

- `backends/docker/images.go` — BUG-575
- `backends/docker/extended.go` — BUG-576, 577, 578, 581, 582
- `backends/docker/volumes.go` — BUG-579
- `frontends/docker/images.go` — BUG-583
- `FEATURE_MATRIX.md` — Added docker import
- `BUGS.md`, `STATUS.md`, `WHAT_WE_DID.md` — State updates

## Verification

- `cd backends/docker && go build ./...` ✅
- `cd frontends/docker && go build ./...` ✅
- `cd backends/core && go test -race -count=1 ./...` — 302 PASS ✅
- `cd frontends/docker && go test -race -count=1 ./...` — 7 PASS ✅
- `make lint` — 0 issues ✅
