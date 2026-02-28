# Sockerless — Next Steps

## Current State

Phase 70 (Simulator Fidelity) nearly complete. **578+ tasks done across 69 phases.** All three simulators now execute real processes with real exit codes and real log output, validated by 10 new SDK tests.

## Immediate: P70-024 — CI Integration for Simulator Smoke Tests

Add the SDK smoke tests (including new real-execution tests) to CI so they run on every push/PR. Either extend `test-comprehensive.yml` or add them to the `ci.yml` test matrix. The smoke tests are self-contained Go test files in `simulators/{cloud}/sdk-tests/`.

## Next Phase: Phase 68 — Multi-Tenant Backend Pools

Named pools of backends with scheduling and resource limits. Each pool has a backend type, concurrency limit, and queue. Requests are routed by label or default.

| Task | Description |
|---|---|
| P68-001 | Pool configuration — YAML/JSON config |
| P68-002 | Pool registry — in-memory, each with own BaseServer + Store |
| P68-003 | Request router — route by label (`com.sockerless.pool`) or default |
| P68-004 | Concurrency limiter — per-pool semaphore, 429 on overflow |
| P68-005 | Pool lifecycle — create/destroy at runtime via management API |
| P68-006 | Pool metrics — per-pool counts on `/internal/metrics` |
| P68-007 | Round-robin scheduling — multi-backend pools |
| P68-008 | Resource limits — per-pool max containers, max memory |
| P68-009 | Unit + integration tests |
| P68-010 | Save final state |

## Test Commands Reference

```bash
# Unit + integration
make test

# Lint all 15 modules
make lint

# Simulator SDK tests (per cloud)
cd simulators/aws/sdk-tests && GOWORK=off go test -v -count=1
cd simulators/gcp/sdk-tests && GOWORK=off go test -v -count=1
cd simulators/azure/sdk-tests && GOWORK=off go test -v -count=1

# Shared ProcessRunner tests (per cloud)
cd simulators/aws/shared && GOWORK=off go test -run TestStartProcess -v

# Simulator-backend integration (all backends)
make sim-test-all

# E2E GitHub / GitLab
make e2e-github-memory
make e2e-gitlab-memory

# Upstream act / gitlab-ci-local
make upstream-test-act
make upstream-test-gitlab-ci-local-memory

# bleephub / gitlabhub
make bleephub-test
cd bleephub && go test -v ./...
cd gitlabhub && go test -v ./...

# Terraform integration
make tf-int-test-all
```
