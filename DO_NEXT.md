# Sockerless — Next Steps

## Current State

Phase 38 complete. Documentation overhauled (DEPLOYMENT.md removed, ARCHITECTURE.md rewritten, production milestones added). **342 tasks done across 38 phases.** Next: add issue tracking to bleephub.

## Immediate Priority: Phase 39 — bleephub: Issues + Labels + Milestones

Full issue tracking. `gh` uses GraphQL exclusively for issue CRUD and listing.

1. **Issue store** — per-repo sequential numbering, state (OPEN/CLOSED), stateReason
2. **Labels** — per-repo label CRUD, issue-label association
3. **Milestones** — per-repo milestone CRUD
4. **REST endpoints** — issue CRUD, comments, filtering by state/assignee/label
5. **GraphQL** — `createIssue`, `closeIssue` mutations, `repository.issues` connection, `search(type: ISSUE)`
6. **Reactions** — basic emoji reactions on issues and comments
7. **`gh` CLI validation** — `gh issue create/list/view/close` work against bleephub

## bleephub Expansion Roadmap

| Phase | What | Key `gh` Commands |
|---|---|---|
| **36** | Users + Auth + GraphQL engine ✓ | `gh auth login`, `gh auth status` |
| **37** | Git repositories ✓ | `gh repo create/view/list/clone`, `git push/pull` |
| **38** | Organizations + teams + RBAC ✓ | `gh org list`, org repo permissions |
| **39** | Issues + labels + milestones | `gh issue create/list/view/close` |
| **40** | Pull requests (create, review, merge) | `gh pr create/list/view/merge/close` |
| **41** | API conformance + `gh` CLI test suite | OpenAPI spec validation, full test suite |
| **42** | Runner enhancements (actions, multi-job) | `uses:` actions, matrix, artifacts |

## After bleephub

| Phase | What | Why |
|---|---|---|
| 43 | Cloud resource tracking | Tag/track all cloud resources, leak detection, cleanup |
| 44 | Crash-only software | Safe to crash at any point, startup = recovery |
| 45 | Upstream test expansion | More external validation |
| 46 | Capability negotiation | Quality of life |

## Production Phases

| Phase | What | Why |
|---|---|---|
| 47 | Production Docker API | `docker run`, Compose, TestContainers, SDK, DOCKER_HOST modes (TCP/SSH) |
| 48 | Production GitHub Actions | Self-hosted runner + github.com on real cloud |
| 49 | Production GitLab CI | gitlab-runner + gitlab.com on real cloud |
| 50 | Docker API hardening | Fix gaps found during production validation |
| 51 | Production operations | Monitoring, alerting, security, TLS, upgrades |

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
