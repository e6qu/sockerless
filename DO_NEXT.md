# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

PR #153 (`docs-cleanup-actionable` branch) — Phase 153 (bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compatibility) **complete**. 13/13 sub-tasks shipped. Docker harness 50/50 PASS via `make bleephub-gh-docker-test`. Awaiting CI green.

## ⚠ One-time merge authorization (user-granted 2026-05-12)

User explicitly authorized auto-merge for **PR #153 and the follow-up PRs #154, #155, #156** once CI passes on each. This overrides the default "never auto-merge" invariant for these four PRs only — it does NOT extend to future PRs. After #156 merges, the default rule resumes.

For each:
1. Push the final commit on the phase branch.
2. Wait for `gh pr checks <N>` to show every required check green.
3. If a check fails, **fix the underlying issue** (do not bend the test). Push the fix. Re-poll.
4. Once all checks pass: `gh pr merge <N> --squash --delete-branch` (or `--merge` if branch history matters — verify the repo's merge style first).
5. `git checkout main && git pull --ff-only` to sync local main.
6. Branch for the next phase off the refreshed main.

## Resume here — after Phase 153 merges

Phase 153 done. Next phase in queue:

## Resumable tracks after Phase 153 merges

### Track A2 — Phase 154 — Broad GitHub API sweep (next phase)

User-requested follow-up after Phase 153 closes. Audit-first sweep of GitHub API surfaces gh CLI / octokit / probot hit. Spec lives in PLAN.md § "Phase 154 — Broad GitHub API sweep (planned, post-153)". Fifteen surface areas:

1. GitHub Apps (deeper) — installation_target, github_app_authorization, new_permissions_accepted, marketplace stub.
2. Orgs — members, teams (parent teams, IdP sync), audit log, security manager, org secrets/variables, dep graph.
3. Installing apps in orgs — org install URL flow, repository_selection at install time.
4. OIDC — Actions OIDC tokens + JWKS + claims (sub, audience, environment).
5. Webhooks (extras) — org-level, enterprise-level, meta / security_advisory / secret_scanning_alert.
6. Pipelines + jobs API — full Actions REST: runs/jobs/steps, logs download, artifacts download, rerun/cancel/attempts.
7. Triggering pipelines — workflow_dispatch validation, repository_dispatch with client_payload.
8. Users API — followers/following/blocked, emails, gpg/ssh keys, status, sponsorship.
9. Groups (teams + IdP) — team membership, IdP group sync surface.
10. SSO integration — SAML SSO header on PATs, SCIM 2.0 provisioning, enforced-SSO redirects.
11. GitHub Pages — `/repos/{o}/{r}/pages` + builds + deployments.
12. Deployments + Environments — full deployments API + Environments (protection rules, reviewers, secrets, branch policies), branch protection.
13. Issue + PR comments depth — issue comments full CRUD + reactions; PR review comments (inline / file-line / range), review threads, resolve+reopen, suggested-changes, comment edit history.
14. Reactions API — full eight reaction types + reaction groups on Issue/PR/comments with real counts.
15. Webhook events parity — full event coverage (branch_protection_rule, check_run, code_scanning_alert, deployment, discussion, label, milestone, project, release, status, watch, workflow_run, etc.). Per-event payload shape verified against real GH samples.

Branch: `phase-154-github-api-sweep` (off `origin/main` once #153 merges). Audit-first: file per-surface gap tickets, prioritize by gh CLI hit-rate.

### Track A3 — Phase 155 / 156 — Docs refresh

User-requested after Phase 154 closes:

- **Phase 155** — bleephub-specific docs: bleephub/README.md, specs/BLEEPHUB_GITHUB_API_PARITY.md, docs/RUNNERS.md § GitHub runner contract, docs/runner-capability-matrix.md, ARCHITECTURE.md (bleephub block), ui/packages/bleephub/README.md, new docs/BLEEPHUB_GH_CLI.md.
- **Phase 156** — project-wide docs: root README, ARCHITECTURE.md, specs/CLOUD_RESOURCE_MAPPING.md, docs/OBSERVABILITY.md, docs/ADMIN_ORCHESTRATION.md, docs/MAKEFILE_STANDARD.md, docs/POD_MATERIALIZATION.md, runner docs, ECS / Lambda design docs.

Acceptance: every claim verified; CLI examples copy-paste-run against current `main`.

### Track B — Live-cloud validation

Lambda live · Cloud Run Services + ACA Apps live · AZF cloud-dns live · Lambda service-mesh live · ACA/AZF Azure AD live. One branch per cell. Teardown self-sufficient.

### Track C — Phase 91d (bookmarked indefinitely)

Real `pd-ephemeral` lifecycle on cloudrun + gcf. Cloud Run lacks the protobuf field. Don't reopen until cloud capability changes.

### Track D — Bleephub persistence expansion

Phase 153 ships SQLite for users / tokens / apps / oauth_apps / installations / installation_tokens / user_to_server_tokens / refresh_tokens / repos. Extending to issues / PRs / hooks / hook deliveries / check_runs / check_suites / labels / milestones / comments / secrets / orgs / teams / memberships is a separate phase once a real use case surfaces. Git storage (go-git) → `filesystem.Storage` is its own phase.

## Per-sub-task status (Phase 153, in-flight)

| Sub-task | Status | Commit / Notes |
|---|---|---|
| P153.1 — store + types | ✓ | `e87239e` |
| P153.2 — token prefixes + middleware | ✓ | `dc3ceb3` |
| P153.3 — missing Apps endpoints | ✓ | `c019df9` |
| P153.4 — app webhook config + deliveries | ✓ | `bba640b` |
| P153.5 — OAuth /applications/{cid}/* | ✓ | `fab271b` |
| P153.6 — perm enforcement | ✓ | `2fb5e06` |
| P153.7 — webhook installation field + events | ✓ | `d5cfb27` |
| P153.8 — Checks API | ✓ | `93d5295` |
| P153.9 — HATEOAS url fields | ✓ | `5f97511` |
| P153.10 — UI updates | ✓ | `297484f` |
| P153.11 — state save + gh-CLI probes | ✓ | `c586b18` |
| P153.12 — SQLite persistence | ✓ | `192c627` |
| P153.13 — real gh CLI compatibility | mostly shipped | `b538d5c` + `dfdf3db`. Native `gh repo create / view / list` + `gh issue create / list` pass. `gh issue view` still fails (BUG-989 — Issue\|PullRequest union missing). |

## Invariants (re-state on every commit)

- **Components stay decoupled.** No admin-required env vars on sims/backends/bleephub.
- **Maximally compatible with real GitHub.** Bleephub accepts everything real GitHub accepts; no fallbacks for legacy bleephub behavior.
- **The `gh` CLI works directly against bleephub.** No URL-hackery shims in production code; tests use real commands.
- **No fallbacks.** Unknown config values fail-loud.
- **GitHub Apps + OAuth Apps are separate concepts.** Distinct entities, distinct token prefixes.
- **Installation tokens are immutable snapshots.** Re-mint to pick up perm changes.
- **Persistence is opt-in + fail-loud.** `log.Fatalf` on persistence-open failure.
- **CI green per commit.** Each commit independently testable.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud`.
- **No docs-only PRs.** Pair docs updates with implementation work on the same branch / PR.
- **Never auto-merge.** Push the PR, wait for user.
- **Single-branch rule.** All in-flight work lands on one branch per phase.

## Session-resume checklist

1. `git fetch origin && git checkout docs-cleanup-actionable && git pull` to land on the in-flight branch.
2. `git log --oneline -10` to see what's already shipped on this branch.
3. Read STATUS.md (snapshot) + this file (concrete next actions).
4. `cd bleephub && go test ./...` to confirm green baseline.
5. `make bleephub-gh-docker-test` to confirm Docker harness still passes (assumes Docker daemon).
6. Resume P153.13 sweep + commit + push.
7. File BUGS.md entries for anything that surfaces; fix in the same session.
8. State-save before pushing: STATUS.md, this file, WHAT_WE_DID.md, MEMORY.md.
