# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `main` (clean — no in-flight branch) |
| In-flight phases | None — Phase 80 closed in PR #139 (2026-05-10). Phase 81 next. |
| Last merged | PR #139 — Phase 80 admin UI Topology page + state save post-#138 (2026-05-10) |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`).

**Next: Phase 81 — per-instance logs + live troubleshooting console.** Live log tail per instance via SSE from admin (reads `.stack-pids/<name>.log`); combined-timeline view (sim + backend interleaved); API console panel (arbitrary HTTP requests against an instance, inspect request/response).

After 81: Phases 82–87 (cloud-resources rollup, sim-UI parity, per-instance state, config edit, health surface, observability). Full sub-task list in [PLAN.md](PLAN.md).

## After Phase 87

- Phases 91–94 — real per-cloud volume provisioning.
- Live-cloud validation track (Lambda live, Cloud Run Services / ACA Apps live, AZF cloud-dns live, Lambda service-mesh live, ACA/AZF Azure AD live).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #139 | Phase 80 complete — admin UI Topology page (`/ui/topology`): project + instance tree, per-instance status polling, Start/Stop/Rebuild, per-kind add/edit instance modal, add/delete project, auto-allocate port, port registry. Replaced legacy ProjectsPage + ProjectCreatePage. |
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
