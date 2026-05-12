# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

`docs-cleanup-actionable` branch / PR #153. Phase 153 (bleephub ↔ GitHub API parity) **in flight on the same branch + PR** at the user's direction. Audit + acceptance criteria: [specs/BLEEPHUB_GITHUB_API_PARITY.md](specs/BLEEPHUB_GITHUB_API_PARITY.md).

## Resume here — Phase 153, remaining sub-tasks

10/13 sub-tasks shipped. CI runs on every push. **Never auto-merge.**

| Sub-task | Status | What |
|---|---|---|
| P153.1 — store + types | ✓ `e87239e0` | Installation suspend/repo-selection, OAuth Apps, gho_/ghu_/ghr_ tokens, Checks store |
| P153.2 — token prefixes + middleware | ✓ `dc3ceb3c` | gho_/ghu_/ghr_ recognised; default mint ghp_ |
| P153.3 — missing Apps endpoints | ✓ `c019df94` | apps/{slug}, suspend, org/user installation, repo-sel mgmt, installation/repositories |
| P153.4 — app webhook config + deliveries | ✓ `bba640b5` | /app/hook/config + /app/hook/deliveries |
| P153.5 — OAuth /applications/{cid}/* | ✓ `fab271b2` | check/reset/revoke token, revoke grant, OAuth app mgmt |
| P153.6 — perm enforcement | ✓ `2fb5e06d` | requirePerm decorator on write endpoints; repo hook redelivery |
| P153.7 — webhook installation field + events | ✓ `d5cfb272` | installation:{id}, X-GitHub-Hook-* headers, installation/installation_repositories events |
| P153.8 — Checks API | ✓ `93d52950` | check-runs + check-suites + annotations |
| P153.9 — HATEOAS url fields | ✓ `5f97511b` | *_url + suspended_at + single_file_name on app/installation JSON |
| P153.10 — UI updates | ✓ `297484f`  | perms/events form, PEM viewer, OAuth Apps tab, suspend/delete |
| **P153.11 — state save + gh CLI tests** | **in flight** | Phase 153 added to bleephub/test/run-gh-test.sh; STATUS/PLAN/DO_NEXT/WHAT_WE_DID updated |
| P153.12 — SQLite persistence | **next** | Mirror sim pattern: `BLEEPHUB_PERSIST=true` gates persistence; SQLite-backed stores for users/tokens/apps/installations/hooks/orgs/teams/issues/PRs. Fail-loud on persistence-open failure (BUG-985 pattern). Git storage stays in-memory for now |
| P153.13 — gh CLI smoke + parity tests | partial | Phase 153 parity probes appended to existing run-gh-test.sh (`make bleephub-gh-test`); real `gh` binary in Docker |

### Concrete next actions on this branch

1. **Wait for CI** to confirm green on `297484f` / `5f97511b`. `gh run list --branch docs-cleanup-actionable -L 1`.
2. **P153.11 final** — commit + push the docs updates (STATUS / DO_NEXT / WHAT_WE_DID) + the new gh-CLI test additions.
3. **P153.12 SQLite persistence**:
   - Read `simulators/aws/shared/server.go::NewServer` for the canonical persistence pattern (env-gated, log.Fatalf on open failure — BUG-985/986).
   - Add `BLEEPHUB_PERSIST=true` + extend `BLEEPHUB_DATA_DIR` to gate SQLite.
   - SQLite-backed implementations of: users + tokens + apps + installations + installation_tokens + user_to_server_tokens + refresh_tokens + oauth_apps + orgs + teams + memberships + repos + issues + labels + milestones + comments + pull_requests + pr_reviews + hooks + hook_deliveries + app_hook_deliveries + check_runs + check_suites + workflow_files + workflows + manifest_codes + auth_codes + device_codes.
   - Git storage (go-git) stays in-memory for now — switching to filesystem.Storage is a separate phase.
   - Tests: regression guard for `BLEEPHUB_PERSIST=true` + bad path → fail loud. Happy path: restart preserves apps/installations/repos/issues.
4. **P153.13 final** — `make bleephub-gh-test` (Docker harness) passes the new Phase 153 parity probes end-to-end. If anything fails, fix in-line.
5. **Open the PR for merge review**. PR #153 already exists. Update its title from `docs:` to something like `phase 153: bleephub ↔ GitHub API parity (+ docs streamline)`.

## Track B (after Phase 153 merges) — Live-cloud validation

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. One branch per cell. Teardown self-sufficient.

## Track C — Phase 91d (bookmarked)

Real `pd-ephemeral` lifecycle on cloudrun + gcf. Cloud Run lacks the protobuf field. Don't reopen until cloud capability changes.

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub.
- **No fallbacks.** Unknown config values fail-loud.
- **GitHub Apps + OAuth Apps are separate concepts.** Distinct entities, distinct token prefixes.
- **Installation tokens are immutable snapshots.** Re-mint to pick up perm changes.
- **CI green per commit.** Each commit independently testable.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud`.
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.
- **Never auto-merge.** Push the PR, wait for user.
- **Single-branch rule.** All in-flight work lands on one branch per phase.

## Session-resume checklist

1. `git status` + `git log --oneline -5 origin/main` to confirm where main is.
2. `git checkout docs-cleanup-actionable && git pull` if continuing Phase 153.
3. Read STATUS.md (snapshot) + this file (concrete actions).
4. `cd bleephub && go test ./...` to establish green before changes.
5. File BUGS.md entries for anything that surfaces, fix in the same session.
6. State-save before pushing: STATUS.md, this file, WHAT_WE_DID.md, MEMORY.md.
