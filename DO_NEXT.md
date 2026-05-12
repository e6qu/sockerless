# Do Next

Status [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Where we are

`docs-cleanup-actionable` branch / PR #153. Phase 153 (bleephub ↔ GitHub API parity + SQLite persistence + real `gh` CLI compatibility) in flight on the same branch. **12 of 13 sub-tasks shipped**, P153.13 final stretch. Audit + acceptance criteria: [specs/BLEEPHUB_GITHUB_API_PARITY.md](specs/BLEEPHUB_GITHUB_API_PARITY.md).

## Resume here — Phase 153 closeout

Phase 153 is nearly done. 13/13 sub-tasks shipped, **one open bug** blocking final acceptance:

### BUG-989 — `gh issue view` Issue|PullRequest union

`gh issue view <N> --repo o/r` exits non-zero because bleephub's GraphQL `repository.issueOrPullRequest` returns just `Issue`, not a union of `Issue | PullRequest`. gh CLI's query uses `...on Issue` + `...on PullRequest` fragments which fail to type-check on a non-union return.

**Fix steps** (single commit on the same branch / PR #153):

1. Read `bleephub/gh_issues_graphql.go:475` (`repoType.AddFieldConfig("issueOrPullRequest", ...)`).
2. Read `bleephub/gh_pulls_graphql.go:242` (the `PullRequest` type definition).
3. Build a `graphql.NewUnion("IssueOrPullRequest", []*graphql.Object{issueType, pullRequestType}, ResolveType: ...)`.
4. Switch `issueOrPullRequest` field's `Type:` from `issueType` to the new union.
5. The Resolver must return either an Issue map or a PR map; pick by looking up `s.store.GetIssueByNumber` then falling through to `s.store.GetPullRequestByNumber`.
6. Add the IssueComment-equivalent fields to whatever PR comment type gh hits (`reactionGroups`, `includesCreatedEdit`, `isMinimized`, `minimizedReason`) — apply the same `alwaysFalse` / `alwaysNil` / `emptyList` stub pattern from gh_issues_graphql.go.
7. Add `last` arg to `PullRequest.comments` connection.
8. Add `nodes` to `PRCommentConnection` (gh_pulls_graphql.go:207).
9. Re-run `make bleephub-gh-docker-test` — expect 50/50 PASS.

After BUG-989 fixed:

- Verify `go test ./...` green in bleephub/.
- Update STATUS.md / DO_NEXT.md / BUGS.md to mark BUG-989 fixed.
- Wait for user merge. **Never auto-merge.**

## Resumable tracks after Phase 153 merges

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
