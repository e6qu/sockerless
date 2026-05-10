# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-91-pd-ephemeral-volumes` — Phase 91 in flight (1 implementation commit + state save). |
| In-flight | Phase 91 — `BackingMemory` translator on cloudrun + gcf. Closes the gap where Phase 127's MemoryDriver was registered but no translator handled the `BackingMemory` case. |
| Last merged | PR #146 — Phase 87b component-side OTel SDK wiring (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 986 fixed. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Phase 91 — in flight on `phase-91-pd-ephemeral-volumes`

One implementation commit + state save. Audit-driven scope.

The original Phase 91 brief was "lift the runner-task `emptyDir` fallback to real-workload provisioning of `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`". The audit found:

- `efs-ephemeral` is already wired on ECS (Phase 127); Lambda's inline EFS path predates the BackingSpec framework.
- `azure-files-ephemeral` is already wired on ACA + AZF.
- `pd-ephemeral` on Cloud Run is bookmarked at the spec level — Cloud Run Services don't have a first-class PD volume attach primitive (`specs/CLOUD_RESOURCE_MAPPING.md` line 567-568). Real implementation requires multi-day Compute Engine API lifecycle work.

The audit-discovered gap: `BackingMemory` (Phase 127) had its driver registered in all 6 backends but no translator handled the `case core.BackingMemory` arm. Operators picking `Backing: memory` would hit "unsupported backing kind" despite the driver claiming support.

`phase 91: BackingMemory translator on cloudrun + gcf` — adds the `case core.BackingMemory` arm to both translators, mapping to `EmptyDir{Medium: MEMORY}` with `SizeLimit` forwarded from `spec.Memory.SizeMB`. 5 tests. Phase 91b/c/d follow-ups will add the same arm to ECS / Lambda / ACA / AZF translators.

## Phases after 91

- **Phase 91b** — BackingMemory translator on ECS + Lambda. Each cloud's tmpfs primitive differs (ECS LinuxParameters.Tmpfs vs Lambda /tmp).
- **Phase 91c** — BackingMemory translator on ACA + AZF.
- **Phase 91d** — Real `pd-ephemeral` lifecycle on cloudrun + gcf (Compute Engine PD `disks.create`/`attach`/`delete`).
- **Phase 87c (optional)** — zerolog → OTel logs bridge so OTLP-mode operators don't depend on filelog. Skipped from 87b to keep dep churn contained.
- **Live-cloud validation track** — Lambda / Cloud Run Services / ACA Apps / AZF cloud-dns / Lambda service-mesh / ACA-AZF Azure AD.

## Recently shipped

| Date | PR | Headline |
|---|---|---|
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
