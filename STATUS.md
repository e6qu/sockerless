# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `main` (clean — no in-flight branch) |
| In-flight phases | None — Phase 79 closed in PR #138 (2026-05-10). Phase 80 next. |
| Last merged | PR #138 — Phase 79 (full topology store + REST + lifecycle) + Phase 87 plan + cloud-resource-mapping consolidation (2026-05-10) |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`).

**Next: Phase 80 — admin UI topology page + per-instance lifecycle.** Replace ProjectsPage with project + instance tree; per-instance Start/Stop/Rebuild controls; "Add instance" form (kind + name + port + per-component config); edit/delete; port registry view (allocated + free ranges). All backend pieces are in place — Phase 80 is a pure UI-side build on the existing `/api/v1/topology/*` surface.

After 80: Phases 81–87 (logs+console, cloud-resources rollup, sim-UI parity, per-instance state, config edit, health surface, observability). Full sub-task list in [PLAN.md](PLAN.md).

## After Phase 87

- Phases 91–94 — real per-cloud volume provisioning.
- Live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #138 | Phase 79 complete — `sockerless.yaml` topology store, `TopologyManager` singleton, full CRUD REST surface, `make/components.mk` granular lifecycle targets, port allocator. + Phase 87 plan (OTel+VictoriaLogs+Jaeger Stack A). + `specs/CLOUD_RESOURCE_MAPPING.md` consolidation (Docker/Podman→cloud quick reference, CI runner requirements with explicit ephemeral + dispatcher subsections, multi-system CI/CD comparison). |
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
