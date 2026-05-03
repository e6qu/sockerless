# Do Next

Resume pointer. Updated after every task. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); runner wiring in [docs/RUNNERS.md](docs/RUNNERS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-03 v9 — final state of session)

**Goal**: cells 5/6/7/8 GREEN with REAL workload (compile + use eval-arithmetic + probe environment) before merging PR #123.

**Current state**: All 4 cells fail at the docker-executor `prepare_executor` or `get_sources` stage. Each new iteration has surfaced a fresh bug (BUG-907..923), all closed live except BUG-922 (cloudrun container removed after first exec) + BUG-923 (gcf CreateFunction.Wait blocks > 120s gitlab-runner timeout). The remaining 2 bugs are SYMPTOMS of an architectural mismatch documented in `specs/CLOUD_RESOURCE_MAPPING.md`:

> Cloud Run **Jobs** are one-shot. Runner cells need long-lived containers persisting across N `docker exec` calls. The proper fix is the Cloud Run **Service** path (config flag `SOCKERLESS_GCR_USE_SERVICE=1` already exists) PLUS the reverse-agent for `docker exec` (already in ACA, port to cloudrun + gcf).

## Phase 122f infra in flight (2026-05-03 v10)

- VPC Access API + Compute API + Service Networking API ENABLED on `sockerless-live-46x3zg4imo`.
- VPC `sockerless-vpc` + subnet `sockerless-connector-subnet` (`10.8.0.0/28`) created in `us-central1`.
- VPC connector `sockerless-connector` creation submitted (background `bxhes1d50`); ~5-10 min provisioning.
- Spec doc updated to 1129 lines with backend-↔-primitive-purity rule per user 2026-05-03 directive (no cross-contamination; chain Function invocations for FaaS long-lived).

## Phase 122f — proper-fix path (next session)

Per `specs/CLOUD_RESOURCE_MAPPING.md` § "Synthesis — Phase 122f scope":

1. **Enable VPC Access on the live project**: `gcloud services enable vpcaccess.googleapis.com compute.googleapis.com --project=sockerless-live-46x3zg4imo` (currently disabled — UseService validation requires `SOCKERLESS_GCR_VPC_CONNECTOR`).
2. **Create VPC + subnet + connector**: existing terraform module `terraform/modules/cloudrun/main.tf` has `google_compute_network.main`, `google_compute_subnetwork.connector`, `google_vpc_access_connector.main` — apply via terragrunt.
3. **Set `SOCKERLESS_GCR_USE_SERVICE=1` + `SOCKERLESS_GCR_VPC_CONNECTOR=<name>`** in the runner image's bootstrap.sh (auto-discover VPC connector via metadata server / convention). Currently bootstrap auto-discovers project + region; extend.
4. **Port reverse-agent from ACA to cloudrun**: `backends/aca/` has the working impl; `backends/cloudrun/` has the skeleton (`s.reverseAgents` field, `RunContainerChangesViaAgent` in delegates). The `/v1/cloudrun/reverse` endpoint needs to be wired + the in-image bootstrap must dial back when `SOCKERLESS_CALLBACK_URL` is set.
5. **For runner-pattern containers (long-lived)**: use base image directly + Cloud Run Service `Container.command` + `args` override. SKIP the overlay-image build (Lesson 6). Pre-deploy ONE Service per runner-image shape (Lesson 1) — sub-task ContainerStart updates the existing Service's revision env instead of creating a fresh Service.
6. **Pool reuse via min_instance_count toggle**: ContainerStop → `min_instance_count=0` (suspend, no charge), ContainerStart → `min_instance_count=1` (resume).

After Phase 122f: BUG-921/922/923 chain becomes moot (no Jobs.RunJob.Wait, no per-container CreateFunction.Wait, no auto-remove on first exec). Cells 5+7 should GREEN end-to-end. Cells 6+8 (gcf) need the pod-overlay path's UpdateService escape hatch (Phase 118 BUG-884 generalization) for runner-pattern.

## Tactical files for resume

- `backends/cloudrun/start_service.go` — `startSingleContainerService` exists; verify it works for runner-pattern (long-lived w/ Container.command override).
- `backends/cloudrun/backend_delegates.go:36-42` — `RunContainerChangesViaAgent`; reverse-agent skeleton.
- `backends/aca/` — reference impl for reverse-agent + Container Apps exec.
- `tests/runners/github/dockerfile-{cloudrun,gcf}/bootstrap.sh` — auto-discovers project + region from metadata server; extend for VPC connector + USE_SERVICE.
- `github-runner-dispatcher-gcp/internal/spawner/spawner.go` — only sets RUNNER_*; sockerless config is runner-image-internal per the dispatcher scope rule.

## Branch state
- `main` synced with `origin/main` at PR #121 merge.
- `phase-118-faas-pods` (PR #123, 17+ commits this session) — all standard CI green; ready for merge once cells GREEN.
- `cell-workflows-on-main` (PR #124, throwaway) — close after cells 5+6 GREEN; do NOT merge.
- `gitlab-cell-7-test` + `gitlab-cell-8-test` on `origin-gitlab` — fire pipelines for cells 7+8.

## Live infra
All in `sockerless-live-46x3zg4imo` (us-central1):
- Dispatcher Cloud Run Service `github-runner-dispatcher-gcp` rev `00006-j4v`
- gitlab-runner-cloudrun rev `00015-mmb`, gitlab-runner-gcf rev `00016-xc2`
- AR: `sockerless-live`, `docker-hub`, `gitlab-registry`, `sockerless-overlay/gcf`
- Secret Manager: `github-pat`, `gitlab-pat`, `gitlab-runner-token-{cloudrun,gcf}`
- GCS: `sockerless-live-46x3zg4imo-build`, `sockerless-live-46x3zg4imo-runner-workspace`

## Resume runbook (next session, condensed)
1. Read `specs/CLOUD_RESOURCE_MAPPING.md` § "Lessons from ECS + Lambda backends → cloudrun + gcf adjustments" (the Phase 122f synthesis).
2. `gcloud services enable vpcaccess.googleapis.com compute.googleapis.com --project=sockerless-live-46x3zg4imo`.
3. `cd terraform/environments/<live-gcp> && terragrunt apply` to create VPC + subnet + connector (or apply just the cloudrun module).
4. Update `tests/runners/github/dockerfile-cloudrun/bootstrap.sh` to set `SOCKERLESS_GCR_USE_SERVICE=1` + `SOCKERLESS_GCR_VPC_CONNECTOR=<auto-discovered>`.
5. Implement reverse-agent end-to-end for cloudrun (port from ACA).
6. Switch ContainerCreate runner-pattern detection: `tail -f /dev/null` cmd OR explicit `sockerless.runner-pattern=true` label → use Service path with command override (skip overlay).
7. Rebuild + push runner images + deploy.
8. Re-fire cells 5+6+7+8 → expect GREEN.
9. Capture three URLs per cell into STATUS.md.
10. Close PR #124 (do NOT merge). PR #123 ready for user merge.
