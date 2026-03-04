# Bug Sprint 11 — API & Backends Audit (BUG-075→082)

**Completed**: 2026-03-04

## Summary

Audited cross-backend lifecycle consistency (restart semantics) and Docker passthrough mapping fidelity. Found and fixed 8 bugs.

## Bugs Fixed

| Bug | Severity | Component | Fix |
|-----|----------|-----------|-----|
| BUG-075 | High | lambda/extended.go, server.go | Added no-op restart handler (matching GCF/AZF) |
| BUG-076 | High | docker/containers.go | Map all 17 HostConfig fields (was 3) |
| BUG-077 | Medium | docker/containers.go | Map ExposedPorts, Volumes, Shell, Healthcheck, StopTimeout |
| BUG-078 | High | docker/containers.go | Map State.Health (Status, FailingStreak, Log) |
| BUG-079 | High | docker/containers.go | Map NetworkSettings.Ports via mapPortBindings |
| BUG-080 | High | docker/containers.go | Map Ports, Mounts, SizeRw, NetworkSettings in list |
| BUG-081 | High | docker/networks.go | Map IPAM and Containers in list + inspect |
| BUG-082 | Medium | docker/images.go | Map all 19 ContainerConfig fields (was 5) |

## Files Modified

- `backends/lambda/extended.go` — BUG-075
- `backends/lambda/server.go` — BUG-075
- `backends/docker/containers.go` — BUG-076, 077, 078, 079, 080
- `backends/docker/networks.go` — BUG-081
- `backends/docker/images.go` — BUG-082
- `BUGS.md` — Added BUG-075→082

## Verification

- `cd backends/lambda && go build ./...` — compiles
- `cd backends/docker && go build ./...` — compiles
- `cd backends/core && go test -race -count=1 ./...` — 286 PASS
- `make lint` — 0 issues across 19 modules
- `make sim-test-all` — 75 PASS (6 backends)
