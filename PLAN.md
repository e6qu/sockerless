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
| Post-PR-#118 audit + Phase 104 framework + Phase 105 waves 1-3 + Phase 108 + Phase 106/107 prep (PR #120) | Audit pass; Phase 104 framework migration complete (13 typed adapters, every dispatch site routed, framework renamed to drop 104 suffix) + cloud-native typed drivers across every cloud backend (Logs/Attach/Exec/Signal/FS/Commit/ProcList; 44/91 matrix cells cloud-native); `core.ImageRef` typed domain object at the typed Registry boundary; libpod-shape golden tests for 8 handlers; Phase 108 sim-parity matrix audit (33 AWS + 16 GCP + 28 Azure rows ✓); Phase 106/107 real-runner harnesses scaffolded under `tests/runners/{github,gitlab}/`; manual-tests directory + repo-wide Phase/BUG-ref strip from code + docs | 802 / 638-648 retro / 804 / 806 / 820–831 / 832–835 / 836–844 |
| Phase 109 strict cloud-API fidelity audit (PR #121) | 19 audit items: Lambda VpcConfig from real subnet CIDR, region/account scoping, AWS Secrets Manager + SSM Parameter Store + KMS + DynamoDB, GCP `compute.firewalls` + `compute.routers`/Cloud NAT + `iam.generateAccessToken` + operations endpoint persistence, Azure IMDS token endpoint + Blob Container ARM CRUD + NSG priority+direction validation + Private DNS AAAA/CNAME/MX/PTR/SRV/TXT records + NAT Gateways + Route Tables + Container Apps/Jobs Azure-AsyncOperation polling + Key Vault ARM+data plane + ARM `SystemData.createdAt` preservation. No-fakes audit on test fixtures clean. | (audit items, no new BUG numbers) |

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

### Phase 110 — Real GitHub + GitLab runner integration (in flight; split into 110a + 110b)

Phase 110 collapses Phases 106 (GitHub) and 107 (GitLab) into one execution stream against ECS + Lambda backends. Architecture and token strategy live in [`docs/RUNNERS.md`](docs/RUNNERS.md) (canonical) — Phase 106/107 sections below remain as the per-runner reference. Branch: `phase-110-runner-integration` (PR #122) for 110a; subsequent branch for 110b.

**Coverage matrix — 4 cells. All required by the end of 110b.**

| Cell | Runner | Backend | Sockerless port | Runner label / tag | Lands in |
|---|---|---|---|---|---|
| 1 | GitHub `actions/runner` | ECS Fargate | `:3375` | `sockerless-ecs` | 110b |
| 2 | GitHub `actions/runner` | AWS Lambda | `:3376` | `sockerless-lambda` | 110b |
| 3 | `gitlab-runner` (docker exec) | ECS Fargate | `:3375` | `sockerless-ecs` | 110a |
| 4 | `gitlab-runner` (docker exec) | AWS Lambda | `:3376` | `sockerless-lambda` | 110a |

**Architectural split — why two halves.**

GitLab Runner is a **dispatcher pattern**: the runner master polls GitLab, and for each job it uses the docker executor to spawn a *job container* via `docker create + docker exec`. The master is just a docker client; it never bind-mounts its own filesystem into the job container; it doesn't need to be co-located with the workload. So **gitlab-runner master can run on the laptop**, point its `--docker-host` at sockerless, and every job container becomes an ECS task or Lambda invocation. **No new sockerless code needed for cells 3 + 4.**

GitHub Actions Runner is a **worker pattern**: the runner *is* the workspace. Its filesystem holds the checkout, the action sources, the job artifacts. For `container:`-using jobs it runs `docker create` with **host bind mounts** (`/home/runner/_work` → `/__w`, `/var/run/docker.sock`, etc.), assuming a shared filesystem between runner and job container. On Fargate / Lambda, two tasks don't share filesystems by default. So GitHub runners can't run as a local laptop process and dispatch jobs to Fargate via sockerless — the bind-mount semantics break. **Cells 1 + 2 require a sockerless code change** and a different topology (runner-as-ECS-task, EFS-backed workspace, sockerless sidecar, bind-mount → EFS translation).

**Token strategy (both halves).** No long-lived tokens in env vars / project settings / disk plaintext / shell history. PATs in macOS Keychain (`gh` keychain-backed for GitHub; `security(1)` entry for GitLab). Runner registration tokens minted per harness run via the platform API (`gh api .../registration-token` for GitHub; `POST /api/v4/user/runners` for GitLab) and deleted on harness exit. Self-healing cleanup: each run starts by deleting any leftover `sockerless-*` runners from a previous crash. Full detail in `docs/RUNNERS.md`.

**Workflow / pipeline trigger discipline.**
- GitHub workflows under `.github/workflows/sockerless-runner-*.yml` use **only** `workflow_dispatch:` and `pull_request: paths: ['tests/runners/**']`. Never trigger on push to main.
- GitLab pipelines kept isolated under `tests/runners/gitlab/pipelines/`; harness triggers via `POST /projects/:id/pipeline` with `ref` set to a throwaway branch.

#### Phase 110a — GitLab cells + dispatcher skeleton (closes PR #122)

Two deliverables, neither blocked on the other:

1. **Cells 3 + 4 — GitLab × ECS / GitLab × Lambda against live infra.** No new code. The GitLab harness already runs `gitlab-runner` locally with the docker executor. Each test cell mints a runner authentication token via `POST /api/v4/user/runners`, registers the runner with `--docker-host tcp://localhost:3375` (or `:3376`), commits a per-cell pipeline YAML on a throwaway branch, triggers a pipeline, polls to success, then unregisters + deletes the runner + branch. Headline value: validates that the live ECS + Lambda backends translate `docker create + exec` (the gitlab-runner pattern) end-to-end against real Fargate / Lambda invocations.

2. **`github-runner-dispatcher` top-level module skeleton.** A new sibling Go module at the repo root (own `go.mod`, independent dep tree, builds standalone). Coupled **only to the public Docker API / CLI** — zero awareness of sockerless. The dispatcher pointed at any docker daemon (local Podman, Docker Desktop, or sockerless via `DOCKER_HOST=tcp://…`) does the same thing: poll GitHub, spawn runner containers, exit. Sockerless is invisible to it.

   - **Mandatory `--repo` flag** (no default — explicit).
   - **`gh auth token` + explicit scope verification at startup**, fail loud with full instructions on missing scopes.
   - **Stateless poller**: `GET /repos/{repo}/actions/runs?status=queued` + per-run `GET .../jobs` every 15 s. Dedup via seen-set with 5-min TTL.
   - **Per-job spawner**: `docker run --pull never <runner-image-uri> -e RUNNER_REG_TOKEN=… -e RUNNER_LABELS=…`. Uses Docker SDK; `DOCKER_HOST` environment dictates target daemon.
   - **60-s idle timeout** enforced inside the runner image's entrypoint script — if `actions/runner` stdout doesn't show "Running job:" within the window, the entrypoint kills the runner. Cleans up duplicate-spawn races without dispatcher state.
   - **Sockerless-daemon liveness check**: skip the poll cycle if `DOCKER_HOST` is unreachable; log a warning. Don't crash, don't auto-start.
   - **Config file** at `~/.sockerless/dispatcher/config.toml` mapping label → `{daemon URL, runner image URI}`. CLI flags can override.
   - **Failure handling**: log + skip on spawn errors. Stateless. Job stays queued; retried next poll.
   - **Logs to stdout only** at this stage (no `/metrics`, no `/healthz` — laptop-foreground binary).

   Skeleton compiles + passes a smoke test against local Podman in 110a; full wiring + ECR push lands in 110b.

110a closes when cells 3 + 4 pass and the dispatcher module compiles + smoke-tests cleanly.

#### Phase 110b — GitHub cells, sockerless EFS feature, dispatcher fully wired

**The bind-mount → EFS translation feature in sockerless ECS + Lambda backends.** This is the headline 110b deliverable.

When a docker client (the actions/runner inside an ECS task or Lambda invocation) calls `docker create -v src:dst`:
1. Sockerless identifies the caller via task metadata (ECS `169.254.170.2/v4/`) or function context (Lambda).
2. Looks up the caller's volume mounts (ECS task definition / Lambda `FileSystemConfigs`).
3. If `src` matches a `containerPath` of an EFS-backed volume in the caller, rewrite the bind mount to a named volume reference for the same EFS access point in the spawned sub-task.
4. Special-case `/var/run/docker.sock`: drop the mount, inject `DOCKER_HOST=tcp://<sidecar-ip>:3375` env in the sub-task so nested `docker run` works.
5. Sub-task runs in the same ECS cluster, mounts the same EFS access point, sees the same workspace data — `container:` directive works end-to-end.

**Runner workload topology** — the runner runs as an ECS task (cell 1) or Lambda invocation (cell 2):

- **Pre-registered ECS task definition** (in Terraform under `terraform/environments/runner/live/`) — multi-container shape (runner + sockerless-backend-ecs sidecar), one EFS volume mounted at `/home/runner/_work`, IAM role + log config + networking all owned by Terraform. Sockerless's ECS backend recognizes the runner image (via `LABEL com.sockerless.ecs.task-definition-family=sockerless-runner` set at Dockerfile build time) and calls `RunTask --task-definition sockerless-runner:LATEST` with per-job env-var container overrides for `REG_TOKEN` / `LABELS` / `RUNNER_NAME`. **No dynamic task def composition for the runner-task** — operator owns the spec. (Job-tasks the runner subsequently spawns via `container:` keep dynamic composition; that's where the EFS-bind-mount feature plugs in.)
- **Lambda function** (cell 2) — same runner image; `FileSystemConfigs` mounts EFS at `/home/runner/_work`. The runner's `DOCKER_HOST` points at sockerless ECS daemon (Lambda doesn't get a sockerless sidecar — Lambda runs one container per invocation). Cell 2 is restricted to non-`container:` workflows (steps run in the runner's Lambda filesystem; only `run: docker run …` steps go through sockerless). Documented limitation, not a fake — Lambda's invocation model doesn't support sibling containers.

**Runner image** — pushed to ECR via a new `sockerless-runner` repo (separate Terraform module from the existing `sockerless-live` ECR repo to avoid mixing per-task images with long-lived runner images). Image carries `LABEL com.sockerless.ecs.task-definition-family=sockerless-runner` so sockerless picks the right pre-registered task def. `:latest` tag during dev iteration; switch to versioned tags + bumped task-def revisions post-Phase 110.

**Dispatcher fully wired** — same `github-runner-dispatcher` binary as 110a, now configured (via the TOML config) to point at the live ECR runner image URI + `tcp://localhost:3375` / `:3376` daemons. The dispatcher is unchanged; it just learned a new image URI.

**Test workloads (per cell):**
- `hello` — single-job `echo $RUNNER_NAME` + `uname -a`. Smoke / wiring sanity. All 4 cells.
- `container-step` — workflow with `container: alpine:latest` and a `run:` step inside. Exercises the bind-mount → EFS translation. Cells 1 + 3 (ECS); cell 2 only if the workflow's runtime fits in 14 min.
- `gotest` — `actions/checkout` + `setup-go` + `go test -count=1 ./...` against a tiny Go module. Multi-step exec, real artifact pull-down. Cell 1 (ECS) only on first pass.
- `service-job` — `services: postgres:16` connectivity. Cross-container DNS via Cloud Map. Cell 1 (ECS) only.

**Bug discipline.** Every harness run that fails for a sockerless reason files a BUG in [BUGS.md](BUGS.md) (`Open` section), gets fixed in-branch, and the entry moves to the relevant `Resolved` section. Per the no-defer / no-fakes rule.

**Phase 110 succeeds when** all 4 cells have a green run on file (workflow runs visible in github.com / gitlab.com Actions UIs), and any bug surfaced during the run has a closed entry in BUGS.md. Capability matrix at [`docs/runner-capability-matrix.md`](docs/runner-capability-matrix.md) gets the cells updated from `TBD` to `PASS`/`FAIL` per workload.

#### Phase 110 — paths forward to GREEN (2026-04-29)

Concrete unblock plan per cell. Source corrections shipped at commit `8c70d1a`; remaining work is operator-driven runtime steps (apply, build, restart) and a re-test sweep.

##### Cell 1 — GitHub × ECS — ✅ GREEN

No further work; reference run https://github.com/e6qu/sockerless/actions/runs/25075259911 (2026-04-28 20:13 UTC). Re-runs on the same task-def + image are expected to PASS. Will be re-fired during the cells-2/3/4 sweep as a regression check.

##### Cell 2 — GitHub × Lambda — ❌ → unblock via 4 steps

Failure: run [25075247501](https://github.com/e6qu/sockerless/actions/runs/25075247501) hit "host bind mounts not supported on ECS backend (`/tmp/runner-state/externals:/__e:ro`)". Surfaced **BUG-861** (missing externals shared-volume entry) which led to **BUG-862** (CRITICAL, runner-Lambda baking the wrong backend). Source-side corrections:

- Dockerfile + bootstrap.sh now use `sockerless-backend-lambda`.
- Terraform IAM swapped from ECS dispatch perms to Lambda dispatch perms (`lambda:CreateFunction/Invoke/Delete/Get/UpdateConfiguration/Tag/ListFunctions`); env vars all `SOCKERLESS_LAMBDA_*`; SHARED_VOLUMES carries both workspace + externals to the same EFS access point.
- New CodeBuild project + S3 build-context bucket so the in-Lambda backend can build sub-task images at runtime without a docker daemon.
- `tests/runners/github/dockerfile-lambda/` Dockerfile now stages `sockerless-agent` + `sockerless-lambda-bootstrap` into `/opt/sockerless/` (image-inject prerequisite for the in-Lambda backend).

**Steps to GREEN:**
1. `cd terraform/environments/lambda/live && source aws.sh && terragrunt apply` — provisions `sockerless-live-image-builder` (CodeBuild) + `sockerless-live-build-context` (S3) + new IAM/env vars on `sockerless-live-runner`. Validates clean today.
2. `cd tests/runners/github/dockerfile-lambda && make codebuild-update` — `make stage` cross-compiles linux/amd64 backend + agent + bootstrap into the build context; `make codebuild-build` tars + S3-uploads the context, triggers CodeBuild, polls until SUCCEEDED; `make update-function` does `aws lambda update-function-code --publish`; `make wait` blocks on `function-updated-v2`. Pure-AWS, no local Docker required. (Local-Docker alternative: `make all`.)
3. Re-run `go test -tags github_runner_live -run TestGitHub_Lambda_Hello -timeout 25m ./tests/runners/github`. Expected: hello workflow runs to GREEN; the `container: alpine:latest` directive triggers the in-Lambda `sockerless-backend-lambda` to spawn an alpine sub-task Lambda (image-mode container) sharing the workspace EFS access point.
4. If new bugs surface (likely candidates: image-inject layer-pull perms; sub-task Lambda VPC config inheriting ENI cap), file in BUGS.md immediately and fix in-branch — no deferral.

##### Cell 3 — GitLab × ECS — ⏸ blocked → unblock via restart

All gitlab-runner harness pre-reqs verified (PAT in keychain, project resolves, runner mint works). Code fix (BUG-859 — `ecsStdinAttachDriver` + `launchAfterStdin`) committed at `c10a317`; the macOS-arm64 fix binary is at `/tmp/sockerless-backend-ecs`. Running PID 75092 still holds pre-fix code via mmap.

**Steps to GREEN:**
1. Restart sockerless ECS — see DO_NEXT.md § "Sockerless restart command". One shell block.
2. `go test -tags gitlab_runner_live -run TestGitLab_ECS_Hello -timeout 30m ./tests/runners/gitlab`. Expected: helper-image pull (BUG-857 fix) → predefined-container env prep + git clone + cleanup (BUG-858 fix) → user-script alpine container with stdin script delivered via the new attach-stdin pipe (BUG-859 fix); `hello from sockerless ecs` + `env | sort` appears in the GitLab job trace.
3. Document the GitLab pipeline URL in DO_NEXT.md. Promote BUG-859 to confirmed-fixed after the green run lands.

##### Cell 4 — GitLab × Lambda — ⏸ blocked → unblock via restart + verify same pattern as cell 3

Local gitlab-runner pointed at sockerless on `:3376`. Code fix (BUG-860 — `lambdaStdinAttachDriver` + deferred-Invoke baking stdin into Payload) committed at `6e3d0fa`; macOS-arm64 fix binary at `/tmp/sockerless-backend-lambda`. PID 70870 still on pre-fix code.

**Caveat — the laptop sockerless-backend-lambda needs the agent + bootstrap binaries on disk** for its own image-inject path (used when gitlab-runner spawns `container:` sub-tasks). Current default paths: `/opt/sockerless/sockerless-agent` + `/opt/sockerless/sockerless-lambda-bootstrap`. On the laptop those will not exist; the user must either:
- Copy the linux/amd64 binaries from `tests/runners/github/dockerfile-lambda/` to `/opt/sockerless/` (the laptop sockerless dispatches builds to CodeBuild, so linux/amd64 is correct even on macOS), OR
- Set `SOCKERLESS_AGENT_BINARY` + `SOCKERLESS_LAMBDA_BOOTSTRAP` env vars in `/tmp/lambda-env.sh` to point at the staged copies, OR
- Provision a sockerless-lambda-runner-on-laptop config that uses the in-cloud CodeBuild project for builds (which already has `SOCKERLESS_AGENT_BINARY`/etc. baked into the image).

**Steps to GREEN:**
1. Pre-stage agent + bootstrap binaries (one-time): copy from `tests/runners/github/dockerfile-lambda/` to `/opt/sockerless/`, OR add the env vars to `/tmp/lambda-env.sh`.
2. Restart sockerless Lambda — same restart block as cell 3, just the `:3376` half.
3. Set `SOCKERLESS_CODEBUILD_PROJECT=sockerless-live-image-builder` + `SOCKERLESS_BUILD_BUCKET=sockerless-live-build-context` in `/tmp/lambda-env.sh` so the laptop sockerless can build sub-task images via CodeBuild.
4. `go test -tags gitlab_runner_live -run TestGitLab_Lambda_Hello -timeout 30m ./tests/runners/gitlab`. Expected: gitlab-runner registered with `--docker-host tcp://localhost:3376`, helper image pulled to ECR-routed Lambda creation, predefined helper Lambda + user-script alpine sub-task Lambda with stdin script in Payload (BUG-860 path), `hello from sockerless lambda` + `date -u` in the GitLab job trace.
5. Document new bugs as they surface; expected candidates: laptop sockerless-lambda + ECR pull-through cache auth from outside-AWS (different network shape than the runner-Lambda case), Lambda function name length / character set issues with gitlab-runner's helper-container naming (`runner-<id>-project-<id>-...`).

##### Cell sweep — single-shot 4-cell verification

Once cells 2/3/4 individually pass, fire all four in sequence to confirm no cross-cell interference:

```bash
go test -v -tags github_runner_live -run TestGitHub_ECS_Hello   -timeout 30m ./tests/runners/github
go test -v -tags github_runner_live -run TestGitHub_Lambda_Hello -timeout 30m ./tests/runners/github
go test -v -tags gitlab_runner_live -run TestGitLab_ECS_Hello    -timeout 30m ./tests/runners/gitlab
go test -v -tags gitlab_runner_live -run TestGitLab_Lambda_Hello -timeout 30m ./tests/runners/gitlab
```

Capture all 4 run/pipeline URLs in DO_NEXT.md. Update `docs/runner-capability-matrix.md` cells from TBD to PASS. Phase 110 closes.

### Phase 111 — Workload identity for runner jobs (queued; gated on Phase 110 closure)

Once the Phase 110 4-cell harness runs end-to-end, the next gap is **workload identity inside the per-job container** — making `aws sts get-caller-identity`, `gcloud auth print-identity-token`, and `az account show` resolve to a real cloud identity from inside a sockerless-dispatched container, exactly as they would on a native cloud runner. Without this, runner jobs that touch cloud APIs (the most common real-world pattern: `aws s3 cp`, `gcloud builds submit`, `az acr login`) would either fail or fall back to PAT-style env var creds, which violates the "no fakes, no fallbacks" rule for end-user workloads.

**What "minimal" means.** The user's job calls `aws sts get-caller-identity` (or equivalent). The CLI inside the container resolves credentials via the platform's standard discovery path. Sockerless ensures that path is actually wired to the cloud workload's identity (task role / function role / service account / managed identity).

**Per-backend wiring — what each cloud already provides.** Real cloud runtimes hand the workload an identity through a standard endpoint; sockerless's only job is to make sure the backend attaches the right role / SA / MI when it provisions the workload, and that the standard endpoints are reachable from inside the container.

| Backend | Native discovery path | What sockerless attaches | Gap to verify |
|---|---|---|---|
| ECS Fargate | `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI` → `http://169.254.170.2/v2/credentials/<id>` | `taskRoleArn` (and `executionRoleArn`) on the task definition | TaskRole must include `sts:GetCallerIdentity` + whatever the workload actually needs |
| AWS Lambda | env vars `AWS_ACCESS_KEY_ID` etc. + `AWS_LAMBDA_RUNTIME_API` for refresh | Function execution role | Same — execution role needs the workload-side permissions |
| Cloud Run / GCF | `http://metadata.google.internal/computeMetadata/v1/instance/service-accounts/default/token` | Service account on the Service / Function | Backend must accept a configurable SA email; SA needs the workload permissions |
| ACA / AZF | IMDS at `http://169.254.169.254/metadata/identity/oauth2/token` (closed in Phase 109 for the sim) and `IDENTITY_ENDPOINT` for App Service-style | System-assigned or user-assigned managed identity on the Container App / Site | Backend must accept a `--managed-identity` config knob and attach it on PUT |

**Sim-side (already partially landed in Phase 109):**
- AWS STS `GetCallerIdentity` — ✓ in `simulators/aws/sts.go` (returns synthetic `arn:aws:sts::<account>:assumed-role/...`).
- AWS ECS Container Credentials endpoint at `169.254.170.2/v2/credentials/<id>` — ✗ not yet served. Required for SDK clients running inside a sim-provisioned container to find creds.
- GCP `iam.serviceAccounts.generateAccessToken` — ✓ closed in Phase 109.
- GCP metadata server at `metadata.google.internal/computeMetadata/v1/...` — ✗ not yet served end-to-end (the sim's HTTP listener can serve it, but containers need DNS / `/etc/hosts` rewriting to reach the sim by that hostname).
- Azure IMDS metadata token endpoint — ✓ closed in Phase 109.

**Scope of "minimal":**

1. **Each backend grows a config knob** for the workload identity:
   - ECS: `SOCKERLESS_ECS_TASK_ROLE_ARN` (already present); add a sanity check that the role's trust policy allows `ecs-tasks.amazonaws.com`.
   - Lambda: `SOCKERLESS_LAMBDA_EXECUTION_ROLE_ARN` (already present).
   - Cloud Run / GCF: `SOCKERLESS_GCR_SERVICE_ACCOUNT` / `SOCKERLESS_GCF_SERVICE_ACCOUNT` — currently the backend attaches the project's default compute SA; should be operator-configurable.
   - ACA / AZF: `SOCKERLESS_ACA_MANAGED_IDENTITY` / `SOCKERLESS_AZF_MANAGED_IDENTITY` — system-assigned by default with optional user-assigned passthrough.

2. **Each cloud's sim grows the missing endpoints** so dev runs without real cloud also succeed:
   - AWS sim: ECS Container Credentials endpoint at the standard IP. Containers run with `AWS_CONTAINER_CREDENTIALS_RELATIVE_URI` pointing at it.
   - GCP sim: metadata server at the standard hostname; containers get `metadata.google.internal` resolved via `/etc/hosts` injection or sidecar DNS.
   - Azure sim: already serves IMDS — verify reachability from inside a sim-provisioned container (may need `169.254.169.254` route trick, or env-var alias).

3. **End-to-end verification.** Add a runner workflow / pipeline (`identity-check.yml` / `.gitlab-ci.yml`) that runs the platform's CLI from inside the container:
   - GitHub × ECS: `aws sts get-caller-identity` returns the configured task role's `assumed-role/...` ARN.
   - GitHub × Lambda: same against the function execution role.
   - GitLab × ECS / × Lambda: same.
   - Add the same shape for GCP + Azure backends once Phase 110's matrix extends past AWS (currently scoped to ECS + Lambda).

**Bug discipline.** Same as Phase 110 — every gap surfaces a real bug; gets fixed in-branch.

**What this is not.**
- Not a new credential broker or sidecar — we use the cloud's native discovery path.
- Not a sockerless-specific identity model — sockerless just stitches the workload to the cloud's existing identity primitives.
- Not OIDC federation between GitHub/GitLab → cloud (separate Phase 112+ if it ever becomes interesting).

**Sequencing.** Phase 111 starts after Phase 110's hello workflow passes on all 4 cells. Most of the work is already partially landed (Phase 109's sim endpoints + per-backend role config); this phase is the verification-and-fill-in pass plus the test workflow.

### Phase 112 — Instance metadata services (queued; conditional)

Phase 111 covers the **identity-credential** half of cloud-instance metadata. Phase 112 covers the **everything else** half — instance attributes (region, AZ, instance type, network interface IPs), tags, user-data, custom attributes — exposed via the cloud's IMDS / metadata server. Activation gated on whether Phase 110/111 surface real-world workloads that trip on it; lots of CI tooling reads IMDS for region detection (`actions/cache` self-tunes by region, `gcloud config list project` falls through to metadata, Azure CLI uses IMDS for default tenant/subscription).

**Per-cloud endpoint surface (what real cloud serves):**

| Cloud | Native endpoint | What runner / app code asks for |
|---|---|---|
| AWS EC2 | `http://169.254.169.254/latest/meta-data/{instance-id,placement/region,placement/availability-zone,iam/security-credentials/<role>,...}` | Region, AZ, instance metadata, ec2 tags (IMDSv2 token-required) |
| AWS Fargate | `${ECS_CONTAINER_METADATA_URI_V4}` → `http://169.254.170.2/v4/<task-id>/{stats,task,...}` | Task ARN, family/revision, container constraints, network namespace info |
| AWS Lambda | env vars only — no metadata server. `AWS_REGION`, `AWS_LAMBDA_FUNCTION_NAME`, etc. directly in env. | Region, function name, memory tier |
| GCP | `http://metadata.google.internal/computeMetadata/v1/{instance,project,...}` (requires `Metadata-Flavor: Google` header) | Project ID, zone, region, instance ID, custom attributes, network interfaces |
| Azure | `http://169.254.169.254/metadata/instance?api-version=...` (requires `Metadata: true` header) | Subscription, resource group, region, VM size, tags, network IP |

**Sim-side state:**
- AWS STS sim: `GetCallerIdentity` ✓ closed in Phase 109.
- AWS IMDSv2 sim: ✗ not implemented. Token-required flow (`PUT /latest/api/token` + `X-aws-ec2-metadata-token` on subsequent GETs).
- AWS Fargate task-metadata v4 sim: ✗ not implemented. Must serve at the ECS task's eth0 link-local IP, scoped per task (so two concurrent sim tasks don't see each other's stats).
- GCP metadata server sim: ✗ not implemented. Already has DNS resolution for `metadata.google.internal` if we add it, but the endpoint itself doesn't exist in the sim yet.
- Azure IMDS sim: token endpoint at `/metadata/identity/oauth2/token` ✓ closed in Phase 109. Instance metadata at `/metadata/instance` ✗ not implemented.

**Conditional activation.** If Phase 110/111 ship and the only metadata operations runners actually exercise are identity-credential reads, Phase 112 stays in the queue indefinitely. If real workloads start hitting region/AZ/instance-metadata endpoints, scope:

1. **Per backend, attach the right metadata service to the workload's network namespace.**
   - ECS: Fargate already serves task-metadata v4 by default — sockerless just needs to ensure the task's `ECS_CONTAINER_METADATA_URI_V4` env var flows through (probably already does).
   - Lambda: env vars only — no IMDS. Phase 111 covers what's needed.
   - Cloud Run / GCF: GCP metadata server reachable from inside the workload by default. Verify.
   - ACA / AZF: Azure IMDS reachable from inside the container app by default. Verify (IMDS Phase 109 sim work is already on the right footing).

2. **Sim-side endpoints.** Bring up the missing four:
   - AWS IMDSv2 (with token discipline)
   - AWS Fargate task-metadata v4 (per-task scoping)
   - GCP metadata server with `Metadata-Flavor: Google` header gate
   - Azure instance metadata at `/metadata/instance`

   Each routed to the sim's HTTP listener; reachable from sim-provisioned containers via DNS/`/etc/hosts` injection or the link-local IP route already used elsewhere.

3. **Runner workflow / pipeline coverage.** Add a `metadata-check.yml` per cell that runs the cloud's native metadata-introspection CLI:
   - AWS: `aws ec2 describe-instances --instance-ids $(curl -s 169.254.169.254/latest/meta-data/instance-id)` (skipped on Fargate; replaced with `curl ${ECS_CONTAINER_METADATA_URI_V4}/task`).
   - GCP: `curl -H "Metadata-Flavor: Google" http://metadata.google.internal/computeMetadata/v1/project/project-id`.
   - Azure: `curl -H "Metadata: true" 'http://169.254.169.254/metadata/instance?api-version=2021-02-01'`.

   Assert each returns a real-shape response (matching the cloud's documented JSON / text shape).

**What this is not.**
- Not a custom metadata broker — we mirror the cloud's native shape.
- Not user-data or cloud-init scope — those are out of band of CI runner needs.
- Not generic key-value storage — sockerless's own state lives in cloud-tag space, not metadata.

**Sequencing.** Phase 112 starts only if a Phase 110 / 111 sweep finds runner workloads that trip on missing metadata endpoints. Until then it sits queued; the spec above is the design draft so the scope doesn't drift.

### Phase 113 — Production github-runner-dispatcher (queued; gated on Phase 110b closure)

Phase 110a/b ship the `github-runner-dispatcher` as a laptop-local binary: short-poll, stateless, single-PAT, single-repo (the `--repo` flag forces explicit scope). Phase 113 is the production-shape variant — what you'd run as a deployed service:

- **Webhook ingress** (replaces 15-s short-polling). HTTPS receiver behind a public URL; subscribes to GitHub `workflow_job:queued` webhook events. Webhook secret validation. Drops latency from ~15 s to ~1 s.
- **GitHub App install model** (replaces PAT). Long-lived via App's installation tokens; finer-grained scope; install-per-repo or install-per-org; rotation handled by the App framework. PAT path stays as a dev-mode fallback.
- **Multi-repo / org-scope**. Webhook config at the org level fans out to N repos. Dispatcher routes by repo + label combination.
- **Warm pool management**. Pre-spawned idle runners absorb queue latency. Stateful — needs a small persistence layer (DynamoDB / Postgres / Redis depending on deployment shape).
- **Deployable shapes**. Lambda function with API Gateway / Function URL for webhooks; ECS service for the polling fallback; Helm chart for k8s. All thin wrappers over the same dispatcher core.
- **Observability**. `/metrics` (Prometheus shape: jobs_seen, runners_spawned, runners_idle_timeout, errors), `/healthz` for liveness, structured logs.

This phase is *not* a rewrite of the laptop-local dispatcher — the core dispatcher logic stays sockerless-agnostic and Docker-API-only. Phase 113 wraps it with ingress + auth + state + deploy infrastructure.

Activation gated on whether the laptop-local 110a/b dispatcher proves the architecture and someone wants the production shape.

### Phase 114 — Long-lived helper task + ECS ExecuteCommand for gitlab-runner on ECS (queued; cell 3 unblock; substantial)

**gitlab-runner docker-executor architecture** (refresher — drives the design):

Each gitlab-runner job creates its OWN helper container and its OWN build container — they live for the duration of that job and only that job. `docker rm` happens at job end; the next job creates fresh containers from scratch. No state carries over between jobs. (Plus one container per `services:` entry, similarly per-job.) The job walks both containers through ~10 stages:

| Stage | Container | Source-of-script |
|---|---|---|
| `resolve_secrets` | (server-side) | gitlab.com |
| `prepare_executor` | — | docker create + network setup, NO container exec |
| `prepare_script` | helper (`-predefined`) | shell wrapper around `mkdir`/`chmod`/`echo` for the build dir |
| `get_sources` | helper | `git init` + `git remote` + `git fetch` + `git checkout` |
| `download_artifacts` | helper | `gitlab-runner-helper artifacts-downloader` (only if `dependencies:` set) |
| `step_script` | **build** (e.g. alpine) | `before_script:` + `script:` from `.gitlab-ci.yml` |
| `after_script` | **build** | `after_script:` from `.gitlab-ci.yml` |
| `archive_cache_on_success` / `archive_cache_on_failure` | helper | `gitlab-runner-helper cache-archiver` (only if `cache:` set) |
| `upload_artifacts_on_success` / `upload_artifacts_on_failure` | helper | `gitlab-runner-helper artifacts-uploader` (only if `artifacts:` set) |
| `cleanup_file_variables` | helper | `rm -rf` over `$CI_PROJECT_DIR/tmp` for file-typed CI variables |

Both containers are created with the SAME stdin-reading entrypoint (gitlab-runner overrides whatever the source image had via `--entrypoint`):

```
ENTRYPOINT ["sh", "-c",
  "if [ -x /usr/local/bin/bash ]; then exec /usr/local/bin/bash; \
   elif [ -x /usr/bin/bash ]; then exec /usr/bin/bash; \
   elif [ -x /bin/bash ]; then exec /bin/bash; \
   elif [ -x /usr/local/bin/sh ]; then exec /usr/local/bin/sh; \
   ... etc ... \
   else echo shell not found; exit 1; fi"]
```

`OpenStdin=true, AttachStdin=true, StdinOnce=true, Tty=false`. The sh-then-bash exec wrapper sits waiting on stdin. Per stage:

1. `docker start <container>` — re-runs the entrypoint (real Docker re-runs ENTRYPOINT on every `start` of a STOPPED container).
2. `docker attach -i <container>` — gitlab-runner pipes the stage's generated shell script as stdin bytes. The shell reads, executes, exits when stdin EOFs.
3. `/wait` — gets the exit code; if non-zero, gitlab-runner skips the user-script stages (`step_script`, `after_script`) and routes through the failure-cleanup chain (`archive_cache_on_failure`, `upload_artifacts_on_failure`, `cleanup_file_variables`).

Every stage's "Running on $(hostname) via $(client)..." banner comes from the **generated shell script's first line**, NOT from the helper image's compiled-in code. So both the helper and build containers are just bash-script-runners; the only difference is which image's filesystem they execute on.

**Why Fargate breaks this**:

- Fargate tasks are **not restartable** — once a task transitions to STOPPED, that task ARN is gone. `RunTask` always creates a new task ARN.
- Fargate tasks have **no runtime stdin channel** — once `RunTask` starts, the task's PID-1 stdin is closed; there's no SDK call to write more bytes to it.

Sockerless's BUG-859 fix translated each `docker start <build-container>` cycle into a fresh `RunTask` with the script baked into `Entrypoint=["sh","-c"], Cmd=[<script>]`. That works for the BUILD container because the bytes ARE the script — replacing the bash-wrapper with `sh -c <script>` is functionally equivalent to `bash <(echo "<script>")`.

It does **not** work for the predefined helper container as currently implemented:

- BUG-867's `-predefined`-suffix filter routes the helper through synchronous `RunTask` with the bash-wrapper Entrypoint. The wrapper exec's into bash, bash reads stdin which closes immediately (Fargate has no runtime stdin), bash exits 0 in <1 s. Stage "succeeds" but did no work — `git fetch` never ran, no sources are checked out, gitlab-runner detects the empty workspace at `step_script` time and routes to the failure-cleanup chain. Cell 3 fails at BUG-868's symptom.
- Removing the `-predefined` filter and entering `launchAfterStdin` for the helper too produces a 13-min hang: gitlab-runner's predefined helper /attach is sometimes for log streaming (no stdin write) — the goroutine waits for stdin EOF that never arrives.
- Adding a content-empirical timeout (3 s) makes the goroutine fall through with an empty buffer; the helper runs with the bash-wrapper Entrypoint preserved (no script bake), and bash again exits in <1 s with no work done.

**Cloud primitive in use** (Phase 114's translation target): Fargate task lifecycle (PROVISIONING → RUNNING → STOPPED, non-reversible) + **ECS ExecuteCommand (SSM Session Manager)** — the cloud's "open an interactive session against a running task" primitive. The task must have been launched with `enableExecuteCommand: true` (sockerless's task definitions already do).

**Fix shape**:

1. **Helper-container detection at /create time**: container's name has `-predefined` suffix → annotate `ECSState.LongLivedHelper = true`. Build containers (no suffix) keep the existing `launchAfterStdin` per-cycle path.
2. **First /start on a long-lived helper**: launch the Fargate task ONCE with overridden `Entrypoint=["sh","-c"], Cmd=["while true; do sleep 60; done"]` (a SIGTERM-friendly idle loop). Wait for `RUNNING + ExecuteCommandAgent.LastStatus == RUNNING` (the latter takes 5-30 s after task RUNNING — already handled by the `cloudExecStart` wait in BUG-853). Cache `(containerID → taskARN)`.
3. **Subsequent /start cycles for the same container**: short-circuit. Don't `RunTask`. Buffer the per-cycle stdin script bytes from the attach pipe.
4. **Per-cycle script delivery via ExecuteCommand**:
   - Open an SSM session: `aws ecs execute-command --task <ARN> --interactive --command "/bin/sh"` — equivalent SDK call: `ssm.StartSession` with the ECS-ExecuteCommand-generated session token.
   - Write the buffered script bytes through the session's I/O stream (the existing SSM AgentMessage frame protocol from Round-8 already handles this — see `backends/ecs/ssm_session.go`).
   - Stream stdout/stderr from the SSM stream into the docker `/attach` hijacked connection with multiplexed-stream framing for non-tty execs.
   - End-of-script marker: emit `; echo SOCKERLESS_EXIT_CODE=$? >&2; exit` after the user script (existing `wrapWithExitCodeMarker` helper).
   - Capture the exit code from the marker line; surface via `/wait` and the container's State.ExitCode.
5. **/wait, /stop, /kill, /rm**:
   - `/wait` blocks until the current SSM session ends + exit-code marker is captured. If no script-delivery cycle is active, `/wait` returns immediately with the last-known exit code.
   - `/stop` calls `ecs.StopTask` (SIGTERM the idle loop, Fargate transitions to STOPPED). Drop the cached taskARN.
   - `/kill` same as /stop with SIGKILL semantics (`StopTask` with `Reason="killed"`).
   - `/rm` deregisters the task definition + drops cached ARN.
6. **/exec on the same container**: same SSM path — open a fresh session against the live task, run the requested command. Already implemented by `cloudExecStart` for non-stdin /exec; needs minor wiring for the stdin-pipe path.

**File layout**:

- New `backends/ecs/long_lived_helper.go` — the long-lived task launcher + per-cycle SSM session bridge.
- Reuse existing `backends/ecs/ssm_session.go` for the stream wiring (already proven by `cloudExecStart`).
- New unit test `backends/ecs/long_lived_helper_test.go` — async-mock SSM session, verify the script-bytes-in / stdout-bytes-out / exit-code-marker round-trip.

**Estimated scope**: ~400-600 lines new code + ~150 lines refactor in `backends/ecs/backend_impl.go`. Comparable to Phase 116's `lambdaInvokeExecDriver`.

**Why this matches the Fargate primitive**: Fargate tasks support long-running workloads with `enableExecuteCommand: true`. The task entrypoint stays an idle loop; sockerless dispatches per-stage work via the cloud's actual "run command in a live task" primitive. No retries, no fakes — `docker start` semantics map onto `RunTask` once + `ExecuteCommand` per cycle.

**Not in scope**: cross-cell concurrency (one long-lived helper per `(backend instance, container ID)` is sufficient — ECS ExecuteCommand supports concurrent sessions per task).

Closes BUG-868. Cell 3 GREEN gates Phase 114 closure. Cell 4 separately addressed by Phase 117.

### Phase 115 — Always-on overlay-inject for Lambda CreateFunction (queued; cell 2/4 unblock)

**Why.** Lambda image-mode imposes two hard constraints on every function image: (a) manifest must be Docker Image Manifest V2 Schema 2 (OCI rejected with `image manifest, config or layer media type ... is not supported`), and (b) the image's ENTRYPOINT must be a Lambda Runtime API client — it has to poll `/2018-06-01/runtime/invocation/next` and post results to `/response` or `/error`, otherwise the function never serves an invocation. The cell-2 alpine image fails both: alpine on AWS Public Gallery is `application/vnd.oci.image.index.v1+json`, and `tail -f /dev/null` ENTRYPOINT doesn't speak Lambda Runtime API (BUG-873).

**Cloud primitive in use.** Lambda CreateFunction with `PackageType=Image, Code.ImageUri=<ECR ref>`. Lambda's image ingestion at function creation time copies layers into a Lambda-internal store; from then on, `Invoke` cold-starts boot the user image and call its ENTRYPOINT. The Runtime API contract is specified at https://docs.aws.amazon.com/lambda/latest/dg/runtimes-api.html.

**Fix shape.** Sockerless owns a translation layer that bridges arbitrary Docker images to Lambda's image-mode contract:

1. **Always go through `BuildAndPushOverlayImage`** for Lambda CreateFunction — drop the no-CallbackURL default branch in `backends/lambda/backend_impl.go`. The overlay (a) bakes `sockerless-lambda-bootstrap` as ENTRYPOINT (resolves Runtime-API gap), (b) is built via plain `docker build` + `docker push` which produces Docker schema 2 (resolves manifest-format gap), (c) preserves the user's original ENTRYPOINT/CMD as `SOCKERLESS_USER_ENTRYPOINT` / `SOCKERLESS_USER_CMD` env vars decoded at bootstrap time.
2. **Use `awscommon.CodeBuildService` when no local docker daemon is available.** The runner-Lambda execution environment has no docker; the existing `BuildAndPushOverlayImage` calls `os/exec docker build` which would fail. Already wired via `s.images.BuildService` in `backends/lambda/server.go:72-76`. Refactor `BuildAndPushOverlayImage` to take a `core.CloudBuildService` dependency and prefer it over `os/exec` when set.
3. **Cache converted images by source-content hash.** sha256 of (BaseImageRef + AgentBinaryPath + BootstrapBinaryPath + UserEntrypoint + UserCmd) → tag in a sockerless-managed `sockerless-live-overlay` ECR repo. Cache hit skips the rebuild; cache miss runs CodeBuild + push.
4. **specs/CLOUD_RESOURCE_MAPPING.md update.** Lambda mapping row already says "container deployment is what lets sockerless put its bootstrap at the entrypoint, which is the prerequisite for the reverse-agent, agent-as-handler, and overlay-rootfs patterns" — extend with explicit "All Lambda images go through overlay-inject; OCI inputs auto-converted to Docker schema 2 by the overlay build."

**Why this matches the Lambda primitive.** Lambda's only image format is Docker schema 2; its only invocation contract is Runtime API. Sockerless cannot "make Lambda accept OCI" — that's an AWS decision. The honest mapping is "every user image gets re-tagged with sockerless-lambda-bootstrap as ENTRYPOINT" — and that re-tag is done via the cloud's image-build primitive (CodeBuild) when local docker isn't available. Same nature as sockerless's reverse-agent translation of `docker exec` for Lambda (no native primitive, sockerless implements the Docker semantic on top of what the cloud actually offers).

**Not in scope.**
- Skipping the overlay when the user supplies an already-Lambda-aware image — possible but fragile to detect (manifest type + runtime API client). Default to overlay-always; operators can opt out via `PrebuiltOverlayImage` (already supported).
- Cross-cloud overlay registry sharing — each cloud needs its own primitive (CodeBuild for AWS, Cloud Build for GCP, ACR Tasks for Azure). Phase 95 + Task #95 audits will close those for the other backends.

Closes BUG-873. Verified live (workflow run 25105165208) at commit `d5073b4`: CodeBuild SUCCEEDED + Lambda CreateFunction returned the ARN. Phase 115 closed. Cell 2 still blocks on BUG-874 (start/exec lifecycle) — see Phase 116.

### Phase 116 — Reverse-agent dial-back for runner-on-Lambda exec lifecycle (queued; cell 2/4 unblock)

**Why.** After Phase 115 lands, the next runner-on-Lambda wall is a docker-vs-Lambda lifecycle mismatch (BUG-874). Lambda has no "start" primitive — only `Invoke` that runs the function once. The current ContainerStart returns immediately while a goroutine fires the V2 Active waiter + Invoke later; the GH runner's first `docker exec` arrives in ~80ms and fails because the function is still `Pending`. Verified live (workflow run 25105474526): runner does `docker create` (201) → `docker start` (204 in 21ms) → `docker exec` 80ms later → `DELETE container` 478ms after that — all before the function transitions to Active, so the Active waiter then logs `Function not found: arn:...:skls-...` because sockerless's DELETE deleted the function.

**Cloud primitive in use.** Lambda Invoke (sync) + the existing reverse-agent WebSocket pattern (`agent_e2e_integration_test.go` already exercises this end-to-end on the sim). The reverse-agent path lets sockerless tunnel `docker exec` frames into a running Lambda invocation through a long-lived WebSocket connection that the bootstrap dials back when it boots.

**Fix shape.**

1. **`SOCKERLESS_CALLBACK_URL` infrastructure** — provision an ALB (or Lambda Function URL with VPC endpoint) fronting the runner-Lambda's sockerless on port 3375 so sub-task Lambdas in the same VPC can dial back. terraform/modules/lambda/runner.tf adds: `aws_lb` + `aws_lb_target_group` (target_type=lambda, attached to `aws_lambda_function.sockerless_runner`) or `aws_lambda_function_url` if simpler.
2. **`SOCKERLESS_CALLBACK_URL` env on the runner-Lambda** — points at the ALB DNS so the in-Lambda sockerless backend wires it through to sub-task `CreateFunctionInput.Environment.Variables["SOCKERLESS_CALLBACK_URL"]`.
3. **Synchronous `ContainerStart`** — block until (a) `FunctionActiveV2Waiter` returns Active, AND (b) `lambda.Invoke` is dispatched (async, since the bootstrap runs the runtime-API loop), AND (c) the reverse-agent dials back (registered in `s.ReverseAgentRegistry`). Only then return 204 to the runner. Time budget: ~30-90s typical for image-mode Lambda + VPC.
4. **`docker exec` via reverse-agent** — already implemented in the lambda backend's exec path when CallbackURL is set. With Phase 116 wiring, this works end-to-end for any sub-task.
5. **`docker stop` / `wait`** — terminate the invocation by sending a TypeShutdown over the reverse-agent WebSocket; the bootstrap exits the runtime-API loop, Lambda completes the invocation, sockerless caches the exit code in `Store.InvocationResults`. Existing pattern; just needs to fire on stop.

**Why this matches the Lambda primitive.** Lambda has no native "long-lived container with synchronous exec" semantic. The reverse-agent pattern is sockerless's translation: an Invoke that stays running until told to exit, with WebSocket as the side-channel for arbitrary commands. Same nature as Phase 114's "long-lived Fargate task + ECS ExecuteCommand" — both translate Docker's stateful container lifecycle onto cloud primitives that don't have it natively. Documented in `specs/CLOUD_RESOURCE_MAPPING.md` Lambda mapping row's "container deployment is what lets sockerless put its bootstrap at the entrypoint, which is the prerequisite for the reverse-agent" line.

**Not in scope.**
- Cell 4 (GitLab × Lambda) — separately addressed by Phase 117 (gitlab-runner stdin-piped per-stage scripts on Lambda primitives).

Closes BUG-874. Cell 2 GREEN gates Phase 116 closure (CLOSED 2026-04-29 at workflow run 25113565115).

### Phase 117 — gitlab-runner per-stage script delivery on Lambda (queued; cell 4 unblock)

**Why.** Phase 116's `lambdaInvokeExecDriver` covers single `docker exec` calls (the GH-runner-on-Lambda case from cell 2 — each `run:` step is one `docker exec` ⟶ one `lambda.Invoke`). gitlab-runner's docker executor instead uses `docker start + docker attach -i` per stage with the script piped through stdin (see Phase 114's gitlab-runner refresher table). On Lambda, the build container is created via `lambda.CreateFunction`; gitlab-runner's per-stage `/start + /attach` cycle has to translate to `lambda.Invoke` calls per stage.

**Cloud primitive in use**: `lambda.Invoke` per script stage, with the script bytes carried in the `Payload`. The bootstrap's `runUserInvocation` already accepts an exec-envelope `{"sockerless":{"exec":{"argv":[...]}}}` form (Phase 116 / Path B). Phase 117 adds a SCRIPT-envelope form: `{"sockerless":{"script":{"shell":"sh","body":"<base64>","workdir":"...","env":[...]}}}` — the bootstrap unwraps it and runs `bash -c "<decoded body>"` in a subprocess.

**Fix shape**:

1. **Build-container side**: `backends/lambda/backend_impl.go` ContainerStart's stdin-pipe path (BUG-860 — currently bakes script bytes into Invoke Payload as-is) is extended to recognise the gitlab-runner stdin-pipe lifecycle. Per /start cycle: Read buffered stdin → wrap as a SCRIPT envelope → call `lambda.Invoke` with the envelope as Payload → wait for response → tunnel stdout/stderr to the docker /attach hijacked conn → record exit code.
2. **Predefined helper side**: same path. Lambda has no equivalent of "long-lived task with ExecuteCommand" — every Invoke is a fresh execution environment. So each stage's helper-image SCRIPT envelope creates a fresh function invocation. State persistence between stages happens via EFS (workspace + externals are shared across invocations, same as cell 2). gitlab-runner's "Running on $(hostname) via $(client)..." banner per stage = each invocation prints its own.
3. **Bootstrap envelope handling**: `agent/cmd/sockerless-lambda-bootstrap` adds a `parseScriptEnvelope` helper alongside the existing `parseExecEnvelope`. When the Invoke Payload matches the script envelope, run `bash -c "<body>"` (or `sh -c` for `shell:"sh"`); otherwise fall through to the existing Path B exec / main-cmd handling.
4. **Path A fallback** (reverse-agent): if `SOCKERLESS_CALLBACK_URL` is set on the runner-Lambda, the bootstrap dials back via WebSocket — gitlab-runner's per-stage scripts still translate to Invoke, but exec-style follow-ups (rare for gitlab-runner) tunnel through the dial-back. This isn't strictly required for cell 4; included for symmetry with Phase 116.

**Why this matches the Lambda primitive**: each gitlab-runner stage is bounded — a few seconds to a few minutes. Lambda's 15-min hard cap is a non-issue for individual stages. Cross-stage state lives on EFS (workspace, externals, file-typed CI variables). The per-stage `lambda.Invoke` model is naturally "Invoke once per discrete unit of work", which matches gitlab-runner's stage abstraction. No long-lived "tail -f /dev/null" workaround needed; Lambda's invoke model fits gitlab-runner's stage cadence cleanly.

**File layout**:

- Modify `agent/cmd/sockerless-lambda-bootstrap/main.go`: add `scriptEnvelope` + `runScriptInvocation`. Mirror of `execEnvelope` / `runExecInvocation` from Phase 116.
- Modify `backends/lambda/backend_impl.go`'s stdin-pipe goroutine: marshal script envelope when `lambdaState.OpenStdin` indicates the per-stage stdin-pipe pattern (use the same per-cycle stdinPipe buffer that Path B's `lambdaInvokeExecDriver` uses — these can share infrastructure).
- New unit tests for the script-envelope round-trip.

**Estimated scope**: ~250-400 lines. Smaller than Phase 114 because Lambda already has the canonical per-cycle dispatch primitive (`Invoke`); we just need a script-shaped envelope.

**Not in scope**: long-lived Lambda functions per gitlab-runner job. Lambda's primitive doesn't support that (15-min cap; no "exec into a running invocation" channel). Each stage = fresh invocation. `/tmp` doesn't persist; EFS does. This is documented in `specs/CLOUD_RESOURCE_MAPPING.md` § "ECS gitlab-runner script delivery" already.

Closes BUG-868's Lambda half (cell 4). Phase 117 can land independently of Phase 114 — the Lambda translation doesn't depend on the ECS long-lived-task work.

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
9. ✓ **GCP sim — `compute.routers` + Cloud NAT.** Was missing; `terraform-provider-google`'s `google_compute_router` and `google_compute_router_nat` 404'd. Added Create/Get/List/Delete/Patch on `/compute/v1/projects/{p}/regions/{r}/routers` + `ComputeRouter`/`ComputeRouterBgp`/`ComputeRouterNAT` types matching real `compute#router` shape (NAT IP allocate options, source-subnetwork ranges, log config, per-subnet selectors).
10. ✓ **Azure sim — `Microsoft.Network/natGateways` + `routeTables`.** Were missing; `azurerm_nat_gateway` and `azurerm_route_table` 404'd. Added PUT/GET/DELETE on both surfaces; defaults match real Azure (NAT IdleTimeoutInMinutes=4, Sku=Standard). Custom routing flows for ACA Apps with VNet integration now work.
11. ✓ **Azure sim — `Azure-AsyncOperation` polling for Container Apps + Jobs.** Real Azure ACA returns 201 Created + `Azure-AsyncOperation` header pointing at `/providers/Microsoft.App/locations/{loc}/operationStatuses/{opId}`; the SDK poller GETs that URL until `status=Succeeded`, then does a final GET on the resource. Sim now mirrors that flow: new `AsyncOperationStatus` type matches the standard ARM polling payload; `acaIssueAsyncOp` records each PUT in InProgress, then a goroutine flips it to Succeeded after 50ms (compresses real Azure's 30-60s reconcile window). PUT on ContainerApps + Jobs sets the header; the operation status endpoint is registered alongside the Apps surface (the path is shared across the Microsoft.App provider). Resource itself is stored Succeeded directly so the final GET always sees the desired state regardless of when polling completes.
12. ✓ **AWS sim — Secrets Manager.** Was missing entirely; `aws-actions/configure-aws-credentials` follow-ups, `aws secretsmanager get-secret-value`, and terraform's `aws_secretsmanager_secret` all 404'd. Added the JSON-protocol actions: CreateSecret/GetSecretValue/DescribeSecret/UpdateSecret/PutSecretValue/DeleteSecret/ListSecrets/TagResource/UntagResource. SecretId resolution accepts both name and ARN; VersionId rotates on update.
13. ✓ **AWS sim — SSM Parameter Store.** Was missing entirely. Added PutParameter/GetParameter/GetParameters/GetParametersByPath/DescribeParameters/DeleteParameter/DeleteParameters with real-AWS-shape `ParameterAlreadyExists` 400 on re-Put without Overwrite=true; Version increments on each update; GetParametersByPath supports both flat and recursive modes.
14. ✓ **AWS sim — KMS Encrypt/Decrypt + key management.** Was missing entirely; SecureString parameters, KmsKeyId-encrypted secrets, S3 SSE-KMS, and direct envelope encryption all failed. Added CreateKey/DescribeKey/ListKeys/ScheduleKeyDeletion/Encrypt/Decrypt/GenerateDataKey/CreateAlias/DeleteAlias/ListAliases. Encryption uses a deterministic envelope (`kms-sim:<keyId>:<base64(plaintext)>`) — opaque to SDK callers but reversible by the sim. KeyId resolution accepts plain UUIDs, ARNs, and aliases.
15. ✓ **Azure sim — Key Vault.** Was missing entirely. Both ARM control plane (`Microsoft.KeyVault/vaults` PUT/GET/DELETE/LIST) **and** data plane (subdomain-routed via `srv.WrapHandler` matching `<vault>.vault.<sim-host>` Host pattern, secret CRUD on `/secrets/{name}`) match real Azure shape; vaultURI returned in the form `<scheme>://<vault>.vault.<sim-host>:<port>/`.
16. ✓ **AWS sim — DynamoDB.** Was missing entirely. CreateTable/DescribeTable/DeleteTable/ListTables + PutItem/GetItem/UpdateItem/DeleteItem/Query/Scan. Critically: `PutItem` with `ConditionExpression="attribute_not_exists(LockID)"` succeeds the first time and returns `ConditionalCheckFailedException` on contention — the exact wire shape Terraform's S3 backend uses for state-locking. Items keyed by `<table>/<hashKey>[|<rangeKey>]`.
17. ✓ **GCP sim — operations endpoint persistence.** Real Cloud Run keeps LRO records around for the SDK to `GetOperation` against. The sim previously returned a synthetic `done=true` Operation for any op id (including ones the sim never issued) — that masked client bugs. Operations are now persisted in a shared `crOperations` Store; `newLRO` writes the record on issue, and `GET /v{1,2}/.../operations/{op}` returns the persisted record or 404 for unknown ids. Cloud Run jobs settle to `CONDITION_SUCCEEDED` on create because the sim has no real reconciliation to do — injecting a synthetic delay just to mimic real Cloud Run's `RECONCILING → SUCCEEDED` window would be exactly the fake behaviour this audit removes.
18. ✓ **Azure sim — `SystemData.createdAt` preserved across updates.** Real ARM stamps `systemData.createdAt` once on resource creation and only updates `lastModifiedAt` on subsequent writes — restamping on every PUT/PATCH would surface in `azure-cli --query systemData` output and break audit trails. Container Apps + Jobs PUT handlers now read the existing record's CreatedAt and reuse it across updates; `LastModifiedAt` is stamped fresh on every write. SystemData type extended with the standard `lastModifiedAt` field. SDK test asserts the contract.

19. ✓ **No-fakes audit on test fixtures — clean.** Repo-wide sweep of `simulators/*/{sdk,cli,terraform,bash}-tests/` confirmed zero violations. All hardcoded IDs are either: (a) sim-pre-registered defaults (e.g. `subnet-0123456789abcdef0` from `ec2.go`), (b) configuration values like subscription/tenant/project IDs, (c) caller-provided UUIDs that real Azure accepts (role-assignment names), or (d) intentional negative-test inputs (`subnet-doesnotexistanywhere` to verify rejection). Tests that need real resource state create resources via the CRUD API first and reference the returned IDs.

**Scope (still pending — sequencing):**

(Phase 109 audit-tracked work is complete. Further audit items will be added as they surface.)

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
