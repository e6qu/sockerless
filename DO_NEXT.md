# Sockerless — Next Steps

## Current State

Phase 47 complete. **409 tasks done across 47 phases.** The `sockerless` CLI manages named backend contexts (`~/.sockerless/contexts/{name}/config.json`). All 8 backends load context env vars at startup (env vars override context values).

## Immediate Priority: Phase 48 — CI Service Container Integration

Wire pod context into CI service container flows. bleephub `services:` parsing, GitLab CI compatibility, health checks, E2E tests.

## After Phase 44

| Phase | What | Status |
|---|---|---|
| 44 | Crash-only software | ✓ Safe to crash at any point, startup = recovery |
| 45 | Podman Pod API + core pod abstraction | ✓ PodContext, PodRegistry, /libpod/pods/*, implicit grouping |
| 46 | Cloud backend multi-container specs | ✓ Deferred start, ECS/CloudRun/ACA multi-container specs, FaaS rejection |
| 47 | sockerless CLI + context management | ✓ CLI module, core context loader, wired into all 8 backends |
| 48 | CI service container integration | bleephub services:, GitLab CI services, health checks, E2E |
| 49 | Upstream test expansion | More external validation |

## Production Phases

| Phase | What | Why |
|---|---|---|
| 50 | Production Docker API | `docker run`, DOCKER_HOST modes (TCP/SSH), registry auth |
| 51 | Production networking + build + streaming | DNS, `docker build`, log streaming |
| 52 | Production Compose + TestContainers + SDK | Higher-level Docker clients on real cloud |
| 53 | Production GitHub Actions | Self-hosted runner + github.com on real cloud |
| 54 | Production GitHub Actions scaling | Multi-job, concurrency, validation matrix |
| 55 | Production GitLab CI | gitlab-runner + gitlab.com on real cloud |
| 56 | Production GitLab CI advanced | Multi-stage, DinD, autoscaling |
| 57 | Docker API hardening | Fix gaps found during production validation |
| 58 | Production operations | Monitoring, alerting, security, TLS, upgrades |

## Future Crash-Only Enhancements (Deferred)

These were scoped out of Phase 44 for future phases:
- **WAL/append-only log** — more robust than JSON overwrite for crash safety
- **Session recovery** — CI runner reconnection after restart
- **Idempotency audit** — ensure all backend operations are replay-safe
- **Chaos testing harness** — kill at random points, restart, verify correctness
- **Operation deduplication** — prevent duplicate cloud resource creation on replay

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
