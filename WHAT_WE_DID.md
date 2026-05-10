# Sockerless — What We Built

Docker-compatible REST API that runs containers on cloud backends (ECS, Lambda, Cloud Run, GCF, ACA, AZF) or local Docker. 7 backends, 3 cloud simulators, validated against SDKs / CLIs / Terraform. Designed to power CI runners on cloud serverless capacity — see [docs/RUNNERS.md](docs/RUNNERS.md).

State [STATUS.md](STATUS.md) · roadmap [PLAN.md](PLAN.md) · resume [DO_NEXT.md](DO_NEXT.md) · bugs [BUGS.md](BUGS.md) · architecture [specs/](specs/).

This file keeps narrative — *why* each phase, what was surprising, what blocked. Per-bug detail in [BUGS.md](BUGS.md); code-level detail in `git log`.

## 2026-05-10 — Phase 83 sim UI parity (`phase-83-sim-ui-parity` branch)

Five-commit branch: shared `ResourceListPage` component → sim-aws / gcp / azure refactor → legacy admin page retirement.

**Audit first.** Spent the first 10 minutes reading the actual sim and admin code instead of trusting the original Phase 83 brief. Found:

- The original brief said "lift sim UIs onto BackendApp shell" — but `BackendApp` carries Docker-domain pages (Containers / Resources / Metrics) that don't apply to sims. Sims model cloud APIs (ECS tasks, Lambda functions, S3 buckets), so the page taxonomy is necessarily different.
- Shell parity was *already there* — `SimulatorApp` already wraps `ErrorBoundary` + `ToastProvider` + `BrowserRouter` + `AppShell` (which itself includes `ThemeToggle`). The sim apps were not the problem.
- The actual gap: each sim page was 20–30 LOC of `<h2>` + `<DataTable>`, no `PageHeading`, no error UX, no filter input, no count meta. Compared to admin pages they looked like sketches.

DO_NEXT.md was rewritten on the back of that audit before implementing anything.

**Shared component.** `ResourceListPage<T>` lives in `@sockerless/ui-core/components`. Owns the `useQuery` so each call site collapses to props:

```tsx
<ResourceListPage<ECSTask>
  kicker="aws · simulator · ecs"
  title={<>Tasks</>}
  countNoun="task"
  columns={columns}
  queryKey={["ecs-tasks"]}
  queryFn={fetchECSTasks}
  filterPlaceholder="Filter tasks…"
  emptyMessage="No ECS tasks tracked."
/>
```

Renders `PageHeading` (kicker / italic title / "{count} {noun}" meta auto-pluralised) + `Spinner` on initial load + `InlineError` with a `Button` retry on failure (driven by react-query's `refetch`) + `DataTable` on success. Refresh defaults to 5 s polling; pass `false` or `0` to disable. `meta` and `actions` slots accept overrides.

5 vitest cases cover: rows + heading on success, singular vs plural meta, error path with retry, meta override hides the default count, actions slot.

**Sweep.** 13 sim pages refactored across simulator-aws / simulator-gcp / simulator-azure. Each Overview page also gained a `PageHeading` with kicker / italic title / status badge actions (was a bare `<h2>` + flex row). Drive-by fix on every Overview: `MetricsCard` was renamed `label`→`title` at some point, but the OverviewPages still passed `label` — broken since the rename, TypeScript caught it the moment the build ran against the new component. 15 MetricsCard call sites fixed.

LOC delta sim-aws: 155 → 196 (+25%). Increase comes from explicit kickers / empty messages / filter placeholders that each old page lacked. Filter input + retry-on-error + count meta are now automatic, not per-page boilerplate. The "code drop expected" line in the original Phase 83 brief was wishful — real outcome is design parity + uniform behavior, with a small line increase. Same shape across all three sims.

**Legacy retirement.** Three admin pages were orphaned in the Phase 79/80 sweep: `Topology` became the source of truth, but `/ui/resources` (legacy registry-backed), `/ui/projects/:name` (legacy project detail), and `/ui/projects/:name/logs` (legacy combined-component logs) were left in the routes. Phase 81 + 82 made the replacements concrete (per-instance logs, project console, cloud-resources rollup), so deleting the originals is unblocked.

Removed:

- 3 page components (`ResourcesPage`, `ProjectDetailPage`, `ProjectLogsPage`).
- 3 vitest files for those pages.
- 9 `AdminApiClient` methods (`projects`, `projectGet`, `projectCreate`, `projectStart`, `projectStop`, `projectDelete`, `projectLogs`, `projectConnection`, `resources`).
- 4 type aliases (`AdminResource`, `ProjectConfig`, `ProjectStatus`, `ProjectConnection`, `CreateProjectRequest`).
- The `Resources` nav item.

Backing Go endpoints stay intact for components added via the `--backend name=addr` CLI flag (legacy registry, not topology-driven). UI tests went from 73 → 62 (−11 from the deleted pages, +0 from the refactor since shared component testing lives in core).

**Granular commit shape.** Five commits (one per chunk: shared component, three sim-package refactors, retirement). CI cycle pending after the first push.

## 2026-05-10 — Phase 81 + Phase 82 + state save (PR #140)

Single PR landing two phases on the topology surface from #138 / #139. Operator workflow reads as: *open `/ui/topology` → click an instance's "logs" → live SSE tail; click a project's "console" → multi-instance combined feed + arbitrary HTTP request panel; click "cloud resources" → see what every running backend has provisioned.*

**Phase 81 (per-instance logs + live troubleshooting console).** Three pieces: SSE log endpoint, instance proxy endpoint, two new UI pages.

- **SSE log endpoint.** `GET /api/v1/topology/projects/{p}/instances/{i}/logs?follow=1&lines=N` reads admin's own `.stack-pids/<name>.log` (written by `make start-component` redirecting stdout/stderr). Without `follow`: last N lines as JSON (default 200, capped at 10k). With `follow=1`: `text/event-stream` with one `data: <line>\n\n` per new line; seeds with last N then polls file size every 250 ms. Subtle: seed read and `os.Open` happen *atomically* once the file appears — initial naive draft seeded before opening then set `offset = file size`, which silently dropped every line that arrived before the file existed (caught by `TestInstanceLogsStreamWaitsForFile`). Truncation (file shrinks) re-opens at offset 0; embedded `\n` collapsed to spaces because SSE spec splits `data:` on newlines and would silently merge log lines on the client.
- **Instance proxy endpoint.** `POST /api/v1/topology/projects/{p}/instances/{i}/proxy` with `{method, path, headers?, body?}` body — admin dials `http://localhost:<inst.Port><path>` server-side, returns `{status, status_text, headers, body, duration_ms}`. Server-side proxy keeps the API console UI free of browser CORS gymnastics (components don't have admin's origin in any allow-list and shouldn't grow one). 30 s timeout, 4 MB body cap. Components-decoupled invariant preserved: the proxy hits whatever the component already exposes; nothing new component-side.
- **Single-instance log tail UI** (`/ui/topology/:project/:instance/logs`). `useLogStream` hook owns one `EventSource` per render, exposes `{lines, connected, error, clear}`, caps the rolling buffer at 5k lines so a chatty component can't OOM the tab. Page has pause/resume + clear + seed-size selector (50/200/500/1000) + `StatusBadge` reflecting connection state. Uses the existing core `LogViewer` for ANSI parsing.
- **Combined console UI** (`/ui/topology/:project/console`). Two cards.
  - *Combined timeline.* Renders one `<StreamSubscriber>` per project instance (fixed order — Rules of Hooks doesn't allow hook calls in dynamic loops; the wrapper component holds the hook). Each subscriber owns its own `EventSource`, forwards lines to a shared `onLine` callback that pushes to a merged buffer (capped at 5k entries, tagged with instance name + arrival counter). Per-instance enable toggles close/reopen the corresponding stream. Sort toggle: by parsed leading timestamp (ISO-8601 prefix or JSON `time`/`ts`/`timestamp` fields, including seconds-vs-millis heuristic) or by arrival order; lines that don't parse sort to the tail in time mode. Color per instance is a stable hash → 6-color palette so the eye picks streams apart.
  - *API console panel.* Instance dropdown (defaults to first), method/path/headers/body inputs (body disabled for GET/HEAD), submits via the proxy endpoint. Response panel: `StatusBadge` mapped from status (2xx → ok, 5xx → error, else warning), duration, collapsible header list, body pretty-printed when `Content-Type: application/json`.
- **Wiring.** New "logs" button on every InstanceRow in `/ui/topology`; new "console" button on each project header. New `topologyInstanceProxy` method on `AdminApiClient` plus `ProxyRequest` / `ProxyResponse` types.

**Phase 82 (cloud-resources rollup).** New backend endpoint + UI page that aggregates `/internal/v1/resources` across every running topology backend.

- **Endpoint.** `GET /api/v1/topology/resources?active=true` walks `TopologyManager.Instances()`, fans out concurrently to each backend (5 s timeout), returns `{sources: [...], resources: [...]}`. Each `rollupSource` carries `{ok, error, resource_count, ...identity}` so the UI distinguishes "0 resources" from "couldn't query this backend". Each `rollupEntry` is the upstream resource attributed with `{project, instance, cloud, backend, port}`. *Sims are not queried* — they implement actual cloud APIs (DescribeInstances, ListBuckets, …) rather than a uniform sockerless resource list. Rollup is therefore backend-only by design, not a fallback. Forwards `?active=true` so cleaned-up rows drop out unless the operator opts in.
- **UI** (`/ui/topology/resources`). Group toggle: instance (default) / cloud / service product (resource_type) / flat. Failed-sources banner surfaces unreachable backends with their error string. Per-row `StatusBadge` from upstream `status`; "cleaned" tag on rows where `cleaned_up=true`. 5 s polling refresh.
- **Decision.** New page sits alongside the legacy `/ui/resources` (driven by the registered-component registry from CLI flags) rather than replacing it. The two have different scope — legacy = whatever was registered via `--backend name=addr`; new = whatever is in `sockerless.yaml`. Phase 83 (sim UI parity) sweep is the natural moment to retire the legacy page once topology is the canonical source.

**Granular commit shape.** Six commits on the branch (`phase 81: SSE log-tail endpoint`, `phase 81: single-instance live log tail UI`, `phase 81: instance proxy endpoint for the API console`, `phase 81: combined timeline + API console UI`, `phase 82: cloud-resources rollup endpoint`, `phase 82: cloud-resources rollup UI`) plus the state-save and README badge bumps. Per-chunk CI verification on each push.

**Test coverage.** Backend: 6 SSE endpoint tests, 6 proxy endpoint tests, 6 rollup endpoint tests (concurrent fan-out, upstream errors, unreachable backends, sims-excluded). UI: 4 InstanceLogsPage tests (with stubbed `EventSource`), 12 ProjectConsolePage tests (parseTimestamp edge cases incl. seconds-vs-millis JSON ts, page renders both panels, SSE opens per instance, lines tagged, disable closes stream, proxy fires through send, GET body disabled), 9 TopologyResourcesPage tests (groupResources unit tests across all 4 groupings, page renders summary, failed-sources banner, grouping chip switches layout, default fetch carries active=true). 73 admin UI tests pass total.

## 2026-05-10 — Phase 80 admin UI topology page + state save (PR #139)

Bundles the post-#138 state save with full Phase 80 delivery on a single PR.

**Phase 80 (admin UI topology page).** New `/ui/topology` route is the operator front door for `sockerless.yaml`. Pure UI build on top of the `/api/v1/topology/*` REST surface shipped in #138 — no business logic in the admin client beyond querying / posting / surfacing errors.

Page composition:

- **Project tree.** One card per project; expanding shows every instance with kind / cloud / backend / port / sim-ref summary (`InstanceRow`). Empty topology renders a centered "no projects configured" stub.
- **Per-instance status.** Each `InstanceRow` runs its own `useQuery` against `GET /api/v1/topology/projects/{p}/instances/{i}/status` with `refetchInterval: 2000`. `StatusBadge` shows `ok` / `unhealthy` / `unknown` / `stopped`; `health_detail` (the failed-probe reason) renders inline when present.
- **Per-instance lifecycle buttons.** Start / Stop / Rebuild POST to the matching lifecycle endpoint. Per-mutation `isPending` + `variables.name` gate the right row's buttons so you can't double-click a slow start. Toast feedback via `useReportError` / `useToast`.
- **Per-project actions.** "+ instance" opens `InstanceForm`; "delete project" opens the `ConfirmDeleteModal` (project removal is destructive — sockerless.yaml entry only; running processes are NOT stopped, the modal copy says so explicitly).
- **Port registry card.** Side panel listing configured `ports.ranges[<kind>]` next to every claimed port across all projects (sorted by port number). Memoised over `topology` so the list re-renders only when topology changes.

Form components:

- **`ProjectForm`** (`components/ProjectForm.tsx`) — minimal modal with one text input. Project's cloud / backend fields are intentionally NOT exposed; those are legacy fields used only by the per-project tuple shape, and modern usage is per-Instance.
- **`InstanceForm`** (`components/InstanceForm.tsx`) — per-kind fields rendered via visibility flags driven off the `kind` select. `sim` shows cloud + port; `backend` adds backend dropdown (cloud-scoped: aws → ecs|lambda; gcp → cloudrun|gcf; azure → aca|azf) + sim-ref dropdown (lists same-project sims); `bleephub` is port-only. "Auto-allocate" button calls `POST /api/v1/topology/allocate-port?kind=<kind>` and fills the port field. Env-config table is fully editable with add/remove rows; empty rows are stripped on submit. Edit mode disables name + kind (rename = delete + add).

Plumbing:

- New API client methods on `AdminApiClient`: `topology`, `topologyReplace`, `topologyInstances`, `topologyAddProject`, `topologyRemoveProject`, `topologyAddInstance`, `topologyUpdateInstance`, `topologyRemoveInstance`, `topologyInstanceStart` / `Stop` / `Rebuild`, `topologyInstanceStatus`, `topologyAllocatePort`. Added `putJSON` helper alongside the existing `postJSON` / `del`.
- Nav: `Projects` → `Topology` (route `/ui/topology`). `ProjectsPage` + `ProjectCreatePage` files deleted with their tests (no fallback / back-compat per the no-fakes directive). `ProjectDetailPage` updated to navigate back to `/ui/topology` on delete.
- Tests: `__tests__/TopologyPage.test.tsx` covers heading + project + instance render, port registry render, project-form open, instance-form open with project pre-selected, empty state, Start button visible for stopped instances, confirm-dialog open path. 13 admin test files / 48 tests pass.
- Docs: `docs/ADMIN_ORCHESTRATION.md` gains an "Admin UI — Topology page" section (what it shows, what it does, what it intentionally doesn't do).

**State save for #138** (also in this PR): STATUS.md / DO_NEXT.md / WHAT_WE_DID.md / PLAN.md updated to reflect PR #138 merged and Phase 80 in flight.

## 2026-05-10 — Phase 79 (full) + Phase 87 plan + docs consolidation (PR #138, merged)

**Phase 79 complete: admin orchestration backend.** `sockerless.yaml` at repo root carries the full topology. `Topology` struct (`topology_store.go`): `projects[]` × `instances[]`, port pool global. YAML marshal + atomic save (tmp + rename). `MigrateLegacyProjects` reads existing `~/.sockerless/admin/projects/*.json` (when YAML absent), derives `[sim, backend]` instances via `DeriveLegacyInstances`, writes `sockerless.yaml`.

`TopologyManager` singleton (`topology_manager.go`): owns the in-memory copy under RWMutex; defensive deep-copy on Get; `replaceLocked` consolidates validate + persist + swap. Surgical APIs: `AddProject`, `RemoveProject`, `AddInstance`, `RemoveInstance`, `UpdateInstance`, `FindInstance`, `Instances` (flat `InstanceRef` list).

`AllocatePort(kind)` (`topology_ports.go`) walks `ports.ranges[<kind>]` pool, skips claimed + bound ports (signal-0 PID probe + listen test), fails loud on no-range / exhausted.

REST surface (`api_topology.go`): GET/PUT topology; GET instances list; GET/POST/PUT/DELETE per-instance; POST start/stop/rebuild; POST allocate-port; POST projects; DELETE projects/{p}; GET status. `crudErrorStatus` maps "already exists"→409, "not found"→404, "validate:..."→400, else 500.

Lifecycle (`instance_lifecycle.go`) shells `make {start,stop,rebuild}-component`. Per-instance Config map serialised to `.stack-pids/<name>.env`, passed via `ENV_FILE=`. Admin doesn't need to know component env schema. PID + log keyed by component name (`.stack-pids/<NAME>.{pid,log,env}`).

Per-instance status (`instance_status.go` + `instance_status_unix.go`): `InstanceStatus{Project, Name, Running, PID, Health, HealthDetail}`. Signal-0 PID probe split per-OS (Windows door open). Health: ANY of (process exit | `/v1/health` non-2xx | 1s timeout) → unhealthy. No auto-restart.

`make/components.mk` granular targets: `start-component KIND=… NAME=… PORT=… [CLOUD=…] [BACKEND=…] [SIM_PORT=…] [ENV_FILE=…]`; `stop-component`, `rebuild-component`, `logs-component`; `status-components` / `stop-components` sweep targets. `make/stack.mk` `stack-X-Y` macros rewritten as composition of `rebuild-component` + `start-component` (`STACK_SIM_CLOUD_<be>` lookup map).

`docs/ADMIN_ORCHESTRATION.md` documents schema, REST surface, make targets, env-file convention, and the explicit "what admin is NOT responsible for" list (no startup hooks, no required env vars, no implicit defaults).

**Phase 87 plan added.** Centralized observability via Stack A (all Apache 2.0): OpenTelemetry Collector receiving OTLP at `:4317`, fanning out to VictoriaLogs (`:9428` UI, 7d retention) for logs and Jaeger all-in-one (`:16686` UI, 72h retention) for traces. Components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set (decoupled-from-admin invariant preserved). Sub-steps in PLAN.md: `backends/core/otel/` SDK wrapper → `otelhttp.NewHandler` middleware → zerolog→OTel-logs bridge → `make stack-observability-{up,down,status}` → admin UI deep links → docs. Open-source-only constraint (MPL incompatible with AGPLv3 ruled out Vector / SigNoz proper); Stack A is fully Apache 2.0 + swappable for AGPL alternatives without component code changes.

**Docs sweep + `specs/CLOUD_RESOURCE_MAPPING.md` consolidation.** New top-level Contents TOC. Two new sections inserted between "Universal rules" and "Mapping per cloud" without removing existing content:

- *Docker / Podman API → cloud + drivers — quick reference*: per-endpoint summary tables (container lifecycle, networks, volumes, build/registry, lifecycle/management) cross-referencing the existing detailed Docker API coverage matrix lower in the file.
- *CI runner requirements — what each runner needs from sockerless*: GitLab runner contract, GitHub Actions runner contract, explicit subsections for "GitHub runner default is NOT ephemeral; we need it ephemeral" (covers `--ephemeral` flag, per-spawn registration token, dispatcher cleanup) and "GitHub runner cannot self-spawn jobs (needs separate dispatcher)" (covers ARC-in-k8s exception that maps to k8s-deployed ACA). Plus a multi-system CI/CD comparison covering Azure DevOps Pipelines, Jenkins, Drone, Buildkite, CircleCI, Tekton, Argo Workflows, Concourse, TeamCity, Bitbucket Pipelines, GoCD, Semaphore, Travis, Bamboo, Earthly, Dagger, Cloud Build, AWS CodeBuild, Harness, Spinnaker, ArgoCD, FluxCD — distinguishing direct fits (Docker-API-shaped), k8s-only systems, closed managed services, and BuildKit/IaC categories.

**Bogus `frontend-docker` InstanceKind dropped.** Speculatively added in step 1; no Go binary backs it (only a UI package). Removed from enum, make targets, port ranges, route handlers, docs per no-fakes principle.

## 2026-05-10 — Phase 78 + 79 step 1 (PR #137, merged)

**Phase 78 (UI polish).** `useTheme` + `ThemeToggle` (sidebar footer; localStorage + prefers-color-scheme + dark default). `ToastProvider` mounted by `BackendApp` / `SimulatorApp` / admin / bleephub; `useToast` / `useReportError` / `useToastQueryErrors`. `InlineError` for in-page errors with Retry. `Modal` (native `<dialog>`-backed) + `ContainerDetailModal` opening from row click. Accessibility pass (DataTable sort headers as buttons + `aria-sort`, clickable rows keyboard-activatable, AppShell skip-link + `aside`/`nav` labels + `main id+tabIndex`, Spinner role). Admin mutations toast on success+failure. `ui/README.md` documents `make stack-X-Y` / `stack-status` / `stack-down` start/stop, default ports, and per-package vite-dev mode. Admin Toast wiring; vitest infra: jsdom origin pinned + `localStorage`/`matchMedia` polyfills + cleanup() between tests.

**Phase 79 step 1.** `Instance` type for admin orchestration (`cmd/sockerless-admin/instance.go`). Per-kind validate (sim / backend / bleephub / frontend-docker; required cloud + backend fields per kind; port > 0; name regex match). `DeriveLegacyInstances` bridges old `ProjectConfig{SimPort, BackendPort}` shape to a `[sim, backend]` Instance list so existing project JSONs continue to enumerate without manual migration.

Critical invariant established: components stay decoupled from admin / UI. Sims, backends, bleephub, frontend-docker remain independently configurable / buildable / runnable. Admin reads only `/v1/health`, `/v1/info`, env vars — no admin-required env vars on components, no startup registration, no "I'm being managed" hooks. Same invariant extends to Phase 87 observability: components emit OTLP only when `OTEL_EXPORTER_OTLP_ENDPOINT` is set.

PR #137 design discussion locked the topology shape: `sockerless.yaml` at repo root, `projects[]` × `instances[]` (0..N of each kind), project model preserved (each project = isolated topology), port pool global, admin shells `make` for lifecycle, Phase 87 observability stack = VictoriaLogs + Jaeger + OpenTelemetry Collector (all Apache 2.0).

## 2026-05-10 — Phase 121b finish (PR #136, merged)

Completes Phase 121b with the items the initial PR (#135) deferred. Cross-cutting work delivered:

- **Network-discovery adapter consolidation** — `cloudMapDiscovery` (ecs), `cloudDNSDiscovery` (cloudrun), `acaCloudDNSDiscovery` (aca) moved into `aws-common` / `gcp-common` / `azure-common` as pattern-B drivers (callback-based, backend-specific state passed through callbacks). Underlying `*Server` register/deregister/resolve methods + their helpers moved alongside; per-backend `network_discovery_adapter.go` + `service_discovery_cloud.go` files deleted. Pattern: backends construct the driver with SDK clients + `LookupNetwork`/`GetNetwork` callbacks; the driver owns the cloud-API calls.
- **Host-aliases discovery opt-in everywhere** — every backend's `Config` gains a typed `NetworkDiscovery api.NetworkDiscoveryKind` field, populated from `SOCKERLESS_<X>_NETWORK_DISCOVERY` env. `Validate` enforces the per-backend allowed set (fail-loud on unsupported values, no fallback to default).
- **AZF DNS adapter** — AZF gains a `NetworkState{DNSZoneName}` model + per-network Azure Private DNS zone provisioning at `NetworkCreate` time + `cloud-dns` case in the discovery switch. Mirrors ACA's zone shape (`skls-<name>.local`); no NSG layer because AZF function apps egress through Azure's managed plane. New `PrivateDNSZones` + `PrivateDNSRecords` clients in `AzureClients`.
- **Lambda DNS + service-mesh** — Lambda gains `NetworkState{NamespaceID}` + `LambdaState.ServiceID` + `EC2` and `ServiceDiscovery` clients + `cloudNamespaceCreate/Delete` + `service-mesh` case. The per-invocation IP isn't peer-reachable (Lambda hyperplane ENIs are shared), so the existing `awscommon.CloudMapDiscovery` register-IP gate (originally for cloudrun-jobs) skips automatically; ResolveName works for the read direction. Validate requires `SOCKERLESS_LAMBDA_SUBNETS` when service-mesh is selected (Cloud Map private DNS namespaces are VPC-bound).
- **Azure AD access driver** — new `api.AccessMechanismAzureAD` + `azurecommon.AzureADAccess` (wraps `azcore.TokenCredential`; per-request `Authorization: Bearer <token>` whose scope is `<audience>/.default`). ACA + AZF gain `Config.Access` + `Config.AccessPrincipal` fields populated from `SOCKERLESS_<X>_ACCESS` / `SOCKERLESS_<X>_ACCESS_PRINCIPAL` env vars (default: `none-internal`). Pairs with operator-side Easy Auth (AAD provider) on the ACA app or function app.
- **DNS↔NetworkDiscovery gating cleanup** — DNS drivers + cloud-side network resources (Cloud Map namespace, Cloud DNS zone, Private DNS zone) were previously wired unconditionally even when the operator picked host-aliases or nat-gateway-only — wasted provisioning + lookups against zones no register path was populating. Now folded into the matching discovery case in `NewServer`, and per-backend `cloudNamespaceCreate`/`cloudNetworkCreate` gated on the matching `NetworkDiscovery` kind. ACA's `cloudNetworkCreate` takes a `provisionDNSZone` bool — NSG always created (cross-container security is independent of discovery), zone only when cloud-dns selected.

Pre-existing GCF `invokeFunction` envelope fallback removed during this PR (per the no-fallbacks directive — bootstrap MUST return the exec envelope; non-envelope is a bug, not a downgrade path).

## 2026-05-09 — Phase 121b initial scope + driver consolidation + GCP sim invoke routing (PR #135, merged)

Single PR. Multi-layer scope:

- **Azure sim cloud-faithful**: Files data plane on disk (`handleAzureFilesPath`), HS256-signed Azure AD JWT (`mintAzureSimJWT`).
- **All-6-backends test harness restructured** to `SOCKERLESS_TEST_TARGET=sim|cloud`. No skips, no fallbacks, no `//go:build integration` tag, no `SOCKERLESS_INTEGRATION` env. `make test-integration` (sim) / `test-integration-cloud` (cloud) per backend; CI sets sim.
- **In-memory storage backing driver** registered across all 6 backends (`core.MemoryDriver`, `BackingMemory`).
- **Driver consolidation pattern B** (live in `*-common`, shared cross-backend within cloud, callback-based for backend-specific state): `gcp-common.IDTokenAccess`, `aws-common.IAMRoleAccess`, `gcp-common.CloudDNSZoneDNS`, `aws-common.CloudMapDNS`, `azure-common.PrivateDNSZoneDNS`. Per-backend adapters deleted.
- **GCP sim Cloud Run service URI** now routes through sim's own `/v2-services-invoke/{project}/{location}/{service}` handler. Was issuing bogus `https://<svc>-<project>.run.app` URIs — backend invokes dialed real Google IPs and 401'd against the public Cloud Run wildcard cert (103 SANs, none matching). Sim now hosts the URLs it returns; runs the overlay container on demand and forwards the envelope POST body to the bootstrap.
- **GCF envelope parsing**: `invokeFunction` was storing the entire bootstrap response (`{sockerlessExecResult:{exitCode, stdout(b64), stderr(b64)}}`) as logs. Extracted `gcpcommon.ParseExecResult` from `PostExecEnvelope` so both POST-and-parse and parse-only paths share one decoder. Subprocess exit code now propagates through `inv.ExitCode`.
- **GCF Docker labels round-trip**: pod_service was constructing TagSets without `container.Config.Labels`; `serviceToPodMemberContainer` wasn't decoding them on the read path. `dockerLabelsFromCloudRunService` merges svc.Labels + svc.Annotations and reverses the AsMap encoding.
- **Cloudrun TestMain in sim mode** disables overlay path. Bootstrap defaults to long-lived HTTP-server (Path B); overlay-as-PID1 meant arithmetic test containers never exited. `TestCloudRunJobTimeout` removed; timer is fully unit-tested in `agent/cmd/sockerless-cloudrun-bootstrap/main_test.go`.
- **Tooling**: `scripts/check-latest-deps.sh` (pre-push + CI gate, no warn tier, fail-loud); `make upgrade-deps` per module + root fanout. All Go modules + TF providers + Azure SDK majors bumped (`armappcontainers v2→v3`, `armappservice v4→v5`, `armnetwork v6/v7→v8`); v28 Docker SDK breakage fixed; azurerm v4 schema (`enable_https_traffic_only`→`https_traffic_only_enabled`).
- **Publish workflow**: dropped QEMU. Per-arch native runners (`ubuntu-latest` amd64, `ubuntu-24.04-arm` arm64). Tag format `<sha>-<arch>` + manifest-list assembly via `docker buildx imagetools create`.

Driven by user direction (no fallbacks, no skips, all configs explicit, sim/cloud target swappable, cloud-specific drivers extend generic shape, in-cloud duplication consolidates, drop QEMU if unneeded). Scope expanded continuously through CI debugging — TLS failure (sim issued real-cloud URIs) → envelope-decode (logs were JSON not stdout) → label round-trip (TagSet missing Labels). Each surfaced as the prior fix unblocked the next layer.

Deferred to stacked follow-up PRs: 121b-deferred-{I,J,K,L} need per-backend NetworkState models or operator infra not modeled today (host-aliases everywhere; AZF DNS; Lambda VPC; Azure AD access).

## 2026-05-09 — Phase 127 Storage driver expansion (PR #134, merged)

3 new `core.StorageBacking`: `pd-ephemeral` (GCP CE PD), `efs-ephemeral` (AWS EFS access point), `azure-files-ephemeral` (Azure Files share). All honor the no-idle-cost directive. Per-cloud drivers in `gcp-common` / `aws-common` / `azure-common`; per-backend `storageBackings` registries wire them in. 15 unit tests. Existing volume materialization paths unchanged (consolidation deferred).

## 2026-05-09 — Phase 126 Access driver (PR #133, merged)

`AccessMechanism` enum (iam-role / id-token / mTLS / none-internal). `AccessDriver` interface = `Mechanism()` + `WorkloadPrincipal() string` + `AuthenticatedClient(ctx, audience) (*http.Client, error)`. Per-backend adapters: cloudrun + GCF id-token (wraps `idtoken.NewClient`), ECS + Lambda iam-role (SigV4 at SDK), ACA + AZF none-internal. Every `idtoken.NewClient` callsite migrated through `s.Access.AuthenticatedClient`; the `idtoken` import disappears from both backends.

## 2026-05-09 — Phase 125 DNS driver (PR #132, merged)

`DNSMechanism` enum (cloud-map / cloud-dns-zone / service-discovery / private-dns-zone / none). `DNSDriver` = `SearchDomain(ctx, networkID)` + `Mechanism()`. Per-backend adapters: cloudrun cloud-dns-zone, ECS cloud-map, ACA private-dns-zone (FaaS = NoOp). `SOCKERLESS_DNS_SEARCH_DOMAIN` injected at every `ContainerCreate`; cloudrun + gcf bootstraps write `search` line to `/etc/resolv.conf`.

## 2026-05-09 — Phase 124 Network discovery driver (PR #131, merged)

`NetworkDiscoveryKind` enum (host-aliases / cloud-dns / service-mesh / nat-gateway-only). Per-backend adapters: cloudrun cloud-DNS, ECS service-mesh (Cloud Map), ACA cloud-DNS, GCF host-aliases (in-process). `BaseServer.NetworkDiscovery` field; all `cloudServiceRegister/Deregister/Resolve` callsites migrated through the driver. Interface signature widened to include explicit `containerID` so Cloud Map (keys by ID) and DNS-zone (keys by hostname) both fit.

## 2026-05-09 — Phase 128 Runner job timeout (PR #130, merged)

Two-layer timeout: bootstrap timer (`runWithTimeout` in cloudrun + gcf bootstraps; SIGTERM → 30s grace → SIGKILL → exit 124) + cloud-native cap (cloudrun TaskTemplate.Timeout, ACA ReplicaTimeout, Lambda 900s) derived from `core.JobTimeoutDefault()`. `SOCKERLESS_JOB_TIMEOUT_SECONDS` contract; per-job override via `docker run -e` wins.

## 2026-05-09 — Phase 135 Sim host model + native arm64 CI (PR #129, merged)

Workloads dispatch through Docker honouring explicit `Architecture` (sim's `linux/arm64` capacity); per-cloud-product host-metadata services (AWS IMDSv2 + ECS task v4; GCP `metadata.google.internal`; Azure IMDS); static no-`os/exec`-of-workload check; SDK metadata tests; native `ubuntu-24.04-arm` CI runners (no QEMU). 12 bugs closed (BUG-949/972/975-984).

## Older closed phases (compressed)

| PRs | Phases | Headline |
|---|---|---|
| #128 | 134 | Makefile standardization + per-app leaf Makefiles + stack orchestration. |
| #127 | 129#4 + 130–132 | Orphan pod-Service GC; sim parity prep; bleephub workflows + oauth REST + UI. |
| #125 | CI reorg | Workflows reorganized: zero auto-fire on main; live-tests-{cloud}. |
| #122–123 | 110 + 118 + 120–123 | 8/8 runner cells GREEN; FaaS pod overlays; cloud-faithful GCP sim; storage-backing driver pilot. |
| #117–121 | 109 + Round-7/8/9 | Live-AWS bug sweep; strict cloud-API fidelity audit. |
| #112–115 | 86–102 | Sim parity; stateless backends; real volumes; FaaS invocation tracking; reverse-agent exec/cp/diff/commit/pause; Docker pod synthesis. |

Per-bug detail in [BUGS.md](BUGS.md). Per-commit detail in `git log`.
