# Sockerless — Next Steps

## Current State

Phase 37 complete. bleephub now has in-memory git repositories with smart HTTP protocol, REST + GraphQL CRUD, and full `gh`/`git` CLI compatibility. **332 tasks done across 37 phases.** Next: add organization and team support to bleephub.

## Immediate Priority: Phase 38 — bleephub: Organizations + Teams + RBAC

Organization accounts, team management, and role-based access control. `gh org list` uses GraphQL. Org membership determines repo visibility and write access.

1. **Organization store** — in-memory org CRUD (id, login, name, description, email, members, teams)
2. **Membership** — org owners, members, outside collaborators
3. **Teams** — CRUD, team membership, team repo permissions
4. **RBAC enforcement** — repo visibility, org-level roles, team-level permissions
5. **REST endpoints** — `GET /api/v3/orgs/{org}`, `GET /api/v3/user/orgs`, membership, teams
6. **GraphQL** — `user.organizations` connection, `organization(login)` query
7. **`gh` CLI validation** — `gh org list` works, repo operations respect org permissions

## bleephub Expansion Roadmap

| Phase | What | Key `gh` Commands |
|---|---|---|
| **36** | Users + Auth + GraphQL engine ✓ | `gh auth login`, `gh auth status` |
| **37** | Git repositories ✓ | `gh repo create/view/list/clone`, `git push/pull` |
| **38** | Organizations + teams + RBAC | `gh org list`, org repo permissions |
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
