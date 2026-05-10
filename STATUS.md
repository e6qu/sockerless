# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-85-config-edit-hot-reload` — Phase 85 work in flight (2 implementation commits + state save). |
| In-flight | Phase 85 — admin config edit + hot reload. Curated key metadata, PUT /config endpoint with classification, POST /reload endpoint + make reload-component target, ConfigEditModal UI with hot/restart badges. |
| Last merged | PR #142 — Phase 84 + BUG-985 + BUG-986 (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 986 fixed. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Phase 85 — in flight on `phase-85-config-edit-hot-reload`

Two implementation commits + state save:

1. `phase 85: config edit + reload endpoints + curated metadata` — three pieces in one commit (metadata informs response, response drives UX):
   - `config_metadata.go`: `ConfigKeyMeta {Name, HotReloadable, Doc}` + curated table. 3 hot keys (SIM_LOG_LEVEL, SOCKERLESS_LOG_LEVEL, SIM_PULL_POLICY), 14 annotated restart keys, unknown keys default to restart-required (safe default).
   - `PUT /api/v1/topology/projects/{p}/instances/{i}/config` writes Instance.Config via TopologyManager.UpdateInstance and returns `{hot_reloadable_changes, restart_required_changes}` so the UI prompts without a second round trip.
   - `POST /api/v1/topology/projects/{p}/instances/{i}/reload` shells new `make reload-component NAME=<n>` (kill -HUP via PID file). Component-side handling of SIGHUP is the component's concern. Reload also re-renders `.stack-pids/<n>.env` so a follow-up restart picks up the latest values.
2. `phase 85: ConfigEditModal + per-row hot/restart badges` — new `<ConfigEditModal>` opens from a "config" button on every InstanceRow. Each row shows a hot or restart badge from the metadata; save → server classifies → footer offers Reload / Reload (partial) + Restart / Close depending on what changed. 6 vitest cases.

## Phases after 85

- **Phase 86** — health + supervision surface. Mark unhealthy on process exit / non-2xx `/v1/health` / 5 s timeout; show last-N log lines + diagnostic links. No auto-restart.
- **Phase 87** — centralized observability (Stack A: OTel Collector + VictoriaLogs + Jaeger). Plan in PLAN.md; lands after 86 because 86's "show last-N log lines on unhealthy" is the file-tail source that 87 promotes to OTel.

After 87: phases 91–94 (real per-cloud volume provisioning), live-cloud validation track (Lambda / Cloud Run Services / ACA Apps / AZF cloud-dns / Lambda service-mesh / ACA-AZF Azure AD).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #142 | Phase 84 + BUG-985 + BUG-986 — sim NewServer + MakeStore fail loud on persistence open; admin SIM_DATA_DIR injection per topology instance; cross-cloud isolation tests; make purge-state operator targets. |
| 2026-05-10 | #141 | Phase 83 — shared `ResourceListPage` in `@sockerless/ui-core`; 13 sim pages refactored across simulator-aws / gcp / azure; legacy `/ui/resources` + `/ui/projects/:name` + `/ui/projects/:name/logs` retired. |
| 2026-05-10 | #140 | Phase 81 + Phase 82 — SSE log endpoint + single-instance tail UI + instance proxy endpoint + combined timeline + API console UI; cloud-resources rollup endpoint + UI with instance/cloud/service/flat groupings + failed-sources banner. |
| 2026-05-10 | #139 | Phase 80 — admin UI Topology page (`/ui/topology`): project + instance tree, per-instance status, Start/Stop/Rebuild, port registry. |
| 2026-05-10 | #138 | Phase 79 — `sockerless.yaml` topology store, `TopologyManager`, CRUD REST surface, `make/components.mk` lifecycle targets, port allocator. + Phase 87 plan + `specs/CLOUD_RESOURCE_MAPPING.md` consolidation. |
| 2026-05-10 | #137 | Phase 78 UI polish (dark mode, Toast/InlineError, Modal, a11y, perf, READMEs) + Phase 79 step 1 (`Instance` type). |
| 2026-05-10 | #136 | Phase 121b finish — driver consolidation, host-aliases everywhere, AZF/Lambda DNS, Azure AD access. |
| 2026-05-09 | #135 | Phase 121b initial — Azure sim hardening, all-6-backends harness restructure, driver consolidation, GCP sim Cloud Run routing, envelope parsing, label round-trip. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).
