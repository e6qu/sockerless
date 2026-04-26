# Sockerless — Roadmap

> **Goal:** Replace Docker Engine with Sockerless for any Docker API client — `docker run`, `docker compose`, TestContainers, CI runners — backed by real cloud infrastructure (AWS, GCP, Azure).

Current state: [STATUS.md](STATUS.md). Bug log: [BUGS.md](BUGS.md). Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md). Architecture: [specs/](specs/). Resume pointer: [DO_NEXT.md](DO_NEXT.md).

## Guiding principles

1. **Docker API fidelity** — match Docker's REST API exactly.
2. **Real execution** — simulators and backends actually run commands; no stubs, fakes, or mocks.
3. **External validation** — proven by unmodified external test suites.
4. **No new frontend abstractions** — Docker REST API is the only interface.
5. **Driver-first handlers** — all handler code through driver interfaces.
6. **LLM-editable files** — source files under 400 lines.
7. **GitHub API fidelity** — bleephub works with unmodified `gh` CLI.
8. **State persistence** — every task ends with a state save (PLAN / STATUS / WHAT_WE_DID / DO_NEXT / BUGS / memory).
9. **No fallbacks, no defers** — every functional gap is a real bug; every bug gets a real fix in the same session it surfaces; cross-cloud sweep on every find.

## Closed phases

Detail in [WHAT_WE_DID.md](WHAT_WE_DID.md); commit + BUG refs in [BUGS.md](BUGS.md).

| Phase(s) | Summary |
|---|---|
| 86–102 + audit sweep | Foundation — sim parity, stateless backends, real volumes, FaaS invocation tracking, reverse-agent exec / cp / diff / commit / pause, Docker pod synthesis, ACA console exec, ECS SSM ops, OCI push, log fidelity. Closes BUG-661–769. |
| Round-7 (PR #117) | Live-AWS bug sweep — 16 bugs (BUG-770..785). |
| Round-8 (PR #118) | Live-AWS bug sweep — 13 bugs (BUG-786..800). Restored Phase 89 stateless invariant (BUG-799/800), real registry-to-registry layer mirror (BUG-788), sync `docker stop` (BUG-790), per-network SG isolation (BUG-794), spec-doc refresh + accepted-gaps section (BUG-787). |

## Pending work

Order is the order of execution unless noted.

### Round-9 manual-test crosswalk (in progress, this branch)

Per-test walk through [PLAN_ECS_MANUAL_TESTING.md](PLAN_ECS_MANUAL_TESTING.md) cross-referenced against [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md). Live working state: [docs/manual-test-spec-crosswalk.md](docs/manual-test-spec-crosswalk.md). Mismatches file as BUG-801..NNN; coverage gaps (spec claims with no test) get added to the runbook as new test rows.

**Scope (in scope, not deferred):**

- ECS — Tracks A (49 tests), B (33), C (11), E (7), F (12), G (7), I (9). **Done.** 3 bugs filed and fixed in-track (BUG-801, BUG-803, BUG-805); 2 deferred to Phase 105 (BUG-804, BUG-806); 1 withdrawn (BUG-802 — measurement artifact).
- **Lambda — Track D (9 tests).** Runs with a sockerless-lambda-bootstrap prebuilt overlay image (`SOCKERLESS_LAMBDA_PREBUILT_OVERLAY_IMAGE`) — D2-D7 verify the function-invocation lifecycle; D8/D9 verify the spec's "NotImpl with named missing prerequisite" path without `SOCKERLESS_CALLBACK_URL`. Cross-built binaries staged at `/tmp/r9-overlay/`; build needs Docker Desktop / podman-machine running on the operator's host.
- **Coverage-gap test rows** to add to the runbook after Track D — verify `sockerless-restart-count` tag value, `sockerless-kill-signal` exit-code, ImagePush layer-byte content, etc.
- **A46 NotImpl wrapper** — translate the bootstrap-PID-file exit-64 case into a clean `NotImplementedError` per spec (currently surfaces as a generic `kill -STOP failed (exit -1):`).

**Skipped this round:**

- Track H (podman-compose) — not installed locally.
- Track J (runner integration) — needs a real GitLab Runner / GitHub Actions runner.
- Tracks against GCP / Azure — terraform live envs don't exist yet.

### Phase 104 — Cross-backend driver framework

Sockerless has a narrow `core.Drivers{Exec, Stream, Filesystem}` and a few concrete drivers (`ReverseAgentExecDriver`, `LocalFilesystemDriver`); each backend then routes its own bespoke implementations of build / commit / cp / diff / export / pause / stats / top / logs through ad-hoc paths. Phase 104 lifts that into one pluggable system.

**Goal:** every "perform docker action X against the cloud" decision flows through a typed `Driver` interface in `backends/core/drivers/`. **Interfaces in core; implementations live with the cloud they use** (`backends/ecs/drivers/`, `backends/aws-common/drivers/`, `backends/aca/drivers/`, etc.). Each backend constructs its `DriverSet` at startup; operators override per-cloud-per-dimension via `SOCKERLESS_<BACKEND>_<DIMENSION>=<impl>`; sim parity required for the default driver in every dimension.

**Driver dimensions (13 separate interfaces — kept finer-grained for independent swap):**

| Dimension | Default per backend | Alt drivers we'd add |
|---|---|---|
| `ExecDriver` | docker→DockerExec; ECS→SSMExec; Lambda/CR/GCF/AZF→ReverseAgentExec; ACA→ACAConsoleExec ⇄ ReverseAgentExec | (none yet) |
| `AttachDriver` | docker→DockerAttach; ECS→CloudWatchAttach; FaaS→CloudLogsReadOnlyAttach (lift `core.AttachViaCloudLogs`) | ACAConsoleAttach |
| `FSReadDriver` (cp →, stat, get-archive) | docker→DockerArchive; ECS→SSMTar; FaaS+CR+ACA→ReverseAgentTar | OverlayUpperRead (Phase 103) |
| `FSWriteDriver` (cp ←, put-archive) | docker→DockerArchive; ECS→SSMTarExtract; FaaS+CR+ACA→ReverseAgentTarExtract | OverlayUpperWrite |
| `FSDiffDriver` | docker→DockerChanges; ECS→SSMFindNewer; FaaS+CR+ACA→ReverseAgentFindNewer | OverlayUpperDiff |
| `FSExportDriver` | docker→DockerExport; ECS→SSMTarRoot; FaaS+CR+ACA→ReverseAgentTarRoot | OverlayMergedExport |
| `CommitDriver` | docker→DockerCommit; FaaS+CR+ACA→ReverseAgentTarLayer+Push; ECS→accepted-gap NotImpl | OverlayLayerCommit |
| `BuildDriver` | docker→LocalDockerBuild; ECS+Lambda→CodeBuild; CR+GCF→CloudBuild; ACA+AZF→ACRTasks | KanikoInContainer, BuildKitRemote |
| `StatsDriver` | docker→DockerStats; AWS→CloudWatchAggregate; GCP→CloudMonitoring; Azure→LogAnalytics | CloudWatchInsightsRich |
| `ProcListDriver` (top) | docker→DockerTop; ECS→SSMPs; FaaS+CR+ACA→ReverseAgentPs | ProcSelfBootstrap |
| `LogsDriver` | docker→DockerLogs; AWS→CloudWatch; GCP→CloudLogging; Azure→LogAnalytics | (none yet) |
| `SignalDriver` (pause/unpause/kill) | docker→DockerKill; ECS→SSMKill; FaaS+CR+ACA→ReverseAgentKill | (none yet) |
| `RegistryDriver` (push/pull) | per-cloud: ECRPullThrough+ECRPush; ARPullThrough+ARPush; ACRCacheRule+ACRPush | (none yet) |

**Envelope:**

```go
type DriverContext struct {
    Ctx       context.Context
    Container api.Container        // pre-resolved by ResolveContainerAuto
    Backend   string               // "docker" | "ecs" | "lambda" | …
    Region    string
    Logger    zerolog.Logger
}

type Driver interface {
    Describe() string  // "<backend> <dimension> via <transport>; missing: <prereq>"
}

// per-dimension typed Options/Result (Exec returns 3 streams, Build returns
// a JSON status stream, Stats returns a snapshot, …)
```

The dispatcher in `backends/core/backend_impl.go` calls `ResolveContainerAuto` once, builds `DriverContext`, then invokes `s.Drivers.<X>.<method>(dctx, opts)`. An unset / `NotImpl` driver auto-emits `NotImplementedError` from `Describe()`.

**Layout (interfaces in core, implementations next to their cloud):**

```
backends/core/drivers/
  types.go            # DriverContext + 13 interfaces
  set.go              # DriverSet aggregate
  override.go         # SOCKERLESS_<BACKEND>_<DIMENSION> env-var overrides
  reverseagent/       # cloud-agnostic — used by every backend that ships
                      # a sockerless bootstrap. Existing core.ReverseAgent*
                      # drivers move here unchanged.

backends/docker/drivers/        # host docker SDK
backends/aws-common/drivers/    # SSM / CodeBuild / ECR (shared ECS+Lambda)
backends/ecs/drivers/           # CloudWatch Logs/Metrics/Attach (ECS-only)
backends/aca/drivers/           # ACA console exec
backends/gcp-common/drivers/    # CloudBuild / Cloud Logging / Cloud Monitoring / AR
backends/azure-common/drivers/  # ACR Tasks / Log Analytics / ACR
```

**Refactor delivery — piecemeal, dimension at a time, no behaviour change per commit:**

1. Add `core/drivers/types.go` with `DriverContext` + per-dimension interfaces.
2. Lift existing `core.ReverseAgent{Exec,Stream}Driver` and `core.LocalFilesystemDriver` into the new namespace; semantic-preserving move.
3. For each dimension in this order — Exec, FSRead, FSWrite, FSDiff, FSExport, ProcList, Signal, Stats, Logs, Attach, Commit, Build, Registry:
   - Add the typed driver interface.
   - Move bespoke per-backend code into a typed driver impl (with `Describe()`).
   - Each backend's `server.go` constructs the default for that dimension.
   - `backend_impl.go` method gets rerouted through the driver.
   - Sim integration test for the default driver lands in the same commit.
   - Bespoke method (e.g. `ContainerExportViaSSM`) gets deleted.
4. After all dimensions are lifted, add the operator-override env-var dispatcher.
5. Spec doc gets a per-backend default-driver matrix table.

**Composition rule:** unset / `NotImpl` driver auto-emits `NotImplementedError` whose message comes from `Describe()`. No per-backend boilerplate.

**Sim contract:** every default driver must work end-to-end against its cloud's simulator. Alternate drivers (Kaniko, OverlayUpper) may be operator-installable only, with a clear "sim doesn't emulate this" note in `specs/CLOUD_RESOURCE_MAPPING.md`.

**Driver-impl testing:** sim-only — drivers test against the real cloud SDK pointed at the simulator, matching today's project culture (no mocks).

### Phase 103 — Overlay-rootfs bootstrap (under Phase 104)

Replaces Phase 98's `find / -newer /proc/1` heuristic with overlayfs upper-dir for diff / commit / cp / export on every backend that ships a sockerless bootstrap (Lambda, Cloud Run, ACA, GCF, AZF). Bootstrap mounts `overlay -o lowerdir=/,upperdir=/sockerless/upper,workdir=/sockerless/work /merged`, pivots, execs user CMD; reverse-agent reads `upper/` directly. Captures deletions as whiteouts (closes the BUG-750 known limitation). Out of scope on ECS — operator's image runs as-is, no bootstrap insertion point.

**Ships under Phase 104** as the first set of alternate drivers under the new framework: `OverlayUpperRead`, `OverlayUpperWrite`, `OverlayUpperDiff`, `OverlayMergedExport`, `OverlayLayerCommit`. Gated behind `SOCKERLESS_OVERLAY_ROOTFS=1` per backend. Caveats: Lambda needs `CAP_SYS_ADMIN` (default has it) + `/tmp` workspace (10 GB cap); Cloud Run / GCF run gVisor with partial overlayfs support (may need tmpfs upper-dir); ACA / AZF full Linux, no caveats.

### Phase 105 — Libpod-shape conformance (parallel to 104)

`podman` CLI uses bindings that expect specific JSON shapes. Sockerless's responses match docker-API but diverge from libpod in places, breaking the CLI even when the docker path works. Cross-walks every libpod handler in `backends/core/handle_libpod*.go` against upstream `pkg/api/handlers/libpod` shapes; fixes BUG-804 (`pod inspect` returns array, libpod expects object), BUG-806 (`pod stop` `Errs` shape mismatch); adds golden-file tests so future shape regressions land at CI time. Independent of Phase 104 — can run in parallel.

### Live-cloud validation runbooks

- **Phase 86 Lambda live track** — scripted already; covered partially by round-9 Track D.
- **Phase 87 live-GCP** — needs project + VPC connector; terraform env to add.
- **Phase 88 live-Azure** — needs subscription + managed environment with VNet integration; terraform env to add.

### Known workaround to convert to a real fix

**BUG-721** — SSM `acknowledge` format isn't accepted by the live AWS agent; backend dedupes retransmitted `output_stream_data` frames by MessageID. BUG-789/798 likely share root cause (live AWS SSM frame parsing). Proper fix needs WebSocket frame capture against a live exec session. Sim path is unaffected.

### Phase 68 — Multi-Tenant Backend Pools (queued)

Named pools of backends with scheduling and resource limits. P68-001 done; 9 sub-tasks remain (registry, router, limiter, lifecycle, metrics, scheduling, limits, tests, state save).

### Phase 78 — UI Polish (queued)

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Webhook delivery UI.
- Cost controls (per-pool spending limits, auto-shutdown).
