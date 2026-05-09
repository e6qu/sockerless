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
11. **Components stay decoupled from admin / UI.** Sims, backends, bleephub, frontend-docker remain independently configurable, buildable, runnable. Admin reads only what they already expose (`/v1/health`, `/v1/info`, env vars). No admin-required env vars on components, no startup registration, no "I'm being managed" hooks.

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

## Roadmap (ordered)

### Phase 78 — UI polish ✓ complete (#137)

Dark mode, error UX, Container detail modal, accessibility, perf, documentation. See `WHAT_WE_DID.md` for details.

### Phase 79 — Topology + admin config service (in progress)

Admin owns the source of truth for "what instances exist". `sockerless.yaml` at repo root carries `projects[]`, each with `instances[]` (sim / backend / bleephub / frontend-docker, 0..N of each). Project model preserved. Existing per-project JSONs auto-migrate.

- ✓ Step 1: `Instance` type + per-kind validate + legacy derivation (in #137, merged).
- ⏳ Step 2: `sockerless.yaml` topology store. **← current.**
- Step 3: REST endpoints (`/v1/admin/topology`, `/v1/admin/instances/{key}/{start|stop|rebuild}`).
- Step 4: `make start-component` / `stop-component` / `rebuild-component` granular targets; existing `stack-X-Y` become wrappers.
- Step 5: Free-port helper + auto-allocation from `ports.ranges`.
- Step 6: One-shot migration of existing JSONs into `sockerless.yaml`.

### Phase 80 — Admin UI: topology page + per-instance lifecycle

Replace ProjectsPage with a project + instance tree; per-instance Start/Stop/Rebuild controls; "Add instance" form; edit / delete; port registry view (allocated + free ranges).

### Phase 81 — Per-instance logs + live troubleshooting console

Live log tail per instance via SSE from admin (reads `.stack-pids/<name>.log`). Combined-timeline view (sim + backend interleaved). API console panel: send arbitrary HTTP requests against an instance, inspect request/response.

### Phase 82 — Cloud-resources rollup in admin

AdminCloudResources page aggregates resources across all running backend + sim instances. Group by cloud / by service product / by sockerless instance.

### Phase 83 — Sim UI parity

Lift sim UIs to match backend UIs: Containers / Resources / Metrics pages, ToastProvider, ErrorBoundary, ThemeToggle, log tailer, API console. Refactor sim Apps onto the same shell shape `BackendApp` uses.

### Phase 84 — Per-instance state isolation + persistence

Sims gain optional persistent state (env-var-driven, `SIM_STATE_DIR=…`) under `./.sockerless-state/<project>/<instance>/`. Multiple sim instances of the same cloud coexist with isolated, durable state across restarts.

### Phase 85 — Config edit + hot reload

Admin-side annotation per-config-key: hot-reloadable vs restart-required. Admin UI edits write back to `sockerless.yaml`; admin triggers reload or restart based on annotation. Annotation lives in admin metadata, not on the component.

### Phase 86 — Health + supervision surface

Mark instance unhealthy on ANY of: process exit, `/v1/health` non-2xx, no `/v1/health` response within 5s. Admin UI shows failing signal + last-N log lines + diagnostic links. No auto-restart (operator-driven recovery).

### Phase 87 — Centralized observability (logs + traces) — Stack A

**Goal:** every sockerless component (sim, backend, bleephub, admin, frontend-docker) emits structured logs + traces to a local OpenTelemetry pipeline. Admin UI deep-links to per-instance log + trace queries.

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
