# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-87b-component-otel-wiring` — Phase 87b in flight (2 implementation commits + state save). |
| In-flight | Phase 87b — component-side OTel SDK wiring. core.InitTracer wired into 6 backend main.go files + new sim/admin InitTracer helpers + otelhttp.NewHandler on sim/admin muxes. |
| Last merged | PR #145 — Phase 87 (Stack A first PR) (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 986 fixed. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Phase 87b — in flight on `phase-87b-component-otel-wiring`

Two implementation commits + state save. Spans now flow from every Go binary into Jaeger when OTEL_EXPORTER_OTLP_ENDPOINT is set. Logs already worked from Phase 87 via the collector's filelog receiver scraping .stack-pids/*.log.

1. `phase 87b: wire core.InitTracer into 6 backend main.go files` — ecs / lambda / cloudrun / gcf / aca / azf each gain a 4-line OTel init at startup (mirroring docker's existing pattern from Phase 86). otelhttp middleware was already in `backends/core/server.go`; this commit makes it actually emit by initialising the tracer. Service names: `sockerless-backend-{name}`.
2. `phase 87b: wire OTel SDK + otelhttp on sims + admin` — 3 sims gain `shared/otel.go` (new InitTracer helper) + otelhttp.NewHandler at the outermost middleware layer + 4-line init in each main.go. Admin gains a duplicated InitTracer helper (separate Go module without backend-core dep) + otelhttp.NewHandler wrapping the mux. 11 new tracer tests.

bleephub was already fully wired (InitTracer + otelhttp) since Phase 86 baseline — no changes needed.

## Phases after 87b

- **Phase 87c (optional)** — zerolog → OTel logs bridge so OTLP-mode operators don't depend on the filelog receiver fallback. Adds OTel logs SDK across the 4 affected modules (backends/core, bleephub, sims/shared × 3, admin). Skipped from Phase 87b to keep the dep churn contained — filelog covers the immediate need.

After 87b/c: phases 91–94 (real per-cloud volume provisioning), live-cloud validation track (Lambda / Cloud Run Services / ACA Apps / AZF cloud-dns / Lambda service-mesh / ACA-AZF Azure AD).

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #145 | Phase 87 (Stack A first PR) — `make stack-observability-{up,down,status}` (otel-collector + VictoriaLogs + Jaeger), filelog receiver scraping `.stack-pids/*.log`, `GET /api/v1/observability` endpoint, VictoriaLogs/Jaeger deep-link chips on the diagnostic panel, `docs/OBSERVABILITY.md`. |
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
