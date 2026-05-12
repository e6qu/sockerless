# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `docs-cleanup-actionable` — docs streamline + **Phase 153 bleephub↔GitHub API parity** in flight on the same branch + PR #153. |
| In-flight | Phase 153 — sub-tasks P153.1 → P153.10 shipped, P153.11 (state save + gh CLI tests) in progress; P153.12 (SQLite persistence) pending. |
| Last merged | PR #152 — `docs/POD_MATERIALIZATION.md` (2026-05-12). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 987 fixed. |
| Live infra | None up. |

## Invariants

- **Components stay decoupled from admin / UI.** Sims, backends, bleephub run independently via env vars; admin reads only `/v1/health`, `/v1/info`, env. No admin-required env vars on components, no startup registration.
- **No fakes / no fallbacks.** Unknown values fail loud. Operator-requested persistence + auth never silently degrade.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` — no implicit skip.
- **Backend ↔ host primitive must match.** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **GitHub Apps + OAuth Apps are separate concepts.** Distinct store entries, distinct token prefixes (`gho_` vs `ghu_`), parallel surfaces on `/applications/{client_id}/...`.
- **specs/CLOUD_RESOURCE_MAPPING.md is authoritative** for "how does sockerless model X on cloud Y".

## Phase 153 progress (`docs-cleanup-actionable` branch / PR #153)

Per-sub-task commits on the branch:

| Sub-task | Commit | What |
|---|---|---|
| P153.1 | `e87239e0` | Store + types: Installation suspend/repo-selection, OAuth Apps, UserToServerToken (gho_/ghu_/ghr_), Checks API store |
| P153.2 | `dc3ceb3c` | Middleware recognises gho_/ghu_/ghr_; default mint switches bph_ → ghp_ |
| P153.3 | `c019df94` | apps/{slug}, suspend/unsuspend, orgs/{org}/installation, users/{u}/installation, repo-selection mgmt, installation/repositories |
| P153.4 | `bba640b5` | App-level webhook config + deliveries (`/app/hook/config` + `/app/hook/deliveries`) |
| P153.5 | `fab271b2` | `/applications/{client_id}/token` family + OAuth App management endpoints |
| P153.6 | `2fb5e06d` | `requirePerm(scope, level)` decorator gates write-class endpoints + repo hook redelivery |
| P153.7 | `d5cfb272` | `installation:{id}` payload field + X-GitHub-Hook-* headers + X-Hub-Signature SHA1 + installation/installation_repositories events |
| P153.8 | `93d52950` | Checks API (check-runs + check-suites + annotations) |
| P153.9 | `5f97511b` | HATEOAS `*_url` fields on appToJSON + installationToJSON (suspended_at, single_file_name, …) |
| P153.10 | `297484f`  | UI: permissions/events form, PEM + secrets viewer, OAuth Apps tab, suspend/delete buttons |
| P153.11 | (this) | Phase 153 added to gh CLI test script + state save |
| P153.12 | pending | SQLite persistence for bleephub state |
| P153.13 | partial | gh CLI parity tests integrated into existing test script (real gh binary in Docker) |

CI runs after each push on PR #153. Never auto-merge — user merges.

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-12 | #152 | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |
| 2026-05-11 | #151 | Phase 87d closeout + Phase 92 — trace propagation + MeterProvider + runtime metrics; `Backing: gcs-fuse` deregistered. |
| 2026-05-10 | #150 | Phase 87c — zerolog → OTel logs bridge across all 12 components. |
| 2026-05-10 | #149 | Phase 91 (consolidated) — Lambda volume_translator framework migration; cloudrun + gcf reject `BackingPDEphemeral`; ECR Public Gallery. |
| 2026-05-10 | #148 | Phase 91b — `BackingMemory` translator on ECS / ACA / AZF. |
| 2026-05-10 | #147 | Phase 91 — `BackingMemory` translator on cloudrun + gcf. |
| 2026-05-10 | #146 | Phase 87b — component-side OTel SDK wiring across 6 backends + 3 sims + admin. |
| 2026-05-10 | #145 | Phase 87 — observability stack (otel-collector + VictoriaLogs + Jaeger). |
| 2026-05-10 | #144 | Phase 86 — health + supervision surface. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md) and [PLAN.md](PLAN.md) § Closed phases.
