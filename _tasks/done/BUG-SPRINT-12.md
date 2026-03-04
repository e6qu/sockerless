# Bug Sprint 12 — API & Backends Audit (BUG-083→090)

**Date**: 2026-03-04
**Status**: Complete
**Bugs Fixed**: 8

## Summary

Audited Docker CREATE direction (API→Docker), FaaS backend lifecycle consistency, core handler correctness, and Docker network/inspect field gaps.

## Bugs

| Bug | Severity | File(s) | Fix |
|-----|----------|---------|-----|
| BUG-083 | High | `backends/docker/containers.go` | Map all 14 missing HostConfig fields (PortBindings, RestartPolicy, Privileged, etc.) |
| BUG-084 | Medium | `backends/docker/containers.go` | Map 7 missing Config fields (StdinOnce, Domainname, Shell, StopTimeout, ExposedPorts, Volumes, Healthcheck) |
| BUG-085 | High | `backends/lambda/extended.go`, `backends/cloudrun-functions/extended.go`, `backends/azure-functions/extended.go` + server.go | Add pause/unpause overrides returning NotImplementedError |
| BUG-086 | Medium | `backends/core/handle_extended.go` | Add already-paused check before not-running check |
| BUG-087 | Medium | `backends/core/handle_exec.go` | Check container existence in exec start |
| BUG-088 | Medium | `backends/core/handle_extended.go` | Add event emission for rename, pause, unpause |
| BUG-089 | Medium | `backends/docker/extended.go` | Add Aliases to network connect mapping |
| BUG-090 | Medium | `backends/docker/containers.go` | Add Aliases to inspect NetworkSettings mapping |

## Verification

- `go build ./...` — all 5 affected modules compile
- `go test -race -count=1 ./...` (core) — 286 PASS
- `make lint` — 0 issues across 19 modules
