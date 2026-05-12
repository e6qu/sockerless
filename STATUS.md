# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `docs-cleanup-actionable` — docs streamline + **Phase 153 bleephub↔GitHub API parity** + **bleephub SQLite persistence** + **real `gh` CLI compatibility** all in flight on the same branch + PR #153. |
| In-flight | Phase 153 — P153.1 → P153.12 shipped (12 commits). P153.13 (real `gh` CLI Docker harness + `gh repo create` / `gh issue create` end-to-end) in progress. |
| Last merged | PR #152 — `docs/POD_MATERIALIZATION.md` (2026-05-12). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 987 fixed. |
| Live infra | None up. |

## Invariants

- **Components stay decoupled from admin / UI.** Sims, backends, bleephub run independently via env vars; admin reads only `/v1/health`, `/v1/info`, env. No admin-required env vars on components, no startup registration.
- **Bleephub is maximally compatible with the GitHub REST/GraphQL API.** If GitHub accepts it, bleephub accepts it — including string-coerced booleans/integers in JSON bodies (what `gh api -f key=value` sends). This is the GitHub API spec; not a fallback.
- **The `gh` CLI must work directly against bleephub.** Tests use real `gh repo create` / `gh issue create` / `gh pr create` against the running bleephub, not `gh api` URL hackery.
- **GitHub Apps and OAuth Apps are separate concepts.** Distinct store entries, distinct token prefixes (`gho_` vs `ghu_` vs `ghs_` vs `ghr_` vs `ghp_`), parallel surfaces on `/applications/{client_id}/...`.
- **Installation tokens are immutable snapshots.** Bumping installation perms post-mint does NOT auto-upgrade existing tokens (mirrors real GH).
- **Persistence is opt-in + fail-loud.** `BLEEPHUB_PERSIST=true` enables SQLite. Open-failure `log.Fatalf`s (BUG-985/986 pattern); never silently falls back to in-memory.
- **No fakes / no fallbacks.** Unknown values fail loud. Operator-requested persistence + auth never silently degrade.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` — no implicit skip.
- **Backend ↔ host primitive must match.** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **specs/CLOUD_RESOURCE_MAPPING.md is authoritative** for "how does sockerless model X on cloud Y".

## Phase 153 progress (`docs-cleanup-actionable` branch / PR #153)

12 commits shipped. Per-sub-task:

| Sub-task | Commit | What |
|---|---|---|
| P153.1 | `e87239e` | Store + types: Installation suspend/repo-selection, OAuth Apps, UserToServerToken (gho_/ghu_/ghr_), Checks API store |
| P153.2 | `dc3ceb3` | Middleware recognises gho_/ghu_/ghr_; default mint switches bph_ → ghp_ |
| P153.3 | `c019df9` | apps/{slug}, suspend/unsuspend, orgs/{org}/installation, users/{u}/installation, repo-selection mgmt, installation/repositories |
| P153.4 | `bba640b` | App-level webhook config + deliveries (`/app/hook/config` + `/app/hook/deliveries`) |
| P153.5 | `fab271b` | `/applications/{client_id}/token` family + OAuth App management endpoints |
| P153.6 | `2fb5e06` | `requirePerm(scope, level)` decorator gates write-class endpoints; repo hook redelivery |
| P153.7 | `d5cfb27` | `installation:{id}` payload field + X-GitHub-Hook-* headers + X-Hub-Signature SHA1 + installation/installation_repositories events |
| P153.8 | `93d5295` | Checks API (check-runs + check-suites + annotations) |
| P153.9 | `5f97511` | HATEOAS `*_url` fields on appToJSON + installationToJSON |
| P153.10 | `297484f` | UI: permissions/events form, PEM + secrets viewer, OAuth Apps tab, suspend/delete |
| P153.11 | `c586b18` | Phase 153 added to gh CLI test script + state save |
| P153.12 | `192c627` | SQLite persistence — KV-style table, 9 buckets persisted, fail-loud on open |
| **P153.13** | **in flight** | Real `gh` CLI Docker harness wired (`make bleephub-gh-docker-test`); GitHub-spec request body tolerance (string-coerced bools/ints — what `gh api -f` sends); `gh repo create` / `gh issue create` / `gh pr create` / `gh release create` end-to-end against bleephub |

CI runs after each push on PR #153. Two consecutive green CI runs on `297484f` and `192c627`. Never auto-merge — user merges.

## Recently shipped (older PRs in WHAT_WE_DID.md)

| Date | PR | Headline |
|---|---|---|
| 2026-05-12 | #152 | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |
| 2026-05-11 | #151 | Phase 87d closeout + Phase 92 — trace propagation + MeterProvider + runtime metrics; `Backing: gcs-fuse` deregistered. |
| 2026-05-10 | #150 | Phase 87c — zerolog → OTel logs bridge across all 12 components. |
| 2026-05-10 | #149 | Phase 91 (consolidated) — Lambda volume_translator framework migration; cloudrun + gcf reject `BackingPDEphemeral`. |
| 2026-05-10 | #148 | Phase 91b — `BackingMemory` translator on ECS / ACA / AZF. |
| 2026-05-10 | #147 | Phase 91 — `BackingMemory` translator on cloudrun + gcf. |
| 2026-05-10 | #146 | Phase 87b — component-side OTel SDK wiring. |
| 2026-05-10 | #145 | Phase 87 — observability stack. |
