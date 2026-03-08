# Upstream Act Test Results

**Act version:** v0.2.84
**Date:** 2026-02-21
**Test image:** alpine:latest

## Multi-Backend Summary

TestRunEvent Docker mode subtests — the primary upstream compatibility metric:

| Backend | PASS | FAIL | Total | Mode | Notes |
|---------|:----:|:----:|:-----:|------|-------|
| ecs | 56 | 31 | 87 | monolithic | Reverse agent (simulator) |
| lambda | 57 | 30 | 87 | monolithic | Reverse agent (FaaS simulator) |
| cloudrun | 54 | 33 | 87 | monolithic | Reverse agent (simulator) |
| gcf | 58 | 29 | 87 | monolithic | Reverse agent (FaaS simulator) |
| aca | 58 | 29 | 87 | monolithic | Reverse agent (simulator) |
| azf | 69 | 16 | 85 | individual | Hangs in monolithic; individual required |

Full test suite (including TestRunEventHostEnvironment, Secrets, PullRequest):

| Backend | PASS | FAIL | Total |
|---------|:----:|:----:|:-----:|
| ecs | 93 | 41 | 134 |
| lambda | 94 | 39 | 133 |
| cloudrun | 91 | 42 | 133 |
| gcf | 95 | 38 | 133 |
| aca | 95 | 38 | 133 |
| azf | ~100 | ~34 | ~134 |

### Run mode notes

- **Individual mode** (`--individual`): Each subtest runs in its own `go test` invocation with 3-minute timeout. Isolates hanging tests. Required for azf (monolithic hangs on first test).
- **Monolithic mode** (default): All subtests in one `go test -timeout 60m`. Faster (~3-5 min for cloud backends), but one hanging test blocks everything.
- Auto-detection: azf uses individual, other backends use monolithic.

## Failure Categories

### Node.js (16 tests — all backends)

Tests requiring JavaScript GitHub Actions need Node.js runtime.

| Test | Notes |
|------|-------|
| basic | node (action) |
| remote-action-js-node-user | node |
| local-action-js | node |
| GITHUB_ENV-use-in-env-ctx | node |
| GITHUB_STATE | node |
| stepsummary | node |
| actions-environment-and-context-tests | node (passes on cloud backends with real node) |
| local-remote-action-overrides | node |
| remote-action-js | node |
| remote-action-composite-js-pre-with-defaults | node |
| uses-composite-check-for-input-collision | node |
| uses-composite-check-for-input-shadowing | node |
| uses-action-with-pre-and-post-step | node |
| uses-nested-composite | node |
| issue-597 | node |
| issue-598 | node |

### Build (2 tests — all backends)

| Test | Root Cause |
|------|-----------|
| local-action-dockerfile | Needs `POST /build` (returns 501) |
| local-action-via-composite-dockerfile | Needs `POST /build` + node |

### API Gap (2 tests — all backends)

| Test | Root Cause |
|------|-----------|
| uses-docker-url | Cmd/entrypoint handling gap |
| docker-action-custom-path | Custom working directory in Docker action |

### Network/Services (4 tests — all backends)

| Test | Root Cause |
|------|-----------|
| services-with-container | Service container health checks + inter-container networking |
| mysql-service-container-with-health-check | MySQL service + health check polling |
| services-host-network | Host network mode not supported |
| networking | Inter-container networking not available |

### Other (2 tests — all backends)

| Test | Root Cause |
|------|-----------|
| shells/pwsh | pwsh not available |
| shells/python | python not available |

## Backend-Specific Notes

### Cloud backends (ECS, CloudRun, ACA — container mode)
- Real bash available via reverse agent subprocess in simulator
- CloudRun has slightly more failures (54 vs 56-58) due to container lifecycle differences

### FaaS backends (Lambda, GCF, AZF — function mode)
- Real bash available via reverse agent subprocess in simulator
- Lambda/GCF complete in monolithic mode (~3-5 min)
- AZF requires individual mode (first test hangs in monolithic)
- AZF individual mode: 69 PASS (best of all backends) — real bash + no test interference
