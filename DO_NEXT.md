# Sockerless — Next Steps

## Current State

Phases 1-67, 69-72 complete. **637+ tasks done across 72 phases.** Phase 72 (Full-Stack E2E Tests) complete — all 4 milestones done. sim-test-all: 75 PASS. Central test-e2e: 65 PASS.

## Next: Phase 68 — Multi-Tenant Backend Pools (Resume)

P68-001 (pool config types/validation/loader) is done. Remaining: P68-002 through P68-010.

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
