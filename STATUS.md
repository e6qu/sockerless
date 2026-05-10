# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-83-sim-ui-parity` — Phase 83 work in flight (5 commits). |
| In-flight | Phase 83 — sim UI parity. Shared `ResourceListPage` extracted; sim-aws / sim-gcp / sim-azure pages refactored onto it; legacy `/ui/resources` + `/ui/projects/:name/*` admin pages retired. |
| Last merged | PR #140 — Phase 81 + 82 (logs/console + cloud-resources rollup) (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Phase 83 — in flight on `phase-83-sim-ui-parity`

Five commits land Phase 83 in granular chunks:

1. `phase 83: add shared ResourceListPage to @sockerless/ui-core` — new component owns the useQuery + PageHeading + Spinner/InlineError/DataTable wiring so per-service sim pages collapse to a columns config + queryFn.
2. `phase 83: refactor simulator-aws pages onto ResourceListPage` — six pages on the shared component + design language. Drive-by `label`→`title` MetricsCard fix.
3. `phase 83: refactor simulator-gcp pages onto ResourceListPage` — same pattern.
4. `phase 83: refactor simulator-azure pages onto ResourceListPage` — same pattern + accessorFn type tightening.
5. `phase 83: retire legacy admin pages superseded by topology` — `/ui/resources` + `/ui/projects/:name` + `/ui/projects/:name/logs` deleted. Companion `AdminApiClient.project*` + `resources()` methods + `AdminResource` / `ProjectStatus` / `ProjectConnection` / `CreateProjectRequest` types deleted. Backing Go endpoints stay for legacy `--backend name=addr` registry path.

Net: 13 sim pages on the shared component (5 tests added in core, 11 tests removed in admin from page deletions); 3 admin pages + 9 client methods + 4 type aliases gone; admin UI tests 73 → 62.

## Phases after 83

- **Phase 84** — per-instance state isolation. `SIM_STATE_DIR=…/.sockerless-state/<project>/<instance>/`; sims gain optional persistent state across restarts, multiple sim instances of the same cloud coexist.
- **Phase 85** — config edit + hot reload. Admin annotates config keys hot-reloadable vs restart-required; UI writes back to `sockerless.yaml` and triggers reload or restart accordingly.
- **Phase 86** — health + supervision surface. Mark unhealthy on process exit / non-2xx `/v1/health` / 5 s timeout; show last-N log lines + diagnostic links. No auto-restart.
- **Phase 87** — centralized observability (Stack A: OTel Collector + VictoriaLogs + Jaeger). Plan in PLAN.md; lands after 86 because 86's "show last-N log lines on unhealthy" is the file-tail source that 87 promotes to OTel.

After 87: phases 91–94 (real per-cloud volume provisioning), live-cloud validation track (Lambda / Cloud Run Services / ACA Apps / AZF cloud-dns / Lambda service-mesh / ACA-AZF Azure AD).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #140 | Phase 81 + Phase 82 — SSE log endpoint + single-instance tail UI + instance proxy endpoint + combined timeline + API console UI; cloud-resources rollup endpoint + UI with instance/cloud/service/flat groupings + failed-sources banner. |
| 2026-05-10 | #139 | Phase 80 — admin UI Topology page (`/ui/topology`): project + instance tree, per-instance status, Start/Stop/Rebuild, port registry. |
| 2026-05-10 | #138 | Phase 79 — `sockerless.yaml` topology store, `TopologyManager`, CRUD REST surface, `make/components.mk` lifecycle targets, port allocator. + Phase 87 plan + `specs/CLOUD_RESOURCE_MAPPING.md` consolidation. |
| 2026-05-10 | #137 | Phase 78 UI polish (dark mode, Toast/InlineError, Modal, a11y, perf, READMEs) + Phase 79 step 1 (`Instance` type). |
| 2026-05-10 | #136 | Phase 121b finish — driver consolidation, host-aliases everywhere, AZF/Lambda DNS, Azure AD access. |
| 2026-05-09 | #135 | Phase 121b initial — Azure sim hardening, all-6-backends harness restructure, driver consolidation, GCP sim Cloud Run routing, envelope parsing, label round-trip. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).
