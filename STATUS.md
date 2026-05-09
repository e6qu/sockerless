# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-79-topology-store` |
| In-flight phases | Phase 79 step 2 (sockerless.yaml topology store) — first commit on this branch |
| Last merged | PR #137 — Phase 78 UI polish + Phase 79 step 1 (Instance type) (2026-05-10) |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`).

- ✓ Step 1: `Instance` type + per-kind validate + legacy derivation (in PR #137, merged).
- ⏳ Step 2: `sockerless.yaml` topology store (single file at repo root, `projects[]` × `instances[]`). **← current commit.**
- ⏳ Step 3: REST endpoints (`/v1/admin/topology`, `/v1/admin/instances/{key}/{start|stop|rebuild}`).
- ⏳ Step 4: `make start-component` / `stop-component` / `rebuild-component` granular targets; existing `stack-X-Y` become wrappers.
- ⏳ Step 5: Free-port helper + auto-allocation from `ports.ranges`.
- ⏳ Step 6: One-shot migration of existing per-project JSONs into `sockerless.yaml`.

After 79: Phases 80–87 (admin UI, logs+console, cloud-resources rollup, sim-UI parity, per-instance state, config edit, health surface, observability). Full sub-task list in [PLAN.md](PLAN.md).

## After Phase 87

- Phases 91–94 — real per-cloud volume provisioning.
- Live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #137 | Phase 78 UI polish (dark mode toggle, Toast/InlineError, Modal + ContainerDetail, a11y, perf, READMEs) + Phase 79 step 1 (Instance type for admin orchestration). |
| 2026-05-10 | #136 | Phase 121b finish — driver consolidation, host-aliases everywhere, AZF/Lambda DNS, Azure AD access. |
| 2026-05-09 | #135 | Phase 121b initial — Azure sim hardening + harness restructure + driver consolidation + GCP sim Cloud Run invoke routing + envelope parsing + label round-trip. |
| 2026-05-09 | #134 | Phase 127 storage driver expansion (pd/efs/azure-files-ephemeral). |
| 2026-05-09 | #133 | Phase 126 Access driver. |
| 2026-05-09 | #132 | Phase 125 DNS driver. |
| 2026-05-09 | #131 | Phase 124 network discovery driver. |
| 2026-05-09 | #130 | Phase 128 runner job timeout. |
| 2026-05-09 | #129 | Phase 135 sim host model + native arm64 CI. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).
