# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-86-health-supervision` — Phase 86 work in flight (3 implementation commits + state save). |
| In-flight | Phase 86 — health + supervision surface. Exit-code capture on every start-component; 5 s probe timeout; bundled diagnostic endpoint (status + tail logs); UnhealthyDiagnosticPanel UI mounted only on broken rows. |
| Last merged | PR #143 — Phase 85 admin config edit + hot reload (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 986 fixed. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Phase 86 — in flight on `phase-86-health-supervision`

Three implementation commits + state save:

1. `phase 86: capture exit codes + bump probe timeout to 5s` — `start-component` wraps the binary in a watcher subshell that records exit code + RFC-3339-utc timestamp to `.stack-pids/<n>.exit` when the binary terminates. `InstanceStatus` gains `Exit` + `CrashedSinceStart` fields (CrashedSinceStart fires when pidfile is present + dead AND an exit record exists — distinguishes crashes from clean stops, which remove the pidfile). `probeHealth` timeout bumped from 1 s to 5 s.
2. `phase 86: diagnostic endpoint bundling status + tail logs` — `GET /api/v1/topology/projects/{p}/instances/{i}/diagnostics?lines=N` returns one combined payload (status + last N lines of `.stack-pids/<n>.log`, default 50, cap 1000). Reuses the Phase 81 `readLastLines` helper.
3. `phase 86: UnhealthyDiagnosticPanel + per-row mount gate` — collapsible panel mounts under InstanceRow when `shouldRender(status)` is true (unhealthy / crashed_since_start / process gone with pidfile). Surfaces reason header + exit info + health_detail + last 50 log lines + deep links to full tail / console + refresh. Polls /diagnostics every 10 s only on broken rows, so cost is bounded.

## Phases after 86

- **Phase 87** — centralized observability (Stack A: OTel Collector + VictoriaLogs + Jaeger). Plan in PLAN.md; lands after 86 because 86's file-tail source is the precursor that 87 promotes to OTel.

After 87: phases 91–94 (real per-cloud volume provisioning), live-cloud validation track (Lambda / Cloud Run Services / ACA Apps / AZF cloud-dns / Lambda service-mesh / ACA-AZF Azure AD).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #143 | Phase 85 — admin config edit + hot reload. Curated `ConfigKeyMeta` table, PUT /config endpoint with classification, POST /reload + `make reload-component` (SIGHUP via PID file), ConfigEditModal UI with hot/restart badges + post-save Reload / Restart prompt. |
| 2026-05-10 | #142 | Phase 84 + BUG-985 + BUG-986 — sim NewServer + MakeStore fail loud on persistence open; admin SIM_DATA_DIR injection per topology instance; cross-cloud isolation tests; make purge-state operator targets. |
| 2026-05-10 | #141 | Phase 83 — shared `ResourceListPage` in `@sockerless/ui-core`; 13 sim pages refactored across simulator-aws / gcp / azure; legacy `/ui/resources` + `/ui/projects/:name` + `/ui/projects/:name/logs` retired. |
| 2026-05-10 | #140 | Phase 81 + Phase 82 — SSE log endpoint + single-instance tail UI + instance proxy endpoint + combined timeline + API console UI; cloud-resources rollup endpoint + UI with instance/cloud/service/flat groupings + failed-sources banner. |
| 2026-05-10 | #139 | Phase 80 — admin UI Topology page (`/ui/topology`): project + instance tree, per-instance status, Start/Stop/Rebuild, port registry. |
| 2026-05-10 | #138 | Phase 79 — `sockerless.yaml` topology store, `TopologyManager`, CRUD REST surface, `make/components.mk` lifecycle targets, port allocator. + Phase 87 plan + `specs/CLOUD_RESOURCE_MAPPING.md` consolidation. |
| 2026-05-10 | #137 | Phase 78 UI polish (dark mode, Toast/InlineError, Modal, a11y, perf, READMEs) + Phase 79 step 1 (`Instance` type). |
| 2026-05-10 | #136 | Phase 121b finish — driver consolidation, host-aliases everywhere, AZF/Lambda DNS, Azure AD access. |
| 2026-05-09 | #135 | Phase 121b initial — Azure sim hardening, all-6-backends harness restructure, driver consolidation, GCP sim Cloud Run routing, envelope parsing, label round-trip. |

Older PRs in [WHAT_WE_DID.md](WHAT_WE_DID.md).
