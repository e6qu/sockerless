# Sockerless — Next Steps

## Current State

Phases 1-67, 69-71 complete. **622+ tasks done across 71 phases.** All three simulators have real process execution, non-trivial arithmetic evaluator tests (27 new), and full SDK/CLI/Terraform validation.

Cloud SDK: AWS 42, GCP 43, Azure 38 | Cloud CLI: AWS 26, GCP 21, Azure 19

## Next Phase: Phase 68 — Multi-Tenant Backend Pools (In Progress)

Named pools of backends with scheduling and resource limits. Each pool has a backend type, concurrency limit, and queue. Requests are routed by label or default.

P68-001 (pool config types/validation/loader) is done.

| Task | Description | Status |
|---|---|---|
| P68-001 | Pool configuration — types, validation, loader | ✅ |
| P68-002 | Pool registry — in-memory, each with own BaseServer + Store | pending |
| P68-003 | Request router — route by label (`com.sockerless.pool`) or default | pending |
| P68-004 | Concurrency limiter — per-pool semaphore, 429 on overflow | pending |
| P68-005 | Pool lifecycle — create/destroy at runtime via management API | pending |
| P68-006 | Pool metrics — per-pool counts on `/internal/metrics` | pending |
| P68-007 | Round-robin scheduling — multi-backend pools | pending |
| P68-008 | Resource limits — per-pool max containers, max memory | pending |
| P68-009 | Unit + integration tests | pending |
| P68-010 | Save final state | pending |

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

# Simulator CLI tests (per cloud, requires aws/gcloud/az CLIs)
cd simulators/aws/cli-tests && GOWORK=off go test -v -count=1
cd simulators/gcp/cli-tests && GOWORK=off go test -v -count=1
cd simulators/azure/cli-tests && GOWORK=off go test -v -count=1

# Arithmetic evaluator tests only
cd simulators/aws/sdk-tests && GOWORK=off go test -v -run Arithmetic -count=1
cd simulators/gcp/sdk-tests && GOWORK=off go test -v -run Arithmetic -count=1
cd simulators/azure/sdk-tests && GOWORK=off go test -v -run Arithmetic -count=1

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
