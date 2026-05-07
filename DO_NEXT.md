# Do Next

Resume pointer. Roadmap [PLAN.md](PLAN.md) · status [STATUS.md](STATUS.md) · bugs [BUGS.md](BUGS.md) · narrative [WHAT_WE_DID.md](WHAT_WE_DID.md) · architecture [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Active work branch — `phase-130` (PR #127)

Single work-branch rule: ALL in-flight work lands here, no side branches. PR #127 grows as commits land.

### 1. Sim parity prep — DONE

- [x] **GCP `iamcredentials.generateIdToken`** — `simulators/gcp/iam.go` extended; `mintSimIdToken` helper in `oauth2.go`.
- [x] **GCP Compute Disks CRUD** — `simulators/gcp/compute.go::registerComputeDisks`. Zonal Insert/Get/List/Delete/Resize/SetLabels + aggregated-list + zonal-ops endpoint. Phase 127 GCP `pd-ephemeral` prep.
- [x] **SDK tests** (6 new in `simulators/gcp/sdk-tests/`): full disk CRUD; aggregated list; not-found; ID-token audience round-trip; missing-SA 404; missing-audience 400. All PASS.
- [x] **`specs/SIM_PARITY_MATRIX.md`** — 8 new rows under GCP § "Phase 126/127 forward-looking (no current backend caller; SDK-test-validated)".

### 2. Phase 130 — bleephub workflow-runs REST (DONE)

Shipped: `bleephub/gh_actions_rest.go` registers all 10 GitHub-shape routes (runs list/get/jobs/cancel/rerun/delete + jobs get/logs + runners list/delete). `bleephub/gh_actions_test.go` covers each endpoint shape (14 new tests, all PASS; full bleephub suite green at 22s). JSON converters bridge bleephub's internal `Workflow`/`WorkflowJob`/`Agent` → GitHub-shape JSON; FNV-1a 64-bit gives stable int64 GitHub-style job IDs from the internal UUIDs.

`POST .../runs/{id}/rerun` returns 422 with a clear message pointing at the existing `/api/v3/bleephub/workflow` submit path — Phase 131 ships the proper `/actions/workflows/{id}/dispatches` route.

### 3. Phase 131 — bleephub workflows REST + UI dispatch (DONE)

Shipped: `bleephub/gh_workflows_rest.go` (4 GitHub-shape routes) + `bleephub/store_workflow_files.go` (new `WorkflowFile` entity with FNV-1a int64 IDs + go-git tree-walk discovery from each repo's in-memory storer at HEAD) + auto-register on `/api/v3/bleephub/workflow` submit. Phase 130's `rerun` handler now wires through the WorkflowFile cache. UI: `WorkflowsPage` has Workflows + Runs tabs + dispatch dialog (ref + inputs JSON). 10 new Go tests + 4 new UI tests PASS; full bleephub Go suite green at 23s; UI test suite 17/17 PASS.

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
