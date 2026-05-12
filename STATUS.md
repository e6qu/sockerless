# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `main` — clean. No phase in flight. |
| In-flight | none. Pick a next item from [DO_NEXT.md](DO_NEXT.md). |
| Last merged | PR #152 — `docs/POD_MATERIALIZATION.md` (2026-05-12). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 987 fixed. |
| Live infra | None up. |

## Invariants

- **Components stay decoupled from admin / UI.** Sims, backends, bleephub run independently via env vars; admin reads only `/v1/health`, `/v1/info`, env. No admin-required env vars on components, no startup registration.
- **No fakes / no fallbacks.** Unknown values fail loud. Operator-requested persistence + auth never silently degrade.
- **Test target gating.** Backend integration tests require `SOCKERLESS_TEST_TARGET=sim|cloud` — no implicit skip.
- **Backend ↔ host primitive must match.** ECS in ECS, Lambda in Lambda, Cloud Run in Cloud Run, GCF in CRF, ACA in ACA, AZF in AZF.
- **specs/CLOUD_RESOURCE_MAPPING.md is authoritative** for "how does sockerless model X on cloud Y".

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-12 | #152 | `docs/POD_MATERIALIZATION.md` — per-backend pod materialization walked through GH + GitLab runners. |
| 2026-05-11 | #151 | Phase 87d closeout + Phase 92 — trace propagation + MeterProvider + runtime metrics + `make stack-observability-validate`; `Backing: gcs-fuse` deregistered on cloudrun + gcf with translator reject pointing at `gcs-sync` (closes BUG-944 + ships BUG-987). |
| 2026-05-10 | #150 | Phase 87c — zerolog → OTel logs bridge across all 12 components. |
| 2026-05-10 | #149 | Phase 91 (consolidated) — Lambda volume_translator framework migration; cloudrun + gcf reject `BackingPDEphemeral`; integration TestMain → ECR Public Gallery. |
| 2026-05-10 | #148 | Phase 91b — `BackingMemory` translator on ECS / ACA / AZF. |
| 2026-05-10 | #147 | Phase 91 — `BackingMemory` translator on cloudrun + gcf. |
| 2026-05-10 | #146 | Phase 87b — component-side OTel SDK wiring across 6 backends + 3 sims + admin. |
| 2026-05-10 | #145 | Phase 87 — observability stack (otel-collector + VictoriaLogs + Jaeger), filelog receiver, `/api/v1/observability`, admin UI deep-link chips. |
| 2026-05-10 | #144 | Phase 86 — health + supervision surface (exit-code capture, `/diagnostics`, `<UnhealthyDiagnosticPanel>`). |
| 2026-05-10 | #143 | Phase 85 — admin config edit + hot reload (`ConfigKeyMeta`, `/reload`, `<ConfigEditModal>`). |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md) and [PLAN.md](PLAN.md) § Closed phases.
