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
| Round-8 + Round-9 (PR #118) | Live-AWS bug sweep — 30 bugs (BUG-786..819). Round 8: stateless invariant (BUG-799/800), real layer mirror (BUG-788), sync `docker stop` (BUG-790), per-network SG isolation (BUG-794). Round 9: live SSM frame capture → exit-code marker (BUG-789/798), `sh -c` exec wrap (BUG-815), busybox-compat find/stat (BUG-816/817), Lambda invoke waiter (BUG-807), tag-based InvocationResult persistence (BUG-811), filter substring match (BUG-795), per-cloud `null_resource sockerless_runtime_sweep` so `terragrunt destroy` is self-sufficient (BUG-819). |
| Post-PR-#118 audit (PR #120 — open) | Bug-audit sweep + phase plan — 18 bugs closed (BUG-802 withdrawn; BUG-638/640/646/648 backfilled retroactively as closed by BUG-788; BUG-804/806 libpod-shape; BUG-820..831 fallback / synthetic-data findings, including docker NCPU/MemTotal hardcode and 0.0.0.0 placeholder leak). New phases queued: 106 (GitHub Actions runner), 107 (GitLab runner), 108 (cross-simulator feature parity audit). Project rule recorded: no defer, no fakes, no fallbacks. |

## Pending work

Order is the order of execution unless noted.

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

### Phase 105 — Libpod-shape conformance (rolling — first wave landed in PR #119)

`podman` CLI uses bindings that expect specific JSON shapes. Sockerless's responses match docker-API but diverge from libpod in places, breaking the CLI even when the docker path works. **First-wave fixes shipped post-PR-#118**: BUG-804 (`PodInspectResponse` expanded to mirror `define.InspectPodData`) and BUG-806 (`PodStop`/`PodKill` Errs normalised to `[]`; per-container failures routed via HTTP 409 ErrorModel) plus golden-shape tests in `backends/core/pod_inspect_shape_test.go`.

Remaining work for this phase: cross-walk every libpod handler in `backends/core/handle_libpod*.go` against upstream `pkg/api/handlers/libpod` shapes; add golden tests for each so future shape regressions land at CI time; verify against a real podman client (currently we have no live podman CLI in CI). Can run in parallel with Phase 104.

### Phase 106 — Real GitHub Actions runner integration

End-to-end test of GitHub Actions self-hosted runners pointed at sockerless via `DOCKER_HOST` (docker-executor mode). Currently we only have synthetic `tests/github_runner_e2e_test.go` mocking the runner's docker calls; this phase runs the real `actions/runner` binary against sockerless and validates the end-to-end flow.

**Backends covered:** ECS + Lambda first (parity with rounds 7-9 live infra). GCF / Cloud Run / ACA / AZF gated on Phase 104's driver framework landing — once the cross-backend driver framework is in, the runner sweeps re-run against every backend with no per-backend code paths.

**Sockerless wiring:**
- `actions/runner` configured with the standard registration token flow (real repo or real org).
- `DOCKER_HOST=tcp://<sockerless>:3375` so every step container, service container, and action runs through sockerless.
- No kubernetes-executor in this phase — that's a follow-up sub-phase if needed.

**Test workloads (canonical):**
1. Single-job, container step (`runs-on: self-hosted` + `container: image=alpine`).
2. Matrix build (3 OS × 2 versions, container per leg).
3. Service container (`services: redis: image=redis:7`) — health check, network reach.
4. Composite action with `actions/checkout`, `actions/setup-go`, `actions/cache`.
5. Artifact upload + download across jobs (`actions/upload-artifact` / `actions/download-artifact`).
6. Secrets injection.
7. Job-failure semantics (a failing step short-circuits subsequent steps, runner reports correct exit code).
8. Step output streaming (live log lines via `docker logs --follow` semantics).

**Cost / live-AWS posture:** time-boxed — provision live infra, run the canonical sweep + 1 real OSS-project workflow, teardown. New per-cloud `null_resource sockerless_runtime_sweep` (BUG-819 fix) means destroys are self-sufficient — no manual cleanup needed.

**Bugs surfaced** are filed BUG-820+ and **fixed in the same phase** (no-defer rule). Golden test fixtures land in `tests/runners/github/`.

### Phase 107 — Real GitLab runner integration

Same shape as Phase 106 but for GitLab Runner with the `docker` executor. The runner registers against gitlab.com (project-scoped) or a self-hosted GitLab; `runners.docker.host` is set to the sockerless DOCKER_HOST.

**Backends covered:** ECS + Lambda first; rest gated on Phase 104.

**Test workloads (canonical):**
1. Single-job pipeline (`image: alpine`, single `script:`).
2. Multi-stage pipeline (build → test → deploy with artifact passing via `artifacts:`).
3. Services (`services: postgres:15` — reach via service name on the network sockerless creates per-job).
4. Cache (`cache.paths` — pull/push to sockerless's image store).
5. `dind` job — `image: docker:cli` + `services: docker:dind` — verify nested docker calls work.
6. Parallel matrix.
7. Manual jobs / `when: on_failure` semantics.
8. Trace streaming (log fidelity; runner's incremental update protocol).

**Sockerless wiring:** docker-executor only this phase. Kubernetes-executor (very common in GitLab self-hosted) is a follow-up under Phase 104 once `backends/core/drivers/` provides the kube-shaped driver dispatcher.

**Bugs surfaced** filed BUG-820+ and fixed in-phase. Fixtures in `tests/runners/gitlab/`.

### Phase 108 — Cross-simulator feature parity audit

Walks every cloud-API surface sockerless touches and verifies the AWS / GCP / Azure simulators implement it at the same fidelity. Driven by `specs/CLOUD_RESOURCE_MAPPING.md` § "Simulator coverage" plus a fresh inventory of every `Op:` in each sim's `*.go` handlers vs the cloud SDK methods sockerless calls. Output: a parity matrix (rows = SDK calls sockerless makes, columns = aws/gcp/azure sim) and a closed-issue list for every gap. Each gap is filed as a BUG and fixed in this phase per the no-defer rule.

The sim binaries already share a `simulators/<cloud>/shared/` package for container execution; any new driver lifted under Phase 104 must land sim parity in the same commit (already a project rule). Phase 108 is the catch-up for parity drift that accumulated before that rule existed.

### Live-cloud validation runbooks

- **Phase 86 Lambda live track** — scripted already; covered partially by round-9 Track D.
- **Phase 87 live-GCP** — needs project + VPC connector; terraform env to add. New per-cloud `null_resource sockerless_runtime_sweep` means destroy is self-sufficient (BUG-819 fix).
- **Phase 88 live-Azure** — needs subscription + managed environment with VNet integration; terraform env to add. Same teardown self-sufficiency.

### Phase 68 — Multi-Tenant Backend Pools (queued)

Named pools of backends with scheduling and resource limits. P68-001 done; 9 sub-tasks remain (registry, router, limiter, lifecycle, metrics, scheduling, limits, tests, state save).

### Phase 78 — UI Polish (queued)

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Webhook delivery UI.
- Cost controls (per-pool spending limits, auto-shutdown).
