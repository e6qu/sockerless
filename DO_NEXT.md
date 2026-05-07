# Do Next

Resume pointer. Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Active work branch — `phase-130` (PR #127)

Single work-branch rule: ALL in-flight work lands here, no side branches. PR #127 grows as commits land.

### 1. Sim parity prep — finish before resuming bleephub

- [x] **GCP `iamcredentials.generateIdToken`** — `simulators/gcp/iam.go` extended; `mintSimIdToken` helper added in `oauth2.go`. Build green.
- [ ] **GCP Compute Disks CRUD** — `simulators/gcp/compute.go`. Zonal disks (Insert/Get/List/Delete/Resize/SetLabels) + aggregated list. Phase 127 GCP `pd-ephemeral` prep.
- [ ] **SDK tests** — `simulators/gcp/sdk-tests/`: cover `generateIdToken` (token shape + audience claim) and Compute Disks CRUD round-trip.
- [ ] **`specs/SIM_PARITY_MATRIX.md`** — add rows for both new APIs.

### 2. Phase 130 — bleephub workflow-runs REST (next)

Goal: unmodified `gh` CLI + the existing GitHub-runner-dispatcher work against bleephub end-to-end (so the 8/8 runner-integration cells could run against bleephub instead of real GitHub for hermetic test coverage).

New file `bleephub/gh_actions_rest.go` registering:

- `GET /api/v3/repos/{o}/{r}/actions/runs` (with `?status=`, `?branch=`, `?event=`)
- `GET /api/v3/repos/{o}/{r}/actions/runs/{run_id}`
- `GET /api/v3/repos/{o}/{r}/actions/runs/{run_id}/jobs`
- `GET /api/v3/repos/{o}/{r}/actions/jobs/{job_id}`
- `GET /api/v3/repos/{o}/{r}/actions/jobs/{job_id}/logs`
- `POST /api/v3/repos/{o}/{r}/actions/runs/{run_id}/cancel`
- `POST /api/v3/repos/{o}/{r}/actions/runs/{run_id}/rerun`
- `DELETE /api/v3/repos/{o}/{r}/actions/runs/{run_id}`
- `GET /api/v3/repos/{o}/{r}/actions/runners`
- `DELETE /api/v3/repos/{o}/{r}/actions/runners/{runner_id}`

JSON shape converters: `workflowRunJSON(*Workflow, baseURL)`, `workflowJobJSON(*WorkflowJob, *Workflow, *Job, baseURL)`, `runnerJSON(*Agent)`. Wired from `server.go::registerRoutes` via new `registerGHActionsRoutes()`. Tests in `bleephub/gh_actions_test.go` cover each endpoint shape against in-memory store.

### 3. Phase 131 — bleephub workflows REST + UI dispatch (after 130)

User chose "more complete": auto-parse `.github/workflows/*.yml` from a repo-on-disk; the bleephub UI gets workflow-dispatch form.

- `GET /api/v3/repos/{o}/{r}/actions/workflows` (list YAML files)
- `GET /api/v3/repos/{o}/{r}/actions/workflows/{id}` (read metadata)
- `GET /api/v3/repos/{o}/{r}/actions/workflows/{id}/runs`
- `POST /api/v3/repos/{o}/{r}/actions/workflows/{id}/dispatches` (with `inputs`, `ref`)
- UI: refactor `WorkflowsPage` into Workflows + Runs tabs; dispatch form.

### 4. Phase 132 — apps + oauth completeness (after 131)

- `GET /user/installations`, `GET /user/installations/{id}/repositories`, `DELETE /installation/token`.
- `GET /login/oauth/authorize` (web flow; companion to existing device flow).
- UI pages: Apps Manager + OAuth Debug. Admin UI gets bleephub admin sub-pages too.

## Blocked

**Live-cloud verification of Phase 129 #4** requires a fresh ephemeral GCP project per `project_gcp_live_setup.md`. Don't bring up new live infra until Phase 128 (job timeout) + the rest of Phase 129 (BigQuery billing export, per-session labels, budget alert, session-end teardown) ship — without those, the regional-CPU-quota debt cycle from 2026-05-07 repeats. 6-day project cost was ~$90 (no per-service breakdown without Console / BigQuery export).

## Project rules

- Never merge PRs — user handles all merges.
- Never push `main`.
- Single work-branch rule — everything stacks on `phase-130`; no side branches.
- New failures during this work file in [BUGS.md](BUGS.md) before any fix attempt.
- Each new driver phase (124–127) starts with a `specs/CLOUD_RESOURCE_MAPPING.md` design pass before code.
