# Sockerless — Status

Roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md).

## Snapshot

| | |
|---|---|
| Active branch | `phase-87d-92-observability-closeout-gcs-sync` — Phase 87d closeout + Phase 92 bundled, in flight. |
| In-flight | Phase 87d — trace context propagation across admin + bleephub HTTP clients (otelhttp.NewTransport on every plain `&http.Client{}`); MeterProvider + runtime metrics across all 12 components (auto-emits HTTP request count/duration/size from the existing otelhttp.NewHandler + Go runtime metrics); `make stack-observability-validate` end-to-end harness. Phase 92 — gcs-fuse deregistered on cloudrun + gcf with translator reject pointing at gcs-sync (Cloud Run rejects cache-TTL flags so gcs-fuse is broken for cross-task workspaces — BUG-987 closes the original BUG-944 documentation). |
| Last merged | PR #150 — Phase 87c full scope (2026-05-10). |
| Cells | 8/8 runner-integration cells GREEN since 2026-05-07. |
| Bugs | 0 open · 987 fixed. |
| Live infra | None up. |

**Invariant:** components stay decoupled from admin / UI. Sims, backends, bleephub run independently via env vars; admin only reads what they already expose (`/v1/health`, `/v1/info`). Phase 81 SSE tails admin's own `.stack-pids/<name>.log`; Phase 82 rollup queries existing `/internal/v1/resources` endpoints — no new component-side wiring.

## Phase 87d + 92 — in flight on `phase-87d-92-observability-closeout-gcs-sync`

**Phase 87d closes Phase 87.** Three real gaps the audit surfaced after the bridge work merged:

1. **Trace context propagation** — admin's 7 plain `&http.Client{}` sites (proxyClient, rollupClient, health-poll, component proxy, project-manager health-wait, sim bootstrap, instance probe) and bleephub's 2 (GitHub Actions tarball fetch, webhook dispatch) now use `otelhttp.NewTransport(http.DefaultTransport)` so outgoing requests carry the `traceparent` header. `tracedHTTPClient(timeout)` helper added in admin's `otel.go` for clean reuse. Global propagator (`TraceContext + Baggage`) set inside each of the 5 `InitObservability` implementations — 12 components covered.
2. **MeterProvider + runtime metrics** — `InitObservability` now also creates a MeterProvider with the OTLP HTTP metric exporter, so `otelhttp.NewHandler`'s built-in HTTP request count / duration / size flow (they were emitted into a no-op meter until now). `runtime.Start` from `go.opentelemetry.io/contrib/instrumentation/runtime` wires Go runtime metrics (goroutines, GC pauses, heap size). 2 new tests in `backends/core/otel_test.go`.
3. **`make stack-observability-validate`** — manual operator-grade end-to-end check. Polls VictoriaLogs + Jaeger until ≥1 log line and ≥1 trace land for the requested service (default `sockerless-backend-docker`), with configurable timeout (`OBS_VALIDATE_TIMEOUT_S`, default 30s) and service override (`OBS_VALIDATE_SERVICE`). Documented in `docs/OBSERVABILITY.md` § Validation.

**Phase 92 closes BUG-944 + ships BUG-987.** `Backing: gcs-fuse` on cloudrun + gcf produced silently broken cross-task workspaces because Cloud Run rejects the cache-TTL gcsfuse flags as unrecognized (`metadata-cache:ttl-secs`, `metadata-cache:negative-ttl-secs`). Without those flags the 5s negative-cache hides freshly-written files from sibling containers. Real fix: deregister `GCSFuseDriver` on cloudrun + gcf, translator rejects `BackingGCSFuse` with a concrete pointer at `gcs-sync` (per-exec tar/untar, no FUSE, strong consistency). Driver code stays in `backends/gcp-common/storage_gcsfuse.go` for hypothetical future backends without the flag-allowlist constraint.

## Phases after 87d / 92

- **Phase 91d** — Real `pd-ephemeral` lifecycle on cloudrun + gcf. Bookmarked: Cloud Run lacks the primitive entirely (`runpb.Volume` has no PersistentDisk field). Implementation requires either a sockerless GCE-style backend or a future Cloud Run feature.
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
