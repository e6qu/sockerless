# Bug Sprint 15 — API & Backends Audit (BUG-107→114)

**Date**: 2026-03-04
**Bugs Fixed**: 8 (4 high, 4 medium)

## Summary

Follow-up audit of `backends/core/`, `backends/cloudrun/`, `backends/aca/`, `backends/lambda/`, `backends/cloudrun-functions/`, `backends/azure-functions/`, and `backends/docker/`. Focused on cleanup gaps, Cmd/Args mapping, AgentRegistry leaks, and Docker passthrough filter/auth issues.

## Bugs

| Bug | Severity | File(s) | Fix |
|-----|----------|---------|-----|
| BUG-107 | High | `core/handle_pods.go` | Pod force-remove: added health, process, network, log, wait, staging, execs cleanup |
| BUG-108 | Medium | `core/handle_containers.go`, `core/handle_extended.go` | Container remove/prune: added StagingDirs + Execs cleanup |
| BUG-109 | High | `cloudrun/jobspec.go` | Track entrypoint/command separately, set `Args` field |
| BUG-110 | High | `aca/jobspec.go` | Track entrypoint/command separately, set `Args` field |
| BUG-111 | Medium | `lambda/containers.go`, `gcf/containers.go`, `azf/containers.go` | Add `AgentRegistry.Remove(id)` on timeout |
| BUG-112 | High | `docker/images.go` | Set `PullOptions{RegistryAuth: req.Auth}` |
| BUG-113 | Medium | `docker/extended.go`, `docker/networks.go` | Parse `filters` query param in all 4 prune handlers |
| BUG-114 | Medium | `docker/networks.go`, `docker/volumes.go`, `docker/extended.go` | Parse `filters` query param in 3 list handlers |

## Verification

- Core tests: 286 PASS (race detector)
- All 7 modified modules build cleanly
- Lint: 0 issues
