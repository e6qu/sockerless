# Sockerless — Next Steps

## Current State

Phase 43 complete. **382 tasks done across 43 phases.** All cloud resources are now tagged, tracked, and recoverable. Shared tag builder with 3 output formats, resource registry with REST endpoints, crash recovery via CloudScanner interface for all 6 backends. 11 new unit tests.

## Immediate Priority: Phase 44 — Crash-Only Software

Make Sockerless a "crash-only" system — always safe to crash and restart.

1. **Research crash-only software** — Candea & Fox (2003), recovery-oriented computing
2. **Persistent resource registry** — WAL or append-only file (build on Phase 43 registry)
3. **Idempotent operations** — audit all backend operations for replay safety
4. **Startup recovery** — replay registry + scan cloud + reconcile (no separate "clean start" path)
5. **Session recovery** — CI runner reconnection after restart
6. **Remove clean shutdown paths** — SIGTERM = immediate exit, startup always assumes crash
7. **Chaos testing** — kill at random points, restart, verify correctness

## bleephub Expansion Roadmap

| Phase | What | Status |
|---|---|---|
| **36** | Users + Auth + GraphQL engine | ✓ |
| **37** | Git repositories | ✓ |
| **38** | Organizations + teams + RBAC | ✓ |
| **39** | Issues + labels + milestones | ✓ |
| **40** | Pull requests (create, review, merge) | ✓ |
| **41** | API conformance + `gh` CLI test suite | ✓ |
| **42** | Runner enhancements (actions, multi-job, matrix, artifacts) | ✓ |

## After Phase 44

| Phase | What | Why |
|---|---|---|
| 44 | Crash-only software | Safe to crash at any point, startup = recovery |
| 45 | Upstream test expansion | More external validation |
| 46 | ~~Capability negotiation~~ | CANCELLED — all backends should support all tests |

## Production Phases

| Phase | What | Why |
|---|---|---|
| 47 | Production Docker API | `docker run`, Compose, TestContainers, SDK, DOCKER_HOST modes (TCP/SSH) |
| 48 | Production networking + build + streaming | Multi-container, `docker build`, log streaming |
| 49 | Production Compose + TestContainers + SDK | Higher-level Docker clients on real cloud |
| 50 | Production GitHub Actions | Self-hosted runner + github.com on real cloud |
| 51 | Production GitHub Actions scaling | Multi-job, concurrency, validation matrix |
| 52 | Production GitLab CI | gitlab-runner + gitlab.com on real cloud |
| 53 | Production GitLab CI advanced | Multi-stage, DinD, autoscaling |
| 54 | Docker API hardening | Fix gaps found during production validation |
| 55 | Production operations | Monitoring, alerting, security, TLS, upgrades |

## Test Commands Reference

```bash
# Unit + integration
make test

# Lint all 14 modules
make lint

# Simulator-backend integration (all backends)
make sim-test-all

# E2E GitHub (per cloud)
make e2e-github-memory

# E2E GitLab (per cloud)
make e2e-gitlab-memory

# Upstream act / gitlab-ci-local
make upstream-test-act
make upstream-test-gitlab-ci-local-memory

# bleephub (official GitHub Actions runner)
make bleephub-test

# bleephub unit tests
cd bleephub && go test -v ./...

# Terraform integration
make tf-int-test-all
```
