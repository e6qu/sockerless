# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-87-observability` — Phase 87 (stack + admin integration) in flight (4 implementation commits + state save). |
| In-flight | Phase 87 — observability stack (OTel Collector + VictoriaLogs + Jaeger) + admin /api/v1/observability config endpoint + UI deep-link chips on the diagnostic panel. **Phase 87b** (OTel SDK wired into each component's main.go + zerolog→OTel logs bridge) is the explicit follow-up. |
| Last merged | PR #144 — Phase 86 health + supervision surface (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 986 fixed. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Phase 87 — in flight on `phase-87-observability`

Four implementation commits + state save. **First-PR scope** is the observability stack itself + the admin UI integration. Component-side OTel SDK wiring across admin / sims / backends / bleephub is the explicit Phase 87b follow-up.

1. `phase 87: stack-observability make targets + collector config` — `make stack-observability-{up,down,status}` brings up otel-collector-contrib + VictoriaLogs + Jaeger as background processes. PIDs land in `.stack-pids/observability/`, distinct from `.stack-pids/*` so the cell stack and observability stack run independently. Default collector config in `make/observability-config/otel-collector.yaml` wires OTLP receivers (4317/4318) + a `filelog` receiver that scrapes `.stack-pids/*.log` so logs flow into VictoriaLogs **without any binary changes** (pidfile-named log file becomes `service.name`). State directories under `.sockerless-state/observability/{logs,traces}/` align with Phase 84 conventions.
2. `phase 87: GET /api/v1/observability config endpoint` — admin reads `OTEL_LOGS_DASHBOARD` + `OTEL_TRACES_DASHBOARD` env vars at boot. Returns `{enabled, logs_dashboard, traces_dashboard, logs_service_param, traces_service_param}` so the UI knows whether to render deep-link chips. Defaults to `service.name=<instance>` (VictoriaLogs) + `service=<instance>` (Jaeger) URL filters; both overridable. 7 unit tests.
3. `phase 87: VictoriaLogs / Jaeger deep links in diagnostic panel` — `<UnhealthyDiagnosticPanel>` fetches `/api/v1/observability` (cached 5-min staleTime) and renders `VictoriaLogs ↗` / `Jaeger ↗` chips when enabled. Disabled (env vars unset) → file-tail-only experience from Phase 86 unchanged. 2 new vitest cases.
4. `phase 87: docs/OBSERVABILITY.md` — two-mode operator guide (no-OTel default + OTel opt-in), service ports, install, retention, stack-swap to OpenObserve / SigNoz, Phase 87b roadmap.

## Phases after 87

- **Phase 87b** — wire OTel SDK + zerolog→OTel logs bridge into each component's `main.go`. `otelhttp.NewHandler` middleware on each mux for per-request spans. Once components emit OTLP, traces light up in Jaeger automatically (logs already work via the filelog receiver from Phase 87 chunk 1).

After 87b: phases 91–94 (real per-cloud volume provisioning), live-cloud validation track (Lambda / Cloud Run Services / ACA Apps / AZF cloud-dns / Lambda service-mesh / ACA-AZF Azure AD).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #144 | Phase 86 — health + supervision surface. Exit-code capture via watcher subshell + `CrashedSinceStart` distinction; 5 s probe timeout; `/diagnostics` endpoint bundling status + last-N logs; `<UnhealthyDiagnosticPanel>` mounted only on broken rows. |
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
