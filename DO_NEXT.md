# Do Next

Resume pointer. Updated after every task. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); runner wiring in [docs/RUNNERS.md](docs/RUNNERS.md).

## Resume pointer (2026-05-03)

**PR #123** (`phase-118-faas-pods`): all standard CI green. Cells 5+6 in flight live; cells 7+8 not started. Goal: all four green BEFORE merging #123.

**PR #124** (`cell-workflows-on-main`, throwaway): lands cell-5/6 workflow yamls on `main` so `workflow_dispatch` can fire them with `--ref phase-118-faas-pods`. Also carries TEMP `pull_request:` triggers on cell-5/6 (constrained to `branches: [main]`+`paths: [.github/workflows/cell-{5,6}-*.yml]`) so PR-#124 pushes auto-fire those cells. **Must NOT be merged.** Close after cells 5+6 GREEN — the `pull_request` trigger is throwaway; PR #123's cell yamls have only `workflow_dispatch` so post-merge state is manual-only (matches cells 1-4).

**Live infra deployed (sockerless-live-46x3zg4imo)**:
- AR `dispatcher:gcp-amd64` (built via Cloud Build, sockerless-sanctioned).
- Secret Manager `github-pat` v1 = `gh auth token`; granted to `sockerless-runner@…iam` SA.
- Cloud Run Service `github-runner-dispatcher-gcp` (us-central1, min=max=1, no-cpu-throttling, runner SA, $PORT=8080 /healthz, REPO=e6qu/sockerless env, GITHUB_TOKEN secret-mounted). URL: `https://github-runner-dispatcher-gcp-199307773205.us-central1.run.app`. Currently serving rev `00002-ll6` (post BUG-908 fix).

**Bugs surfaced live this session**:
- **BUG-907 (closed in PR #123 + rebuilt images)**: bash apostrophe in `${var:?msg}` form crashes runner bootstrap. Fixed text + rebuilt cloudrun + gcf runner images in AR.
- **BUG-908 (closed in dispatcher rev 00002)**: `Cloud Run Jobs.CreateJob` rejects nested `Job.Name` — must be empty (name comes from `JobId` field on the request). `spawner.go` fixed.
- **BUG-909 (open, IN PROGRESS)**: cloudrun + gcf backends lack bind-mount → managed-volume translation. github-runner emits `-v /var/run/docker.sock:/var/run/docker.sock`, `-v /tmp/runner-work:/__w`, `-v /opt/runner/externals:/__e:ro`; sockerless-backend-cloudrun rejects all of them with "host bind mounts are not supported". Phase-110b-equivalent for GCP. Mirror `backends/ecs/config.go::SharedVolume{Name,ContainerPath,AccessPointID,FileSystemID}` + `backends/ecs/backend_impl.go::ContainerCreate` bind-mount translator. **For GCP**: SharedVolume backed by GCS bucket (Cloud Run Jobs `Volume{Gcs{Bucket}}`). Drop `/var/run/docker.sock` mount unconditionally. Translate matching ContainerPath → named-volume ref; drop sub-paths.

**BUG-909 wire-up beyond the backend**:
- `github-runner-dispatcher-gcp/internal/spawner/spawner.go` must add a Cloud Run `Volume{Gcs{Bucket}}` to the spawned runner Cloud Run Job + a corresponding container `VolumeMounts` entry mounting at `/tmp/runner-work` (and `/opt/runner/externals`), AND set `SOCKERLESS_GCP_SHARED_VOLUMES=runner-workspace=/tmp/runner-work=<bucket>,runner-externals=/opt/runner/externals=<bucket>` env so the in-image backend's translation knows the mapping.
- New terraform resource: `google_storage_bucket.runner_workspace` in `terraform/modules/cloudrun/runner.tf` (force_destroy + 1-day lifecycle, mirrors `aws_efs_file_system` + access points in `terraform/modules/ecs/runner.tf`). Bucket name surfaces as dispatcher config `runner_workspace_bucket` field.

**Next steps (sequenced)**:
1. Read `backends/ecs/config.go` lines 40-155 + `backends/ecs/backend_impl.go` lines ~100-170 for the canonical SharedVolumes + ContainerCreate translator pattern. Mirror onto `backends/cloudrun/config.go` + `backend_impl.go` + `containers.go` (Cloud Run Job creation must add `Volume.Gcs.Bucket` + container `VolumeMounts`).
2. Same for `backends/cloudrun-functions/` (gcf — uses Cloud Run Service under the hood for Gen2; check whether Functions Gen2 supports gcs volumes — if not, fall back to GCS-FUSE-via-overlay-bootstrap pattern; do NOT add fakes/fallbacks).
3. Add `runner_workspace_bucket` TOML field to `github-runner-dispatcher-gcp/internal/config/config.go` (REQUIRED, no fallback). Update `internal/spawner/spawner.go::Spawn` to add the Volume + VolumeMount + env.
4. ✅ DONE 2026-05-03: GCS bucket `gs://sockerless-live-46x3zg4imo-runner-workspace` created in us-central1 with uniform bucket-level access; runner SA granted `roles/storage.admin`.
5. Update `~/.sockerless/dispatcher-gcp/config.toml` + the baked `github-runner-dispatcher-gcp/config.toml.example` with the new `runner_workspace_bucket = "sockerless-live-46x3zg4imo-runner-workspace"`.
6. Rebuild backends: `make -C tests/runners/github/dockerfile-cloudrun push-amd64` + `make -C tests/runners/github/dockerfile-gcf push-amd64`.
7. Rebuild + redeploy dispatcher: `gcloud builds submit --config=github-runner-dispatcher-gcp/cloudbuild.yaml --gcs-source-staging-dir=gs://sockerless-live-46x3zg4imo-build/source .` then `gcloud run services update github-runner-dispatcher-gcp --image=us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/dispatcher:gcp-amd64 --region=us-central1 --project=sockerless-live-46x3zg4imo`.
8. Trigger PR #124 push (any small commit on `cell-workflows-on-main`) to re-fire cells 5+6.
9. Watch `gh pr checks 124 --watch` + `gcloud logging read resource.type=cloud_run_revision resource.labels.service_name=github-runner-dispatcher-gcp` for spawn events.
10. Capture three URLs per cell into STATUS.md (CI run + Cloud Run Job execution + Cloud Logging).
11. After cells 5+6 GREEN: build serverless gitlab-runner on Cloud Run, register against `e6qu/sockerless` GitLab project, push the cell-7/8 yamls via the existing `tests/runners/gitlab/harness_test.go::runCell` pattern. GitLab PAT at `~/.sockerless/gitlab-pat` (mode 600).
12. Close PR #124 (do NOT merge). Update STATUS / WHAT_WE_DID / BUGS / PLAN.

**Re-orient if context lost**: read `.github/workflows/cell-5-cloudrun.yml` (current shape), `backends/cloudrun/backend_impl.go::ContainerCreate` (line ~75 — bind-mount rejection), `backends/ecs/backend_impl.go::ContainerCreate` (line ~100 — reference impl), `backends/ecs/config.go` lines 40-155 (SharedVolume + helpers).

## Branch state

- `main` synced with `origin/main` at PR #121 merge. Phase 110 PR (#122) merged.
- **Active session: Phase 118 — live-GCP track**, working on `main` since the changes touch shared core code (`backends/core/log_driver.go`, `backends/core/tags.go`) plus per-backend modules. Branch will be cut as `phase-118-live-gcp` before push.
- Active live-cloud project: `sockerless-live-46x3zg4imo` (billing 019E9E-AF0BD0-6A6F75 free trial, ephemeral-project workflow per `feedback_teardown_aggressive.md` adapted for GCP).

## Phase 118 — live-GCP track

| Backend | State |
|---|---|
| `cloudrun` (Cloud Run Jobs) | ✅ Live-validated. BUG-877..885 closed (image-resolve, logs filter, post-mortem logs, ps -a dedup, stale rows, --rm cleanup, log-ingestion race). Manual sweep clean: clean stdout, zero leaked Jobs, zero ghost containers. |
| `gcf` (Cloud Run Functions Gen2) | 🚧 Architecture decided + spec updated. BUG-884 fix in progress: stub-Buildpacks-Go-source + post-create `Run.Services.UpdateService(image=overlay)` + content-addressed AR cache + label-based stateless reuse pool. |
| `azf` (Azure Functions) | Queued. Direct image deploy supported on Premium/Flex/AppService — simpler shape than gcf. Pod question (E supervision-via-overlay vs G NotImplemented) still open. |

## Next actions (sequenced by user 2026-05-02)

State-save after each task: STATUS.md + WHAT_WE_DID.md + BUGS.md + memory + this file.

**Resume pointer for next session: PR #123 (`phase-118-faas-pods`) bundles Phase 118 + Phase 120 + Phase 121 + Phase 122 (GCP+Azure dispatchers) + Phase 122c (sanctioned cloud builders + terraform-managed dispatcher resources) — ALL CI GREEN at commit `a646602` (2026-05-02). Latest local work: BUG-907 (bash bootstrap apostrophe parse) fixed; runner image rebuilt + retested live (boots cleanly through to GitHub runner registration step which fails on dummy token, expected). Terraform extended: GCS build_context bucket + Cloud Build API + extended runner SA roles on cloudrun/gcf modules; ECR pull-through cache rule on lambda/ecs modules (gated for singleton); ACR cache rule + AcrPush + Contributor roles on aca/azf modules. Phase 121 (cloud-faithful GCP sim hardening) closed via the BUG-893..906 chain. Phase 121b (Azure mirror) and Phase 122 (per-cloud github-runner-dispatcher GCP/Azure mirrors) are queued — if continuing on the same branch/PR per the "all work in one PR" direction, start with the parallel-fix sweep below; otherwise the PR is ready for the operator to merge as-is. Phase 119 (k8s shim) was discarded after exploration. Live-AWS test of 118b deferred (operator to authorize). Sub-118c (AZF) deferred until cells GREEN.**

**Operator rule reinforced 2026-05-02**: backends MUST stay clean — stateless, with no awareness of the simulator code or each other. EndpointURL-gated bypasses inside backend code are tech debt per the no-fakes / no-fallbacks rule. The simulator must present a faithful API surface so backends behave identically against either. Lambda's existing `case s.config.EndpointURL != "":` branch is also flagged for removal (Phase 121e).

**Parallel-fix sweep before Phase 121b/122 (queued for next session)**:

1. **AWS sim — Lambda ListFunctions filter** (`simulators/aws/lambda.go::handleLambdaListFunctions`): currently returns every function; mirror the GCP sim's `matchesFunctionFilter` shape onto AWS Lambda's tag-filter / `Marker` / `MaxItems` query params. Same root-cause class as BUG-906.
2. **Azure sim shared helpers** (`simulators/azure/shared/container.go`): copy `StartHTTPContainer` / `StopAndRemoveContainer` / `StreamContainerLogs` from `simulators/gcp/shared/container.go`; ACA / Azure Functions invocation will need them once the overlay-bootstrap shape lands.
3. **Azure sim — `simulators/azure/functions.go::invokeAzureFunctionProcess`**: replace the blocking `sim.StartContainerSync` call with the cloud-faithful HTTP-invoke pattern from `invokeOverlayContainerHTTP` once `backends/azure-functions` adopts the overlay-bootstrap shape.
4. **Azure sim — backing-resource auto-creation in `PUT /sites/{siteName}`**: `seedAppServicePlanDefaults` + `seedStorageAccountDefaults` mirroring the gcf ↔ Cloud Run linkage from BUG-901.

**Next session work order:**

1. Run the parallel-fix sweep above (one commit per concern, on the same `phase-118-faas-pods` branch).
2. ~~Phase 122 (GCP runner dispatcher)~~ SHIPPED 2026-05-02 on PR #123. `github-runner-dispatcher-gcp/` uses `cloud.google.com/go/run/apiv2` for Cloud Run Jobs creation; reuses the AWS dispatcher's poller / scopes via `replace github.com/sockerless/github-runner-dispatcher-aws => ../github-runner-dispatcher-aws` (poller + scopes promoted from `internal/` to `pkg/` so cross-module imports resolve). The original `github-runner-dispatcher` was renamed to `github-runner-dispatcher-aws` for naming consistency.
3. Phase 122b (Azure runner dispatcher): mirror onto ACA Jobs API (`Microsoft.App/jobs`).
4. Build + push the four runner images (`tests/runners/{github,gitlab}/dockerfile-{cloudrun,gcf}/Makefile` — `make all`).
5. Original work-order item: re-check PR #123 CI status (`gh pr checks 123`); ALL GREEN at `a646602` but every commit triggers a fresh run.
2. Build + push the four runner images (`tests/runners/{github,gitlab}/dockerfile-{cloudrun,gcf}/Makefile` — `make all`).
3. Configure `~/.sockerless/dispatcher/config.toml` with the two new label entries (cells 5+6).
4. `docker run` the two long-lived gitlab-runner containers (cells 7+8).
5. Run each cell harness: `go test -v -tags=gcp_runner_live -run TestCell5_GH_Cloudrun -timeout 30m ./tests/runners/gcp-cells` (and the others).
6. Capture GREEN URLs in STATUS.md's 4-cell table; iterate on bugs (BUG-887+) until all four GREEN.
7. Tear down: stop gitlab-runner containers; stop dispatcher; eventually `gcloud projects delete sockerless-live-46x3zg4imo`.

See `manual-tests/04-gcp-runner-cells.md` for the full operator runbook.

1. ✅ **Sub-118a — Fix BUG-886** (closed 2026-05-02). Cursor `>=lastTS` + per-entry seen-set dedup + 18s settle window + write-error pipe-close detection. cloudrun manual sweep ALL 16 ROWS PASS.
2. ✅ **gcf full sweep retest** (closed 2026-05-02). After adding `CheckLogBuffers: true` to `core.AttachViaCloudLogs`: `manual-test-real-workloads.sh gcf` ALL 16 ROWS PASS — including end-to-end Go-build-and-run in `golang:1.22-alpine` through the overlay-and-swap path.
3. ✅ **Sub-118b code complete** (2026-05-02): `backends/lambda/pool.go` (claim/release via tags), `backend_impl.go` ContainerCreate pool-query + post-create tagging, ContainerRemove pool-release-or-delete, `config.go` SOCKERLESS_LAMBDA_POOL_MAX (default 10). Build + vet + unit tests pass. ⏳ Live-AWS test pending operator authorization — separate from main flow.
4. ✅ **Sub-118d-gcf — FaaS pod implementation for the gcf backend** (code complete 2026-05-02). Five files touched, all unit-tested:
   - `agent/cmd/sockerless-gcf-bootstrap/main.go` — supervisor mode when `SOCKERLESS_POD_CONTAINERS` is set: fork+chroot+exec per non-main pod member as long-lived sidecar; main member runs in foreground per HTTP invoke; sidecar stdout teed to supervisor stdout with `[<name>] ` prefix; honest namespace-degradation warning at startup.
   - `backends/cloudrun-functions/image_inject.go` — `PodOverlaySpec`, `RenderPodOverlayDockerfile` (multi-stage COPY --from per member; first member's rootfs cp -a snapshot before layered COPYs), `EncodePodManifest`/`DecodePodManifest`, `PodOverlayContentTag`, `TarPodOverlayContext`.
   - `backends/cloudrun-functions/pod_materialize.go` — `materializePodFunction` collapses the pod into one Function (deletes per-member throwaways → builds merged pod overlay → CreateFunction with `sockerless_pod=<name>` label → UpdateService image swap → HTTP invoke fanning result to all member WaitChs + LogBuffers).
   - `backends/cloudrun-functions/backend_impl.go::ContainerStart` — replaces the previous multi-container rejection with the `PodDeferredStart` → `materializePodFunction` path. Single-container path unchanged.
   - `backends/cloudrun-functions/cloud_state.go::queryFunctions` — pod-aware row emission: when `sockerless_pod` label set, decode `SOCKERLESS_POD_CONTAINERS` and emit one `docker ps` row per member with `HostConfig.PidMode = "shared-degraded"` + `Config.Labels["sockerless.namespace.*"]` honesty surface.
   - Tests: `cmd/sockerless-gcf-bootstrap/main_test.go` (parsing, quoting, prefix-writer, pod-main pick), `image_inject_pod_test.go` (manifest roundtrip, dockerfile rendering, content-tag stability), `pod_materialize_test.go` (containers→spec conversion across name/unnamed/main-at-zero variants), `pod_cloud_state_test.go` (pod members from function, degradation labels). All pass under `go test -run ...`. Live verification deferred to sub-118e cell sweeps.
5. 🚧 **Sub-118d-lambda — FaaS pod implementation for the Lambda backend (NEXT)**. Mirror the gcf work to lambda. Mostly mechanical from the gcf design:
   - `agent/cmd/sockerless-lambda-bootstrap/main.go` — add supervisor mode that pre-warms sidecars on init (Lambda invocation is `lambda.Invoke`, so sidecars start at function-instance init not per-request); main member runs via the existing Runtime API loop.
   - `backends/lambda/image_inject.go` — add `RenderPodOverlayDockerfile` + `PodOverlayContentTag` siblings to the existing single-container helpers.
   - `backends/lambda/pod_materialize.go` (new) — same shape as `backends/cloudrun-functions/pod_materialize.go`. Per-member throwaway Lambdas deleted; merged pod Lambda created with `sockerless-pod=<name>` tag (Lambda uses tags not labels) + `SOCKERLESS_POD_CONTAINERS` env.
   - `backends/lambda/backend_impl.go::ContainerStart` — wire `PodDeferredStart` → `materializePodFunction`.
   - `backends/lambda/cloud_state.go` — pod-aware row emission via `Function.Tags["sockerless-pod"]` lookup.
5. 🚧 **Sub-118e — 4 new live-GCP runner cells (AFTER 118d)**:
   - **Cell 5 GH × cloudrun**: GitHub Actions self-hosted runner with docker-executor → sockerless cloudrun. User workflow `container:` directive becomes a Cloud Run Job per step. `services:` (e.g. postgres sidecar) ride along as a sockerless pod (multi-container in one Cloud Run Job's task definition). Goal: `echo hello-from-sockerless-cloudrun` runs in a workflow.
   - **Cell 6 GH × gcf**: same shape but every container goes through the gcf overlay-and-swap path; pods become a multi-container overlay with the supervisor-in-overlay bootstrap (sub-118d output).
   - **Cell 7 GL × cloudrun**: GitLab Runner with docker-executor → cloudrun. Per-stage scripts map to per-stage Cloud Run Jobs. Helper / pre / post containers as pod members.
   - **Cell 8 GL × gcf**: stages reuse pool entries via overlay-content-hash (same image content = same Function reused, sub-118b-style amortized cold start). Helpers as pod members via 118d.
   - Goal: each cell's hello-world workflow runs end-to-end through the runner; `services:` (e.g. postgres) demonstrates localhost peer reachability via the supervisor pattern.
   - Harness pattern: mirror `tests/runners/{github,gitlab}/` cells 1-4 (existing AWS runner-cells); the fixtures are the same, only the backend differs.
6. **Sub-118c — AZF live track + greenfield pool** (defer until 118e closes): needs Azure subscription + service principal from operator. Then `agent/cmd/sockerless-azf-bootstrap`, ACR overlay-build, `WebApps.CreateOrUpdate(linuxFxVersion=DOCKER|<overlay>)`, pool reuse. Cells 9-12 (GH/GL × azf-cloudrun-equivalent / azf-functions) would mirror 118e for Azure.
7. **Teardown** — `gcloud projects delete sockerless-live-46x3zg4imo` once 118d + 118e close.

## Live-cloud project state at session boundary

- **Project**: `sockerless-live-46x3zg4imo` (free-trial billing, ephemeral-project workflow per `feedback_teardown_aggressive.md` adapted for GCP)
- **Service account**: `sockerless-runner@sockerless-live-46x3zg4imo.iam.gserviceaccount.com` with run.admin / run.invoker / cloudfunctions.developer / iam.serviceAccountUser / logging.viewer / storage.admin / artifactregistry.writer / cloudbuild.builds.editor + bucket-level objectAdmin on the build bucket
- **SA key**: `/tmp/sockerless-live-46x3zg4imo-key.json` — set `GOOGLE_APPLICATION_CREDENTIALS` to this for gcf invoke (idtoken signing requires SA creds; user-cred ADC fails loudly)
- **AR repos**: `docker-hub` (remote-Docker-Hub proxy), `sockerless-overlay` (overlay images), `sockerless-live` (initial)
- **GCS bucket**: `gs://sockerless-live-46x3zg4imo-build` (Cloud Build context + stub-Buildpacks-Go source archive)
- **Background backends running**: cloudrun on `127.0.0.1:3375` (PID in `/tmp/sockerless-live-logs/cloudrun.pid`), gcf on `127.0.0.1:3376` (PID in `/tmp/sockerless-live-logs/gcf.pid`). Logs in `/tmp/sockerless-live-logs/`.
- **Test-results dir**: `/tmp/sockerless-real-workloads/{cloudrun,gcf}/`
- **Manual sweep script**: `scripts/manual-test-real-workloads.sh` (bundled probes + Go-build workload)

## Active blockers

Phase 110 closed; cells stable. Historic blockers retained below for context only.

### Historic blockers (all closed)

1. **BUG-868 (Phase 114, substantial) — gitlab-runner stdin-piped per-stage scripts vs Fargate's no-runtime-stdin + non-restartable tasks.** Verified live with `--debug`: cell 3 traces (https://gitlab.com/e6qu/sockerless/-/jobs/14144936826 + 14146329550) show `prepare_script → get_sources → archive_cache_on_failure → upload_artifacts_on_failure → cleanup_file_variables → ERROR exit 1` — gitlab-runner's failure-path cleanup chain. `step_script` is silently skipped because `get_sources` did no real work (stdin script never delivered to predefined helper). **Phase 114 implementation**: launch the predefined helper as a long-lived Fargate task (`Cmd=["while true; do sleep 60; done"]`), cache `(containerID → taskARN)`, dispatch each stage's stdin script via `ecs.ExecuteCommand` (SSM Session Manager) against the live task, capture exit-code marker. Reuses existing SSM frame-capture from Round-8. ~400-600 lines. Detailed implementation plan + gitlab-runner architectural refresher in `PLAN.md § Phase 114`. Doc: `specs/CLOUD_RESOURCE_MAPPING.md § "ECS gitlab-runner script delivery"`.
2. **Phase 117 — gitlab-runner per-stage script delivery on Lambda (cell 4).** Independent of Phase 114. Each gitlab-runner stage's stdin-piped script becomes one `lambda.Invoke` with a SCRIPT envelope `{"sockerless":{"script":{"body":"<base64>",...}}}`. The bootstrap parses + runs `bash -c "<body>"`. EFS for cross-stage state. ~250-400 lines. See `PLAN.md § Phase 117`.

Cells 1 + 2 GREEN. PR #122 CI GREEN at `88aca1e`. Recent pushes:

- `99c8ca0` — BUG-869 + BUG-870 (CodeBuild buildspec → Docker schema 2; EFS access point lookup)
- `b3be64f` — BUG-871 + BUG-872 (Lambda single-FSC + `/mnt/...` mount path collapse + symlinks; cache prefix mismatch)
- `d5073b4` — Phase 115 always-on overlay-inject (closes BUG-873)
- `455c019` — Phase 116 exec-via-Invoke partial (Path B + bind-link bake + workdir off Lambda)
- `9695341` — Phase 116 wire-up via lambdaInvokeExecDriver in Typed.Exec (closes BUG-874; cell 2 GREEN at workflow run https://github.com/e6qu/sockerless/actions/runs/25113565115)

## Operational state — 2026-04-29 ~00:00 UTC

- **AWS creds:** ⚠ expired; was active via `aws.sh` (root `729079515331`). Refresh before resuming.
- **Live AWS infra: UP in eu-west-1.** ECS cluster `sockerless-live` (35 base + 4 runner-extension resources). Lambda live env (8 resources). EFS `fs-069c02e0e8823b64e` with two access points: `runner_workspace=fsap-0f60e569bae585f25`, `runner_externals=fsap-0ff9f9686208c4ed7`.
- **Runner-task ECS task definition:** `sockerless-live-runner:2` registered in eu-west-1. Single-container Fargate (1024 CPU / 2048 MB, X86_64). Image: `729079515331.dkr.ecr.eu-west-1.amazonaws.com/sockerless-live:runner-amd64` (latest digest pushed to ECR; `LABEL com.sockerless.ecs.task-definition-family=sockerless-runner`). EFS volumes mounted at `/home/runner/_work` and `/home/runner/externals`; entrypoint pre-populates externals on first start (tar pipe).
- **Sockerless code changes (BUG-850..853 — all on this branch):**
  1. `Config.SharedVolumes` + bind-mount → EFS translation in `backends/ecs/backend_impl.go`. Sub-path drop. `/var/run/docker.sock` drop.
  2. `accessPointForVolume` short-circuits to `SharedVolume.AccessPointID` when the named volume matches a configured shared volume.
  3. ECS server overrides `s.Drivers.Network` with `SyntheticNetworkDriver` (metadata-only — Fargate has its own netns, Linux netns is the wrong abstraction for cloud).
  4. Sub-tasks include the operator's default SG alongside per-network SG (so EFS mount targets stay reachable).
  5. `cloudExecStart` waits for `ExecuteCommandAgent.LastStatus == RUNNING` before issuing `ExecuteCommand`.
- **PR-#122 commits:** working tree has all the above plus state-doc updates; ready to commit + push.

## Phase 110b — Cell 1 status: ✅ GREEN

**Successful run:** https://github.com/e6qu/sockerless/actions/runs/25052661438 (commit `7362197` pushed 2026-04-28).

All 6 workflow steps passed. `container: alpine:latest` directive flowed through sockerless's bind-mount → EFS translation; sub-task spawned in Fargate with shared EFS access points; `docker exec` succeeded after the new ExecuteCommandAgent-ready wait.

Iteration history (recorded for future debugging):
- 25049909614 — Initialize containers failed: bind mount rejection (BUG-850 not yet shipped).
- 25051339655 — Initialize failed: netns (BUG-851).
- 25051469196 — Initialize failed: EFS mount timeout from sub-task (BUG-852).
- 25051866900 — Initialize ✓; Run echo failed exit 255 (BUG-853).
- 25052043048 — Initialize ✓; exec failed: missing `ecs:ExecuteCommand` IAM perm.
- 25052216785, 25052362819 — same exec-agent-not-ready failure (BUG-853 confirmed, fix not yet shipped).
- 25052661438 — **GREEN** — first run with the BUG-853 wait fix shipped.

## 4-cell verification status (2026-04-29 ~14:30 UTC)

| Cell | Status | Latest evidence URL | Next |
|---|---|---|---|
| 1 GH × ECS | ✅ PASS | https://github.com/e6qu/sockerless/actions/runs/25075259911 | re-run during sweep once 3/4 verified |
| 2 GH × Lambda | ✅ PASS | https://github.com/e6qu/sockerless/actions/runs/25113565115 | re-run during sweep once 3/4 verified |
| 3 GL × ECS | 🟡 step_script skipped (BUG-868) | latest https://gitlab.com/e6qu/sockerless/-/jobs/14144936826 | implement Phase 114 (long-lived BUILD container + SSM ExecuteCommand for stdin-piped script delivery) |
| 4 GL × Lambda | ⏸ inherits Phase 114 pattern | n/a | adapt Phase 114 to Lambda primitives (Path B exec-via-Invoke from Phase 116 already covers single execs; need gitlab-runner stdin-pipe → lambda.Invoke flow) |

## CI status

✅ **PR #122 CI fully GREEN** as of commit `88aca1e` (BUG-866 v2): all 10 jobs PASS — lint, test, test (e2e), sim (aws/gcp/azure), ui, build-check, smoke, terraform.

## Bugs shipped this iteration (PR #122)

- BUG-859 (H, ECS attach stdin)
- BUG-860 (H, Lambda attach stdin)
- BUG-861 (H, Lambda externals shared-volume entry — symptom of BUG-862)
- BUG-862 (CRITICAL, runner-Lambda baked wrong backend — codified class-of-bug rule)
- BUG-863 (M, integration / smoke / test arch env var missing)
- BUG-864 (L, terraform-test substring-match false positive)
- BUG-865 (H, image-resolve routes locally-built images through Public Gallery)
- BUG-866 (H, deferred-stdin path entered too eagerly — v1 fall-through, v2 only-when-pipe-loaded)
- BUG-869 (H, CodeBuild buildspec produced OCI manifest; Lambda image-mode rejects)
- BUG-870 (H, EFS access-point ARN lookup filtered by `sockerless-managed` tag — operator-provisioned APs lacked it)
- BUG-871 (H, Lambda single-FSC + `/mnt/...` mount path constraint — collapse + BIND_LINKS bootstrap symlinks + EFS subpath in SharedVolume)
- BUG-872 (H, pull-through cache prefix mismatch with ECS — derive prefix the same way both backends do)

## Up next on this branch — Phase 116 (BUG-873) and Phase 114 (BUG-868)

Phase 116 — Lambda image-mode requires Docker schema 2 manifests AND Runtime API client at the entrypoint. Cell 2's alpine image fails both. Architectural fix: route ALL Lambda CreateFunction calls through `BuildAndPushOverlayImage` overlay-inject, swapping its `os/exec docker build` for `awscommon.CodeBuildService` so it works inside the runner-Lambda. Cache converted images by source-content hash. Implementation steps:

1. Refactor `BuildAndPushOverlayImage` in `backends/lambda/image_inject.go`: accept a `core.CloudBuildService` dependency. When available, build via CodeBuild (already wired via `s.images.BuildService` in `server.go:72-76`); else fall back to local docker.
2. `backend_impl.go` create flow: drop the no-CallbackURL default branch. Always go through overlay-inject.
3. New ECR repo (`sockerless-live-overlay`) for converted images, tag = sha256 of `BaseImageRef + AgentBinaryPath + BootstrapBinaryPath + UserEntrypoint + UserCmd`. Skip rebuild on cache hit.
4. `specs/CLOUD_RESOURCE_MAPPING.md` Lambda mapping row: extend with "Lambda images go through overlay-inject; OCI inputs auto-converted to Docker schema 2 by the overlay build."

Phase 114 — gitlab-runner `start-attach-script` per-command lifecycle. Each script step does `docker start <helper>` + `docker attach`. On Fargate the task entrypoint runs once and exits — gitlab-runner expects the helper to stay running. Fix: keep the task alive with synthetic `tail -f /dev/null`-style entrypoint, route each /start's script through SSM ExecuteCommand. Implementation steps:

1. Add a "long-lived helper" mode to ECS ContainerStart when ECSState.OpenStdin is true and the gitlab-runner -predefined suffix is absent (i.e. user-script container).
2. First /start: run a task whose entrypoint is `sh -c 'while sleep 60; do :; done'`; record the task ARN.
3. Subsequent /start cycles for the same container ID: skip RunTask; use SSM ExecuteCommand to run the buffered stdin bytes as a script in the existing task.
4. /attach reads from SSM session output (already implemented for `docker exec`).
5. Container stop: `ecs.StopTask`.

After Phases 111 + 112 land, all 4 cells should reach GREEN.

## Original cell-2 unblock recipe (now superseded by Phase 116)

Source-side corrections shipped through commit `b3be64f`. Full runner hurdle catalog (15 closed + 8 predicted) in [docs/RUNNERS.md § Runner hurdles](docs/RUNNERS.md) — that's where future-debugging starts.

The operator-driven runtime steps below remain accurate as the live-infra prep needed for any cell-2 verification run after Phase 116 lands:

### Step 1 — Apply terraform (cells 2 + 4 prep)

```bash
cd /Users/zardoz/projects/sockerless/terraform/environments/lambda/live
source aws.sh
terragrunt apply
```

Provisions on `sockerless-live`:
- `sockerless-live-image-builder` CodeBuild project (linux/amd64 standard, privileged docker, inline buildspec)
- `sockerless-live-build-context` S3 bucket (24-hour lifecycle on `build-context/` prefix)
- IAM role `sockerless-live-codebuild-role` (S3 read + ECR push + CloudWatch Logs)
- Updates `sockerless-live-runner` Lambda: ECS dispatch IAM perms → Lambda dispatch perms; env vars all `SOCKERLESS_LAMBDA_*` (workspace + externals SHARED_VOLUMES, plus CODEBUILD_PROJECT + BUILD_BUCKET).

### Step 2 — Rebuild runner-Lambda image (cell 2 unblock)

No local Docker daemon needed:

```bash
cd /Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda
make codebuild-update
```

Pipeline: `make stage` (cross-compile linux/amd64 backend + agent + bootstrap into the build context) → `make upload-context` (tar + S3 upload) → `make codebuild-build` (start CodeBuild + poll every 10 s until SUCCEEDED) → `make update-function` (`aws lambda update-function-code --publish`) → `make wait` (`aws lambda wait function-updated-v2`).

Local-Docker alternative if preferred: `make all`.

### Step 3 — Restart sockerless backends (cells 3 + 4 unblock)

```bash
kill 75092 70870
source /Users/zardoz/projects/sockerless/aws.sh
source /tmp/ecs-env.sh
nohup /tmp/sockerless-backend-ecs    -addr :3375 -log-level debug \
    >>/tmp/sockerless-ecs.log    2>&1 &
source /tmp/lambda-env.sh
nohup /tmp/sockerless-backend-lambda -addr :3376 -log-level debug \
    >>/tmp/sockerless-lambda.log 2>&1 &
curl -s http://localhost:3375/_ping; echo
curl -s http://localhost:3376/_ping; echo
```

The macOS-arm64 binaries at `/tmp/sockerless-backend-{ecs,lambda}` were rebuilt this session and contain BUG-859 / BUG-860 fixes.

### Step 3a — Cell-4 prerequisite: agent + bootstrap on disk

The laptop sockerless-backend-lambda's image-inject path needs the agent + bootstrap binaries available locally. Pick one:

```bash
# Option A: copy the linux/amd64 binaries to /opt/sockerless/
sudo mkdir -p /opt/sockerless && \
  sudo cp /Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda/sockerless-agent /opt/sockerless/ && \
  sudo cp /Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda/sockerless-lambda-bootstrap /opt/sockerless/

# Option B: append env vars to /tmp/lambda-env.sh before re-sourcing it in step 3:
cat >> /tmp/lambda-env.sh <<'EOF'
export SOCKERLESS_AGENT_BINARY=/Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda/sockerless-agent
export SOCKERLESS_LAMBDA_BOOTSTRAP=/Users/zardoz/projects/sockerless/tests/runners/github/dockerfile-lambda/sockerless-lambda-bootstrap
export SOCKERLESS_CODEBUILD_PROJECT=sockerless-live-image-builder
export SOCKERLESS_BUILD_BUCKET=sockerless-live-build-context
EOF
```

### Step 4 — 4-cell verification sweep

Tell me when steps 1-3 are done and I'll fire all four cells:

```bash
go test -v -tags github_runner_live -run TestGitHub_ECS_Hello    -timeout 30m ./tests/runners/github
go test -v -tags github_runner_live -run TestGitHub_Lambda_Hello -timeout 30m ./tests/runners/github
go test -v -tags gitlab_runner_live -run TestGitLab_ECS_Hello    -timeout 30m ./tests/runners/gitlab
go test -v -tags gitlab_runner_live -run TestGitLab_Lambda_Hello -timeout 30m ./tests/runners/gitlab
```

I'll capture all four run / pipeline URLs back into this doc. Phase 110 closes when all four are GREEN with their evidence URLs recorded.

### After all four cells GREEN

5. Update `docs/runner-capability-matrix.md`: TBD → PASS for cells 1-4.
6. Phase 110b dispatcher wiring: ECR push pipeline for the dispatcher's own runner image; end-to-end harness wiring through the dispatcher binary (vs the current per-cell direct dispatch).
7. **Tear down live AWS** at session end (`terragrunt destroy` from both `terraform/environments/{ecs,lambda}/live`).

## Sockerless restart command

```bash
kill 75092 70870
source /Users/zardoz/projects/sockerless/aws.sh
source /tmp/ecs-env.sh
nohup /tmp/sockerless-backend-ecs    -addr :3375 -log-level debug \
    >>/tmp/sockerless-ecs.log    2>&1 &
source /tmp/lambda-env.sh
nohup /tmp/sockerless-backend-lambda -addr :3376 -log-level debug \
    >>/tmp/sockerless-lambda.log 2>&1 &
curl -s http://localhost:3375/_ping; echo
curl -s http://localhost:3376/_ping; echo
```

The macOS-arm64 binaries at `/tmp/sockerless-backend-{ecs,lambda}` were rebuilt this session and contain BUG-859 / BUG-860 fixes.

## Resume notes

- **Live infra is UP** — re-run `terragrunt destroy` when done with the session.
- Sockerless ECS backend running locally on `:3375` (laptop), Lambda on `:3376`. Both in `eu-west-1`.
- Runner image already pushed to ECR; ECS task def revision 2 active.
- `gh auth token` keychain-backed; GitLab PAT in `security` keychain.
- The architecture proven by run 25052661438 generalizes to Lambda once SharedVolumes is mirrored. The user said "no fakes / no fallbacks / no workarounds" — Lambda work should follow the same shape.

## Bug log this session (PR #122)

860 fixed (BUG-845..860). 2 open: BUG-861 (Lambda externals shared-volume entry) + BUG-862 (CRITICAL — backend ↔ host primitive mismatch, runner-Lambda baked the ECS backend). Both ship as part of the same cell-2 fix round (rebuild runner-Lambda image with sockerless-backend-lambda + apply new terraform). Class-of-bug rule documented at top of [BUGS.md](BUGS.md): cross-cloud-primitive baking is a P0.

| # | Sev | Area | One-liner |
|---|-----|------|-----------|
| 845 | M | terraform | Lambda live env was us-east-1 → realigned to eu-west-1 + sockerless-tf-state. |
| 846 | M | image-resolve | Docker Hub PAT path replaced with AWS Public Gallery routing. |
| 847 | L | tests/runners | GH runner asset URL `darwin` → `osx`; bumped 2.319.1 → 2.334.0. |
| 848 | M | ecs/lambda | `docker info` Architecture from required `SOCKERLESS_*_ARCHITECTURE` env vars. |
| 849 | M | tests/runners | Drop broken `--add-host host-gateway`; install docker CLI in runner image. |
| 850 | H | ecs (bind mounts) | `Config.SharedVolumes` + bind-mount → EFS translation; sub-path drop; docker.sock drop. |
| 851 | M | ecs (network) | Override `s.Drivers.Network` with metadata-only synthetic; netns is wrong for Fargate. |
| 852 | M | ecs (network) | Sub-tasks need operator default SG too (EFS mount target allow-list). |
| 853 | H | ecs (exec) | Wait for `ExecuteCommandAgent.LastStatus == RUNNING` before `ExecuteCommand`. |
| 854 | M | ecs (image-resolve) | sha256-only refs no longer misroute through `public.ecr.aws/docker/library/sha256:...`; resolve via local Store or surface clear error. |
| 855 | M | aws-common (volumes) | EFS access-point path overflow on long volume names — fall back to `/sockerless/v/<sha256[:16]>`. |
| 856 | M | terraform / runner-lambda | `SOCKERLESS_ECS_SHARED_VOLUMES` aligned to Lambda's `/tmp/runner-state/...` paths. |
| 857 | M | tests/runners (gitlab) | gitlab-runner-helper image pre-pushed to ECR + Basic-auth-direct routing for ECR-shaped registries. |
| 858 | M | ecs (container lifecycle) | `ContainerStart` falls back to `ResolveContainerAuto` for STOPPED-then-restarted containers; PendingCreates preserved through `waitForTaskRunning`. |
| 859 | H | ecs (attach stdin) | `ecsStdinAttachDriver` captures `docker attach` stdin into a per-cycle `stdinPipe`; `launchAfterStdin` defers RunTask until stdin EOF then bakes the script into the task definition's `Entrypoint=[sh,-c]` + `Cmd=[<script>]`. ECSState gains `OpenStdin` so per-cycle restarts (gitlab-runner reuses container ID across script steps) survive PendingCreates churn. |
| 860 | H | lambda (attach stdin) | Mirror of BUG-859 for Lambda: `lambdaStdinAttachDriver` captures stdin → buffered → `lambda.Invoke` Payload (the bootstrap pipes Payload to user entrypoint as stdin, so `Cmd=[sh]` runs the script). LambdaState gains `OpenStdin`. |
| 861 | H | runner-lambda image + lambda backend | ⚠ open. Cell 2 fail surfaced "host bind mounts not supported on ECS backend" for `/tmp/runner-state/externals:/__e:ro`. Root cause was BUG-862 (wrong backend baked in); fix lands together with BUG-862's terraform `SOCKERLESS_LAMBDA_SHARED_VOLUMES` carrying both workspace + externals paths. |
| 862 | **CRITICAL** | architecture / runner-lambda | ⚠ open. Runner-Lambda image baked `sockerless-backend-ecs` and dispatched `container:` sub-tasks via `ecs.RunTask` to Fargate — backend ↔ host primitive mismatch. Project rule (now top of BUGS.md + MEMORY.md + CLOUD_RESOURCE_MAPPING.md universal rule #9): each backend runs on its own native primitive. Source fixed (Dockerfile, bootstrap, terraform IAM + env vars, agent + bootstrap binaries staged into image); awaits image rebuild + `terragrunt apply`. |

## Cross-links

- Roadmap: [PLAN.md](PLAN.md)
- Phase roll-up: [STATUS.md](STATUS.md)
- Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md)
- Bug log: [BUGS.md](BUGS.md)
- Runner wiring: [docs/RUNNERS.md](docs/RUNNERS.md)

## Standing rules (carry forward)

- **No fakes, no fallbacks, no workarounds** — every gap is a real bug with a real fix. (User reaffirmed several times this session.)
- **Sim parity per commit** — any new SDK call updates `specs/SIM_PARITY_MATRIX.md` + adds the sim handler.
- **State save after every major piece of work** (PLAN / STATUS / WHAT_WE_DID / DO_NEXT / BUGS) — mandatory at ~80% context.
- **Never merge PRs** — user handles all merges.
- **Branch hygiene** — rebase on `origin/main` before push.
- **`github-runner-dispatcher-aws` is sockerless-agnostic** — pure Docker SDK / CLI client. (GCP variant `github-runner-dispatcher-gcp` and Azure variant `github-runner-dispatcher-azure` use `cloud.google.com/go/run/apiv2` and `armappcontainers` respectively, also sockerless-agnostic — they dispatch directly via the cloud control plane.)
