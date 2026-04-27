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
10. **Sim parity per commit** — any new SDK call added to a backend must update [specs/SIM_PARITY_MATRIX.md](specs/SIM_PARITY_MATRIX.md) and add the sim handler in the same commit.

## Closed phases

Detail in [WHAT_WE_DID.md](WHAT_WE_DID.md); commit + BUG refs in [BUGS.md](BUGS.md).

| Phase(s) / round | Headline | Bugs closed |
|---|---|---|
| 86–102 (PRs #112–#115) | Sim parity, stateless backends, real volumes, FaaS invocation tracking, reverse-agent exec/cp/diff/commit/pause, Docker pod synthesis, ACA console exec, ECS SSM ops, OCI push, log fidelity. | 661–769 |
| Round-7 (PR #117) | Live-AWS bug sweep | 770–785 |
| Round-8 + Round-9 (PR #118) | Live-AWS bug sweep — stateless invariant, real layer mirror, sync `docker stop`, per-network SG isolation, live SSM frame capture → exit-code marker, `sh -c` exec wrap, busybox-compat find/stat, Lambda invoke waiter, tag-based InvocationResult persistence, per-cloud terragrunt sweep | 786–819 |
| Post-PR-#118 audit + Phase 104 framework + Phase 105 waves 1-3 + Phase 108 + Phase 106/107 prep (PR #120 — open) | Audit pass; Phase 104 framework migration complete (13 typed adapters, every dispatch site routed, framework renamed to drop 104 suffix) + cloud-native typed drivers across every cloud backend (Logs/Attach/Exec/Signal/FS/Commit/ProcList; 44/91 matrix cells cloud-native); `core.ImageRef` typed domain object at the typed Registry boundary; libpod-shape golden tests for 8 handlers; Phase 108 sim-parity matrix audit (33 AWS + 16 GCP + 28 Azure rows ✓); Phase 106/107 real-runner harnesses scaffolded under `tests/runners/{github,gitlab}/`; manual-tests directory + repo-wide Phase/BUG-ref strip from code + docs | 802 / 638-648 retro / 804 / 806 / 820–831 / 832–835 |

## Pending work

Order is the order of execution unless noted.

### Phase 104 — Cross-backend driver framework (in flight)

Lift sockerless's narrow `core.Drivers{Exec, Stream, Filesystem}` plus the bespoke per-backend ad-hoc paths into one pluggable system: every "perform docker action X against the cloud" decision flows through a typed `Driver` interface. **Interfaces in core; implementations live with the cloud they use.** Each backend constructs its `DriverSet` at startup; operators override per-cloud-per-dimension via `SOCKERLESS_<BACKEND>_<DIMENSION>=<impl>`; sim parity required for the default driver in every dimension.

**13 driver dimensions** (kept finer-grained for independent swap):

| Dimension | Default per backend |
|---|---|
| `ExecDriver` ✓ adapter shipped | docker→DockerExec; ECS→SSMExec; Lambda/CR/GCF/AZF→ReverseAgentExec; ACA→ACAConsoleExec |
| `AttachDriver` ✓ adapter shipped | docker→DockerAttach; ECS→CloudWatchAttach; FaaS→CloudLogsReadOnlyAttach |
| `LogsDriver` ✓ adapter shipped | docker→DockerLogs; AWS→CloudWatch; GCP→CloudLogging; Azure→LogAnalytics |
| `SignalDriver` ✓ adapter shipped | docker→DockerKill; ECS→SSMKill; FaaS+CR+ACA→ReverseAgentKill |
| `FSReadDriver` (cp →, stat, get-archive) | docker→DockerArchive; ECS→SSMTar; FaaS+CR+ACA→ReverseAgentTar |
| `FSWriteDriver` (cp ←, put-archive) | docker→DockerArchive; ECS→SSMTarExtract; FaaS+CR+ACA→ReverseAgentTarExtract |
| `FSDiffDriver` | docker→DockerChanges; ECS→SSMFindNewer; FaaS+CR+ACA→ReverseAgentFindNewer |
| `FSExportDriver` | docker→DockerExport; ECS→SSMTarRoot; FaaS+CR+ACA→ReverseAgentTarRoot |
| `CommitDriver` | docker→DockerCommit; FaaS+CR+ACA→ReverseAgentTarLayer+Push; ECS→accepted-gap NotImpl |
| `BuildDriver` | docker→LocalDockerBuild; ECS+Lambda→CodeBuild; CR+GCF→CloudBuild; ACA+AZF→ACRTasks |
| `StatsDriver` | docker→DockerStats; AWS→CloudWatchAggregate; GCP→CloudMonitoring; Azure→LogAnalytics |
| `ProcListDriver` (top) | docker→DockerTop; ECS→SSMPs; FaaS+CR+ACA→ReverseAgentPs |
| `RegistryDriver` (push/pull) | per-cloud: ECRPullThrough+ECRPush; ARPullThrough+ARPush; ACRCacheRule+ACRPush |

**Envelope:** `DriverContext{Ctx, Container, Backend, Region, Logger}` + per-dimension typed Options/Result. Every driver implements `Describe() string` so unset/`NotImpl` slots auto-emit `NotImplementedError` with a precise message.

**Layout:**
```
backends/core/drivers/      # interfaces only
backends/docker/drivers/    # host docker SDK
backends/aws-common/drivers/    # SSM / CodeBuild / ECR
backends/ecs/drivers/       # CloudWatch Logs/Metrics/Attach
backends/aca/drivers/       # ACA console exec
backends/gcp-common/drivers/    # CloudBuild / Cloud Logging / Cloud Monitoring / AR
backends/azure-common/drivers/  # ACR Tasks / Log Analytics / ACR
```

**Refactor delivery — piecemeal, one dimension per commit, no behaviour change per commit.** Each dimension gets: typed driver interface → typed default impl with `Describe()` → `backend_impl.go` reroutes through the driver → sim integration test for the default driver lands in the same commit → bespoke method (e.g. `ContainerExportViaSSM`) deleted. After all dimensions are lifted, the operator-override env-var dispatcher comes online. Spec doc gets a per-backend default-driver matrix table.

**Wrapper-removal pass (post-migration cleanup).** The legacy `WrapLegacyXxx` adapters in `backends/core/driver_adapt_*.go` exist as scaffolding so the transition is no-behaviour-change per commit. Once every backend has a typed cloud-native driver wired in for a given dimension, the corresponding `WrapLegacyXxx` and `LegacyXxxFn` scaffolding gets removed and the equivalent `BaseServer.ContainerXxx` method on the api.Backend interface is shrunk to a thin proxy or removed entirely (depending on whether non-typed callers exist). This is a coordinated cleanup, not piecemeal — it has to land after the last backend is migrated.

**Stronger type safety, post-cleanup.** Several typed driver interfaces still carry `interface{}` / loosely-typed values inherited from the legacy api.Backend shape — e.g. `BuildOptions` previously dropped fields, `Stats` returns map[string]any. Once the wrappers are removed the interfaces get a tightening pass: replace string-keyed maps with typed structs, introduce typed enums where the docker API only has strings (e.g. `Signal`, `RestartCondition`), surface domain types (`ImageRef` instead of `string`) where parsing currently happens at every callsite. The bar: the typed interfaces should be obviously correct from their signatures alone — no need to consult the impl to know what's allowed.

**`ImageRef` migration (sub-track of the type tightening).** Image references are currently passed as bare `string` and re-parsed at every callsite (10+ ad-hoc parses across `backends/core/{registry,backend_impl,handle_docker_api}.go`, `backends/docker/backend_impl.go`, `backends/aws-common/build.go`, etc.). Introduce `core.ImageRef{Domain, Path, Tag, Digest}` with `ParseImageRef(s) (ImageRef, error)` + `String()`, change the typed `RegistryDriver.Push/Pull` signatures to take `ImageRef`, and migrate every ad-hoc parse site to use the canonical type. The handler parses once at the dispatch boundary; everything downstream gets typed access. This is its own coordinated PR, not piecemeal — partial migration leaves the codebase with two parsers.

**Sim contract:** every default driver must work end-to-end against its cloud's simulator. Alternate drivers (Kaniko, OverlayUpper) may be operator-installable only.

**Phase 103 (overlay-rootfs bootstrap)** ships under Phase 104 as alternate FSRead/Write/Diff/Export/Commit drivers gated behind `SOCKERLESS_OVERLAY_ROOTFS=1`. Replaces Phase 98's `find / -newer /proc/1` heuristic with overlayfs upper-dir for diff/commit/cp/export on every backend that ships a sockerless bootstrap (Lambda, Cloud Run, ACA, GCF, AZF). Captures deletions as whiteouts (closes the BUG-750 known limitation). Out of scope on ECS — operator's image runs as-is, no bootstrap insertion point.

### Phase 105 — Libpod-shape conformance (rolling — waves 1-3 landed)

`podman` CLI uses bindings that expect specific JSON shapes. **Waves 1-3 landed** in PRs #119/#120: BUG-804 (`PodInspectResponse` mirrors `define.InspectPodData`), BUG-806 (`PodStop`/`PodKill` Errs normalised; per-container failures via HTTP 409 ErrorModel), plus golden shape tests for `info`, `containers/json`, rm-report, `images/pull` stream, `networks/json`, `volumes/json`, `system/df` — 8 handlers covered.

**Wave 4 remaining** (lower priority): events stream, exec start hijack, container CRUD beyond list. Verify against a real podman client (currently no live podman in CI). Can run in parallel with Phase 104.

### Phase 106 — Real GitHub Actions runner integration (in flight)

End-to-end test of GitHub Actions self-hosted runners pointed at sockerless via `DOCKER_HOST`. The repo already has *simulated* runner E2E tests (`tests/github_runner_e2e_test.go` replays the runner's Docker REST sequence); this phase runs the real `actions/runner` binary against sockerless and validates the live flow.

**Harness shipped.** [`tests/runners/github/harness_test.go`](tests/runners/github/harness_test.go) gates on `SOCKERLESS_GH_RUNNER_TOKEN` + `SOCKERLESS_GH_REPO`, downloads the `actions/runner` tarball, configures + registers the runner, dispatches a workflow via `gh api`, polls until completion, and unregisters cleanly on exit. Build-tag-gated (`github_runner_live`) so default `go test ./...` doesn't try to download anything. Sample workflows in [`tests/runners/github/workflows/`](tests/runners/github/workflows/).

**What's left:** running the harness against a real GitHub repo + live ECS infrastructure, filling out the canonical sweep below, capturing first findings as bugs.

**Architecture.** Runner is long-lived on a runner host (small VM, container, or laptop) with `DOCKER_HOST=tcp://sockerless:2375`. Stock runner binary — no fork. Every step container, service container, and `uses: docker://` action gets intercepted by sockerless and dispatched to the configured backend. Jobs are ephemeral (run inside ECS or Lambda); the runner itself stays up.

**Backend routing — two paths:**
- **(a) Per-backend daemon (v1).** One sockerless instance per backend, each on its own port. The runner host runs one self-hosted runner per `runs-on:` label; each runner uses a different DOCKER_HOST. Simple, no new code, but you register two runners per host.
- **(b) Single daemon, label-based dispatch (v2 — Phase 68 follow-up).** Sockerless reads a `SOCKERLESS_LABEL_TO_BACKEND` map and routes `/containers/create` per the label header the runner sends. Lines up with Phase 68 (Multi-Tenant Backend Pools). Lands once Phase 68 is unblocked.

**ECS vs Lambda dispatch.** ECS is the default for everything: long-running, exec-able, multi-step, services, cache. Lambda fits *fast one-shots* only (≤15 min, no service container, no `docker attach` mid-stream) — best for container actions, lint, fast unit tests.

**Backends covered.** ECS + Lambda first (parity with rounds 7-9 live infra). GCF / Cloud Run / ACA / AZF gated on Phase 104's driver framework — once cross-backend drivers are typed, runner sweeps re-run against every backend with no per-backend code paths.

**Test workloads (canonical, ECS unless noted):**
1. Single-job, container step (`runs-on: self-hosted` + `container: image=alpine`) — both ECS + Lambda.
2. Matrix build (3 OS × 2 versions, container per leg).
3. Service container (`services: redis: image=redis:7`) — health check, network reach.
4. Composite action with `actions/checkout`, `actions/setup-go`, `actions/cache`.
5. Artifact upload + download across jobs.
6. Secrets injection.
7. Job-failure semantics (failing step short-circuits; runner reports correct exit code).
8. Step output streaming (live log lines via `docker logs --follow`).
9. Container action (`uses: docker://alpine:latest`) — Lambda candidate (one-shot, fast).

**Cost / live-AWS posture.** Time-boxed — provision live infra, run the canonical sweep + 1 real OSS-project workflow, teardown. Per-cloud `null_resource sockerless_runtime_sweep` (BUG-819) means destroys are self-sufficient.

**Test scaffolding.** `tests/runners/github/harness_test.go` gates on `SOCKERLESS_GH_RUNNER_TOKEN` + `SOCKERLESS_GH_REPO` env vars; downloads `actions/runner`, configures it, dispatches a workflow via `gh api repos/.../dispatches`, polls completion. Sample workflow YAMLs in `tests/runners/github/workflows/`.

**Bugs surfaced** filed BUG-836+ and **fixed in the same phase** (no-defer rule).

### Phase 107 — Real GitLab runner integration (in flight)

Same shape as Phase 106 but for GitLab Runner with the `docker` executor. Registered against gitlab.com (via the project's `origin-gitlab` mirror) or a self-hosted GitLab; `runners.docker.host` = sockerless DOCKER_HOST.

**Harness shipped.** [`tests/runners/gitlab/harness_test.go`](tests/runners/gitlab/harness_test.go) gates on `SOCKERLESS_GL_RUNNER_TOKEN` + `SOCKERLESS_GL_PROJECT` + `SOCKERLESS_GL_API_TOKEN`, registers `gitlab-runner` with `--executor docker --docker-host $DOCKER_HOST`, commits a `.gitlab-ci.yml`, triggers a pipeline via the projects API, polls until terminal, and unregisters on cleanup. Build-tag-gated (`gitlab_runner_live`). Sample pipelines in [`tests/runners/gitlab/pipelines/`](tests/runners/gitlab/pipelines/).

**Mirror-side prep.** The `origin-gitlab` mirror needs CI enabled — likely a one-time settings flip. Alternative: a self-hosted GitLab CE in a test container (heavier, isolated).

**Backends covered.** ECS + Lambda first; rest gated on Phase 104.

**Test workloads (canonical):**
1. Single-job pipeline (`image: alpine`, single `script:`).
2. Multi-stage pipeline (build → test → deploy with artifact passing via `artifacts:`).
3. Services (`services: postgres:15` — reach via service name on the per-job network sockerless creates).
4. Cache (`cache.paths` — pull/push to sockerless's image store).
5. `dind` job — `image: docker:cli` + `services: docker:dind`.
6. Parallel matrix.
7. Manual jobs / `when: on_failure` semantics.
8. Trace streaming.

**Sockerless wiring.** Docker-executor only this phase. Kubernetes-executor (very common in GitLab self-hosted) is a follow-up under Phase 104 once `backends/core/drivers/` provides the kube-shaped driver dispatcher.

**Test scaffolding.** `tests/runners/gitlab/harness_test.go` gates on `SOCKERLESS_GL_RUNNER_TOKEN` + `SOCKERLESS_GL_PROJECT`; downloads `gitlab-runner`, registers + runs.

**Bugs surfaced** filed BUG-836+ and fixed in-phase.

### Phase 109 — Strict cloud-API fidelity audit across all sims (in flight)

Audit-driven sweep against the rule **"no fakes, no fallbacks, no synthetic data; sim shape must match the real cloud API end-to-end"**. Triggered by PR #120 CI failures that traced back to synthetic responses (hardcoded subnet IDs, hardcoded private IPs, AgentMessage-frame stdin dropped on the floor, etc). Goal: every sim slice sockerless touches behaves like the real cloud — same wire shape, same validation rules, same state transitions, same SDK / CLI / Terraform-provider compatibility.

**Why now.** Sockerless's runner-coverage phases (106 + 107) drive workloads that exercise the sims at much higher fidelity than the SDK/CLI matrix has so far — image pulls + service containers + multi-step shells + binary stdin (act / setup-go / actions/cache). Every fake the runner trips becomes a live-cloud bug we'd hit on the real cloud. Better to stamp them out in the sim now.

**Scope (closed in PR #120 — already shipped):**
- BUG-836: AWS sim ECS task lifecycle ran the real container only when `awslogs` was configured — synthetic RUNNING-forever for log-less tasks. Fixed: container starts unconditionally; `discardLogSink` carries the path when no log driver is configured.
- BUG-839: Azure sim every site shared `r.Host` as DefaultHostName — multi-site routing collided. Fixed: per-site `<name>.azurewebsites.net` matching real Azure.
- BUG-842: AWS sim SSM session ignored `input_stream_data` AgentMessage frames — binary stdin (tar, gzip) never reached the user process. Fixed: simulator now decodes the AgentMessage protocol matching real ssm-agent.
- BUG-844: AWS sim ECS RunTask returned hardcoded subnet ID `subnet-sim00001` and `10.0.<i>.<i+100>` IPs ignoring the request — fixed: subnet must exist in EC2 sim store, IP allocated from its real CidrBlock.
- AWS sim EFS `CreateMountTarget` IPs from real subnet CIDR (same fix shape as ECS).
- AWS sim default subnet ID renamed to AWS-shape `subnet-0123456789abcdef0` so it round-trips through the AWS CLI's parameter validator (min length 15).

**Closed in PR #121** (rolled into the in-flight sweep):

1. ✓ **AWS sim — Lambda VpcConfig from real subnet CIDR.** CreateFunction (and UpdateFunctionConfiguration) accept VpcConfig and allocate a Hyperplane ENI per SubnetId from the subnet's actual CidrBlock via AllocateSubnetIP. Real Lambda validates the subnets exist; the sim now matches that contract instead of dropping VpcConfig.
2. ✓ **AWS sim — region / account scoping.** New `simulators/aws/aws_identity.go` centralizes `awsRegion()`, `awsAccountID()`, `awsAvailabilityZone()`. Operator-configurable via `SOCKERLESS_AWS_REGION` and `SOCKERLESS_AWS_ACCOUNT_ID`. Every ARN-emitting handler — ECS, Lambda, ECR, CloudWatch, CloudMap, EFS, IAM, S3, STS, EC2 — reads through these helpers instead of hardcoded literals.
3. ✓ **GCP sim — `compute.firewalls` resource.** Was completely missing — `terraform-provider-google`'s `google_compute_firewall` and the Go SDK's `FirewallsRESTClient` 404'd against the sim. Added Create/Get/List/Delete/Patch handlers + `ComputeFirewall` type matching real `compute#firewall` shape (Network, Direction, Priority, SourceRanges, SourceTags, TargetTags, Allowed[]/Denied[], LogConfig).
4. ✓ **GCP sim — `iam.serviceAccounts.generateAccessToken`.** Was missing; `gcloud auth application-default` and `google-github-actions/auth` call this to mint scoped OAuth tokens. Added `POST /v1/projects/{p}/serviceAccounts/{email}:generateAccessToken` returning a deterministic `ya29.sim-<uuid>` placeholder + 1-hour RFC3339 expireTime.
5. ✓ **Azure sim — IMDS metadata token endpoint.** `DefaultAzureCredential`/`ChainedTokenCredential`/`gh-action-azure-login` all call either `http://169.254.169.254/metadata/identity/oauth2/token` (VM IMDS) or `$IDENTITY_ENDPOINT` (App Service / Container Apps). Both routes added under `managedidentity.go`; shared handler validates `resource` and returns the standard token JSON shape.
6. ✓ **Azure sim — Blob Container ARM control plane.** `Microsoft.Storage/storageAccounts/{a}/blobServices/default/containers/{name}` was missing — `azurerm_storage_container` and `armstorage.NewBlobContainersClient` 404'd. Added PUT/GET/DELETE/LIST handlers + `BlobContainer`/`BlobContainerProps` types matching the ARM shape; defaults match real-Azure-on-create.
7. ✓ **Azure sim — NSG rule priority+direction uniqueness.** Real Azure rejects duplicate `(Priority, Direction)` pairs within an NSG with `SecurityRuleParameterPriorityAlreadyTaken`. The sim silently overwrote, masking misconfigurations. PUT now walks existing rules + 400s on a collision; same priority across different directions is still accepted (real-Azure behavior — priority space is per-direction).
8. ✓ **Azure sim — Private DNS AAAA/CNAME/MX/PTR/SRV/TXT records.** Was A-only. RecordSetProperties extended with the standard ARM record-type slices/singletons; generic CRUD loop registers PUT/GET/DELETE/LIST for every type. `azurerm_private_dns_{cname,mx,txt,aaaa,ptr,srv}_record` and the SDK's RecordSetsClient (any non-A type) now round-trip.

**Scope (still pending — sequencing):**

9. **Azure sim — Container Apps + Jobs `provisioningState` machine.** Real Azure: PUT returns 201 Created with `provisioningState=Creating` + `Azure-AsyncOperation` header; the SDK poller follows the header until `status=Succeeded`. Sim returns `Succeeded` directly. Proper fix needs a `/providers/Microsoft.App/locations/{loc}/operationStatuses/{id}` polling endpoint that returns InProgress→Succeeded. (Current state is the wider end of acceptable real-Azure shape — Azure does have a sync fast-path — but the proper async flow is on the to-do list.)
10. **GCP sim — Cloud Run jobs state machine.** Job `TerminalCondition.State = "CONDITION_SUCCEEDED"` on create. Real Cloud Run goes `CONDITION_INITIALIZING` → `BUILDING` → `DEPLOYING` → `SUCCEEDED`. Same shape as item 9.
11. **GCP sim — `compute.routers` + Cloud NAT** for serverless egress.
12. **Azure sim — `Microsoft.Network/natGateways`** + route tables for custom routing.
13. **All sims — timestamps + state stamps respect-on-write.** Several handlers re-set `CreatedAt = time.Now()` on every read; should stamp at write.
14. **No-fakes audit pass on test fixtures.** Repo-wide grep for `subnet-*`, `vpc-*`, `arn:aws:*`, `projects/test-*`, `00000000-...` style IDs in tests — every test must either use a sim-pre-registered ID or create the resource via the sim's CRUD API first.

**Per-bug log lives in [BUGS.md](BUGS.md) round-10.** Each item lands as its own commit with: typed sim handler change → SDK/CLI test pass against the new contract → no-fakes regression test added.

### Live-cloud validation runbooks

Per-cloud `null_resource sockerless_runtime_sweep` (BUG-819) makes every backend's `terragrunt destroy` self-sufficient.

- **Lambda live track** — scripted; partially covered by round-9 Track D.
- **Live-GCP** — needs project + VPC connector; terraform env to add.
- **Live-Azure** — needs subscription + managed environment with VNet integration; terraform env to add.

### Phase 68 — Multi-Tenant Backend Pools (queued)

Named pools of backends with scheduling and resource limits. P68-001 done; 9 sub-tasks remain (registry, router, limiter, lifecycle, metrics, scheduling, limits, tests, state save). Fold in Phase 106's label-based-dispatch as the headline use case.

### Phase 78 — UI Polish (queued)

Dark mode, design tokens, error handling UX, container detail modal, auto-refresh, performance audit, accessibility, E2E smoke, documentation.

## Future ideas

- GraphQL subscriptions for real-time event streaming.
- Full GitHub App permission scoping.
- Webhook delivery UI.
- Cost controls (per-pool spending limits, auto-shutdown).
