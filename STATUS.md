# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-87c-zerolog-otel-bridge` — Phase 87c (full scope) in flight on PR #150. |
| In-flight | Phase 87c — zerolog → OTel logs bridge across **all 12 components**: 7 backends (`backends/core` bridge) + 3 sims + bleephub + admin (own bridge code per module since they don't share `backends/core`). Each binary uses `zerolog.MultiLevelWriter(consoleW, obs.LogWriter)` (admin uses `io.MultiWriter` + `TextLogWriter` since it's stdlib `log`, not zerolog). |
| Last merged | PR #149 — Phase 91 consolidated (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 986 fixed. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Phase 87c — in flight on `phase-87c-zerolog-otel-bridge` (PR #150)

Closes the observability story for **every** sockerless process — every log line now flows through BOTH stderr AND the OTel logs SDK when `OTEL_EXPORTER_OTLP_ENDPOINT` is set.

`backends/core/otel.go` (used by 7 backends):
- New `InitObservability(serviceName) (*Observability, error)` returns `{LogWriter, Shutdown}` (or zero value with no-op shutdown when OTel disabled). `InitTracer` stays for backward compat.
- New `OTelLogWriter` implements `io.Writer` so it slots into `zerolog.MultiLevelWriter(consoleW, otelW)`. Parses each JSON line and emits an OTel log Record. Maps zerolog level → severity, message → body, time → timestamp; promotes other fields to attributes.

Mirrored bridges (separate Go modules — can't import `backends/core`):
- `simulators/{aws,gcp,azure}/shared/otel.go` — full `Observability`. `Config.LogWriter` field plumbs through `NewServer` into the existing zerolog setup.
- `bleephub/otel.go` — full `Observability`; `cmd/main.go` uses `MultiLevelWriter`.
- `cmd/sockerless-admin/otel.go` — `Observability` adds `TextLogWriter` (stdlib `log` is flat text, not zerolog JSON); `main.go` wires `log.SetOutput(io.MultiWriter(os.Stderr, TextLogWriter))`.

5 new core tests. 12 components covered. Components-decoupled invariant intact.

## Phases after 87c

- **Phase 91d** — Real `pd-ephemeral` lifecycle on cloudrun + gcf. Multi-day cloud-API work.
- **Live-cloud validation track** — Lambda / Cloud Run Services / ACA Apps / AZF cloud-dns / Lambda service-mesh / ACA-AZF Azure AD.

## Recently shipped

| Date | PR | Headline |
|---|---|---|
| 2026-05-10 | #149 | Phase 91 (consolidated) — Lambda volume_translator scaffolding + framework migration; cloudrun + gcf reject `BackingPDEphemeral` with concrete pointers; integration TestMain switched to public.ecr.aws to dodge Docker Hub throttling. |
| 2026-05-10 | #148 | Phase 91b — `BackingMemory` translator on ECS / ACA / AZF. ACA `StorageTypeEmptyDir`; ECS + AZF reject loudly with concrete pointers. |
| 2026-05-10 | #147 | Phase 91 — `BackingMemory` translator on cloudrun + gcf (`EmptyDir{Memory}` + `SizeLimit` from `spec.Memory.SizeMB`). Closes the framework-vs-translator gap on the GCP backends. |
| 2026-05-10 | #146 | Phase 87b — wire OTel SDK across 6 backend main.go files + 3 sim shared/otel.go helpers + admin otel.go + otelhttp.NewHandler on sim/admin muxes. Spans flow from every Go binary into Jaeger when OTEL_EXPORTER_OTLP_ENDPOINT is set. |
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
