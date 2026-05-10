# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

State [STATUS.md](STATUS.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/](specs/).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **Real execution** — sims and backends actually run commands; no stubs, fakes, or mocks.
3. **External validation** — proven by unmodified external test suites.
4. **Driver-first handlers** — handler code routes through driver interfaces.
5. **LLM-editable files** — source files under 400 lines.
6. **State persistence** — every task ends with a state save.
7. **No fallbacks, no skips, no defers, no fakes** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find.
8. **Sim parity per commit** — any new SDK call adds a sim handler + matrix row in the same commit.
9. **Single work-branch rule** — all in-flight work lands on one branch. User handles every merge.
10. **Cross-cloud is permanently off the table** — cloud-specific drivers extend the generic shape; cross-cloud duplication is fine, in-cloud duplication consolidates into `*-common`.
11. **Components stay decoupled from admin / UI.** Sims, backends, bleephub remain independently configurable, buildable, runnable. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). No admin-required env vars on components, no startup registration, no "I'm being managed" hooks.

## Closed phases (PR index)

Headline-only. Per-bug detail in [BUGS.md](BUGS.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md).

| PR | Phases | Headline |
|---|---|---|
| #112–123 | 86–123 | Sim parity; stateless backends; FaaS pod overlays; storage-backing driver pilot; **8/8 runner cells GREEN.** |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #128 | 134 | Makefile standardization + per-app leaf Makefiles + stack orchestration. |
| #129 | 135 | Sim host model + 3-tier coverage + native arm64 CI runners. |
| #130 | 128 | Runner job timeout (bootstrap timer + cloud-native cap; SIGTERM → 30s → SIGKILL → exit 124). |
| #131 | 124 | Network discovery driver (host-aliases / cloud-dns / service-mesh / nat-gateway-only). |
| #132 | 125 | DNS driver (cloud-map / cloud-dns-zone / private-dns-zone / service-discovery / none). |
| #133 | 126 | Access driver (iam-role / id-token / mTLS / none-internal). |
| #134 | 127 | Storage driver expansion (pd-ephemeral / efs-ephemeral / azure-files-ephemeral). |
| #135 | 121b (initial) | Azure sim hardening, all-6-backends test harness restructure, in-memory storage, driver consolidation pattern B, GCP sim Cloud Run invoke routing, GCF envelope decode + label round-trip, drop QEMU. |
| #136 | 121b (finish) | Network-discovery adapter consolidation; host-aliases opt-in everywhere; AZF cloud-dns + Lambda service-mesh wiring; Azure AD access driver; pair DNS + cloud-side provisioning to NetworkDiscovery. |
| #137 | 78 + 79 step 1 | UI polish (dark mode toggle, Toast / InlineError, Modal + ContainerDetail, a11y, perf, READMEs) + admin `Instance` type. |
| #138 | 79 (full) + 87 plan | `sockerless.yaml` topology + `TopologyManager` + CRUD REST + lifecycle endpoints + `make/components.mk` granular targets + port allocator + Phase 87 observability plan (OTel+VictoriaLogs+Jaeger Stack A) + `specs/CLOUD_RESOURCE_MAPPING.md` consolidation (Docker/Podman→cloud quick reference, CI runner requirements, multi-system CI/CD comparison). |
| #139 | 80 + state save | Admin UI Topology page (`/ui/topology`): project + instance tree, per-instance status polling, Start/Stop/Rebuild, per-kind add/edit instance modal, add/delete project modal, auto-allocate port, port registry. Replaces legacy ProjectsPage + ProjectCreatePage. + state save for #138. |
| #140 | 81 + 82 + state save | Phase 81 — SSE log endpoint (`/api/v1/topology/projects/{p}/instances/{i}/logs?follow=1&lines=N`), instance proxy endpoint (`/proxy`), single-instance log tail UI (`/ui/topology/:project/:instance/logs`), combined timeline + API console UI (`/ui/topology/:project/console`). Phase 82 — cloud-resources rollup endpoint (`/api/v1/topology/resources`) + UI (`/ui/topology/resources`) with by-instance / by-cloud / by-service / flat groupings + failed-sources banner. |
| #141 | 83 | Phase 83 — shared `ResourceListPage` in `@sockerless/ui-core`; 13 sim pages refactored across simulator-aws / gcp / azure onto the shared component + design language; legacy `/ui/resources` + `/ui/projects/:name` + `/ui/projects/:name/logs` admin pages retired. |
| #142 | 84 + BUG-985 + BUG-986 | Phase 84 — sim shared `NewServer` returns `(*Server, error)` + `MakeStore` log.Fatalf on per-table failure (no silent in-memory degradation when persistence requested). Admin `SIM_DATA_DIR` injection per topology instance. Cross-cloud isolation tests. `make purge-state` operator targets. |
| #143 | 85 | Phase 85 — admin config edit + hot reload. Curated `ConfigKeyMeta` table, PUT /config endpoint with classification, POST /reload + `make reload-component` (SIGHUP via PID file), ConfigEditModal UI with hot/restart badges + post-save Reload/Restart prompt. |
| #144 | 86 | Phase 86 — health + supervision surface. Exit-code capture via watcher subshell + `CrashedSinceStart` distinction; 5 s probe timeout; `/diagnostics` endpoint bundling status + last-N logs; `<UnhealthyDiagnosticPanel>` mounted only on broken rows. |
| #145 | 87 | Phase 87 (Stack A first PR) — observability stack make targets, collector config with filelog receiver, /api/v1/observability endpoint, VictoriaLogs/Jaeger UI deep-link chips, docs/OBSERVABILITY.md. |
| #146 | 87b | Phase 87b — wire OTel SDK + otelhttp.NewHandler across 6 backend main.go files + 3 sim shared/otel.go helpers + admin otel.go. Trace emission for every Go binary when OTEL_EXPORTER_OTLP_ENDPOINT is set. |
| #147 | 91 | Phase 91 — `BackingMemory` translator on cloudrun + gcf. `EmptyDir{Medium: MEMORY}` + `SizeLimit` from `spec.Memory.SizeMB`. |
| #148 | 91b | Phase 91b — `BackingMemory` translator on ECS / ACA / AZF. ACA `StorageTypeEmptyDir`; ECS + AZF reject loudly with concrete pointers. |

## Roadmap (ordered)

### Phase 78 — UI polish ✓ complete (#137)

Dark mode, error UX, Container detail modal, accessibility, perf, documentation. See `WHAT_WE_DID.md` for details.

### Phase 79 — Topology + admin config service ✓ complete (PR #138)

Admin owns the source of truth for "what instances exist". `sockerless.yaml` at repo root carries `projects[]`, each with `instances[]` (sim / backend / bleephub, 0..N of each). Project model preserved. Existing per-project JSONs auto-migrate.

- ✓ Step 1: `Instance` type + per-kind validate + legacy derivation (#137).
- ✓ Step 2: `sockerless.yaml` topology store + `MigrateLegacyProjects` (#138).
- ✓ Step 3: `TopologyManager` singleton + read/write REST surface (#138).
- ✓ Step 4: `make/components.mk` granular targets; `stack-X-Y` rewritten as composition (#138).
- ✓ Step 5: `TopologyManager.AllocatePort` from `ports.ranges` (#138).
- ✓ Step 6: lifecycle REST endpoints shell `make {start,stop,rebuild}-component` (#138).
- ✓ Step 7: surgical CRUD endpoints (project + instance add/remove/edit) + per-instance status endpoint + `docs/ADMIN_ORCHESTRATION.md` (#138).

### Phase 80 — Admin UI: topology page + per-instance lifecycle ✓ complete (PR #139)

Topology page at `/ui/topology`: project + instance tree, per-instance status badge polled every 2s, per-instance Start/Stop/Rebuild buttons, per-kind add/edit instance modal (sim/backend/bleephub), add/delete project modal, auto-allocate port from configured pool, port registry view (configured ranges + claimed ports). Replaced legacy ProjectsPage + ProjectCreatePage. See `docs/ADMIN_ORCHESTRATION.md` § Admin UI — Topology page.

### Phase 81 — Per-instance logs + live troubleshooting console ✓ complete (PR #140 open)

`GET /api/v1/topology/projects/{p}/instances/{i}/logs?follow=1&lines=N` reads `.stack-pids/<name>.log`. Without `follow`: last N lines as JSON. With `follow=1`: SSE stream (seeded with last N, then one event per new line; keep-alive comments, truncation re-opens).

`POST /api/v1/topology/projects/{p}/instances/{i}/proxy` server-side dial to `http://localhost:<inst.Port>` so the API console panel avoids browser CORS.

UI: `/ui/topology/:project/:instance/logs` (live SSE tail with pause/resume/clear/seed-size). `/ui/topology/:project/console` (combined timeline subscribing to all per-instance streams, tagged + sorted by parsed timestamp or arrival; API console with method/path/headers/body fired through the proxy).

### Phase 82 — Cloud-resources rollup in admin ✓ complete (PR #140 open)

`GET /api/v1/topology/resources` aggregates `/internal/v1/resources` across every running backend instance in the topology, attributing each row with project + instance + cloud + backend. Sims excluded by design (they expose cloud APIs directly, not a uniform resource list). Per-source status surfaced so "0 resources" stays distinct from "couldn't query".

UI: `/ui/topology/resources` with grouping toggle (instance / cloud / service product / flat), active-only toggle, failed-sources banner, per-row status badge + cleaned-up tag.

### Phase 83 — Sim UI parity ✓ in flight (`phase-83-sim-ui-parity` branch)

`@sockerless/ui-core` gains a shared `ResourceListPage<T>` component owning the useQuery + heading + Spinner/InlineError-with-Retry/DataTable wiring. Each per-service sim page (ECS tasks, Lambda functions, Cloud Run jobs, ACR registries, …) collapses to a columns config + queryFn — automatic filter input, count meta, error retry, kicker styling, empty-state message all included.

Sweep: 13 sim pages refactored across simulator-aws / simulator-gcp / simulator-azure. OverviewPage gets `PageHeading` with kicker / meta / status badge in actions slot. Drive-by fix: `MetricsCard` was renamed `label`→`title` at some point but the OverviewPages still passed `label` (broken since the rename — TypeScript caught it).

Legacy `/ui/resources` (registry-backed) + `/ui/projects/:name` + `/ui/projects/:name/logs` admin pages retired — orphaned by the Phase 79/80 sweep, replacements landed in Phase 81/82. Companion `AdminApiClient.project*` + `resources()` methods and 4 type aliases removed. Backing Go endpoints stay for `--backend name=addr` CLI users.

Out of scope (deliberate): no Containers / Resources / Metrics pages on sims — those are backend concepts (Docker lifecycle, sockerless-tracked resources, backend metrics). Sims model cloud APIs directly.

### Phase 84 — Per-instance state isolation + persistence ✓ in flight (`phase-84-instance-state-isolation`)

Admin's `InstanceLifecycle.Start` writes `SIM_DATA_DIR=<repo>/.sockerless-state/<project>/<instance>/` into `.stack-pids/<n>.env` for sim instances. Multiple sim instances of the same cloud coexist with isolated state across restarts. Operator opts into persistence by adding `SIM_PERSIST=true` to the instance Config — admin doesn't force it.

BUG-985 + BUG-986 fixed in the same patch: both silent in-memory fallbacks in the sim shared layer (`NewServer` on `OpenDB` failure, `MakeStore` on `NewSQLiteStore` per-table failure). `NewServer` now returns `(*Server, error)`; `MakeStore` calls `log.Fatalf`. Sim main.go on the OpenDB path calls `log.Fatalf`. Operator-requested persistence fails loud at the start instead of silently degrading.

Operator workflow: `make purge-state PROJECT=<p> NAME=<i>` (single-instance) and `make purge-state-all` for clean-slate sweeps. `stop-component` deliberately preserves state.

Cross-cloud sweep: 5 test cases mirrored across simulators/{aws,gcp,azure}/shared/ — cross-DataDir isolation, persist-survives-reopen, BUG-985 regression guard, persist happy path, no-persist path.

### Phase 85 — Config edit + hot reload ✓ in flight (`phase-85-config-edit-hot-reload`)

Admin curates a `ConfigKeyMeta` table — 3 hot-reloadable keys (SIM_LOG_LEVEL, SOCKERLESS_LOG_LEVEL, SIM_PULL_POLICY) + 14 annotated restart-required keys + safe default (restart-required) for unknown keys. Metadata lives admin-side, NOT on the component.

`PUT /api/v1/topology/projects/{p}/instances/{i}/config` writes Instance.Config and returns the change classification so the UI prompts in one round trip.

`POST /api/v1/topology/projects/{p}/instances/{i}/reload` shells `make reload-component` (kill -HUP via PID file). Component-side handling of SIGHUP is the component's concern — Phase 85 ships the signal path; component absorption is per-binary.

UI: `<ConfigEditModal>` opens from a "config" button on every InstanceRow. Per-row hot/restart badges. Save → footer offers Reload / Reload (partial) + Restart / Close based on what classified server-side.

### Phase 86 — Health + supervision surface ✓ in flight (`phase-86-health-supervision`)

`start-component` wraps the binary in a watcher subshell that records exit code + RFC-3339-utc timestamp to `.stack-pids/<n>.exit` when the binary terminates. `InstanceStatus` gains `Exit` + `CrashedSinceStart` fields. `probeHealth` timeout bumped 1 s → 5 s.

`GET /api/v1/topology/projects/{p}/instances/{i}/diagnostics?lines=N` returns status + last N log lines in one shot (default 50, cap 1000). UI: `<UnhealthyDiagnosticPanel>` collapsible panel under InstanceRow rendered only when `shouldRender(status)` is true (unhealthy / crashed_since_start / process gone with pidfile). Polls /diagnostics every 10 s only on broken rows; cost is bounded.

No auto-restart — operator-driven recovery via the existing Restart / Reload / Stop buttons. Component-side handling is unchanged.

### Phase 87 — Centralized observability (logs + traces) — Stack A ✓ shipped (PR #145)

`make stack-observability-{up,down,status}` brings up otel-collector-contrib + VictoriaLogs + Jaeger. Default collector config scrapes `.stack-pids/*.log` so logs flow without binary changes. `GET /api/v1/observability` + UI deep-link chips on the diagnostic panel. `docs/OBSERVABILITY.md`.

### Phase 87b — Component-side OTel SDK wiring ✓ in flight (`phase-87b-component-otel-wiring`)

Trace emission for every Go binary. `core.InitTracer` wired into 6 backend main.go files (ecs / lambda / cloudrun / gcf / aca / azf — docker already had it from Phase 86). New `simulator.InitTracer` in each per-cloud sim shared package + `otelhttp.NewHandler` at the outermost middleware layer + 4-line init in each sim main.go. Admin gains its own duplicated `InitTracer` helper (separate Go module without backend-core dep) + otelhttp wrap on the mux. bleephub already wired since Phase 86. 11 new tracer tests.

**Phase 87c** ✓ in flight (`phase-87c-zerolog-otel-bridge`, PR #150) — zerolog → OTel logs bridge across all 12 components. `backends/core/otel.go` adds `InitObservability` + `OTelLogWriter`; 7 backends use it. Mirrored bridge in each `simulators/{aws,gcp,azure}/shared/otel.go` (separate Go module — `Config.LogWriter` threaded through `NewServer`), `bleephub/otel.go`, and `cmd/sockerless-admin/otel.go` (adds `TextLogWriter` for stdlib `log`). Components-decoupled invariant intact: emission gated on `OTEL_EXPORTER_OTLP_ENDPOINT`; no admin/UI dep injected. 5 new core tests.

**Stack A — Apache 2.0 throughout, three binaries:**
- **OpenTelemetry Collector** (Apache 2.0) receives OTLP at `localhost:4317`, fans out: logs → VictoriaLogs OTLP HTTP, traces → Jaeger OTLP, optional metrics → VictoriaMetrics.
- **VictoriaLogs** (Apache 2.0) for logs. Built-in UI on `:9428`. `--retentionPeriod=7d` cap.
- **Jaeger** all-in-one (Apache 2.0) for traces. Built-in UI on `:16686`. `--badger.span-store-ttl=72h` cap.

**Invariant preserved:** components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set in their env. Unset = today's behaviour (zerolog → stdout). No admin coupling, no required env var, no startup registration.

Sub-steps:

1. **`backends/core/otel/`** — wraps go.opentelemetry.io/otel SDK setup (logs + traces + resource attrs). Reads `OTEL_EXPORTER_OTLP_ENDPOINT` + service name. Used by every component's `main.go` in 3 lines.
2. **HTTP middleware** — wrap each backend / sim mux with `otelhttp.NewHandler` so spans land per request automatically.
3. **zerolog → OTel logs bridge** — existing `zerolog.Logger` calls also export to the OTel logs SDK. No log-line API changes.
4. **`make stack-observability-{up,down,status}`** in `make/stack.mk`. Runs collector + VictoriaLogs + Jaeger as background processes; PIDs in `.stack-pids/observability/`. Default config emits to `./.sockerless-state/observability/{logs,traces}/` with rotation + 5 GB total cap.
5. **Admin UI integration** — per-instance "View logs" + "View traces" deep links (filter by `service.name = <instance-name>`). Inline log tail (Phase 81) still works for the no-OTel path.
6. **Documentation** — `ui/README.md` + new `docs/OBSERVABILITY.md` cover both modes.

**Order vs other phases:** lands after Phase 86. Phase 86 ships with file-tail source for "show last-N log lines on unhealthy"; Phase 87 promotes to OTel-source when the collector is up.

If Stack A turns out unsuitable: same component code (OTLP) works against OpenObserve (AGPL) or SigNoz (MIT) — only `make/stack.mk` changes.

## Future phases

### Phases 91–94 — Real per-cloud volume provisioning

Lift the runner-task `emptyDir` fallback to real-workload provisioning of `pd-ephemeral` / `efs-ephemeral` / `azure-files-ephemeral`. Designs in `specs/CLOUD_RESOURCE_MAPPING.md` § Volume provisioning per backend.

**Phase 91 ✓ shipped (PR #147)** — `BackingMemory` translator on cloudrun + gcf. Audit found the actual gap: Phase 127's MemoryDriver was registered in every backend's storageBackings registry, but no translator handled the `case core.BackingMemory` arm.

**Phase 91b ✓ shipped (PR #148)** — `BackingMemory` translator on ECS / ACA / AZF. ACA gets clean `StorageTypeEmptyDir`; ECS + AZF reject loudly.

**Phase 91c (consolidated) ✓ in flight (`phase-91c-lambda-backingspec-migration`)** — Lambda `volume_translator.go` scaffolding + `fileSystemConfigsForBinds` migration to framework dispatch + cloudrun/gcf `BackingPDEphemeral` rejection arms with concrete pointers (`gcs-fuse` + `gcs-sync` alternatives, GCE-backend bookmark) + integration TestMain switched to ECR Public Gallery (Docker Hub throttling fix).

**Phase 91d** — Real `pd-ephemeral` lifecycle on cloudrun + gcf. Cloud Run Admin API doesn't expose PD attach as a first-class primitive today; would require a future GCE-style sockerless backend or wait for Cloud Run feature.

### Live-cloud validation track

Per-backend live-cloud sweeps separate from unit/sim CI. Live-AWS ECS validated 2026-04-20. Outstanding:
- Lambda live (deferred from Phase 86).
- Cloud Run Services / ACA Apps live (closed in code 2026-04-21 behind UseService/UseApp).
- AZF + cloud-dns on Azure live (new in #136).
- Lambda + service-mesh on AWS live (new in #136).
- ACA / AZF + Azure AD access on Azure live (new in #136).

## Driver phase template

Storage backing (Phase 123) is the pilot. Each driver phase follows:

1. `api/<dim>_driver.go` — enum + struct fields on the relevant config.
2. `backends/core/<dim>_driver.go` — driver interface + registry + no-op default.
3. `backends/<cloud>-common/<dim>_<impl>.go` — per-cloud impl (pattern B: shared by both backends in that cloud).
4. `backends/<cloud-product>/server.go` — wires the per-cloud driver into the backend's registry at startup.
5. Operator config: env var selects the driver per backend.
6. **No-fallbacks at resolve** — unset / unknown driver name returns an error.
7. Migration of existing inline calls to the registry.

Each phase starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Sockerless GCE-style backend (would unlock Phase 127 GCP `pd-ephemeral` for real workloads).
