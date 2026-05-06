# Do Next

Resume pointer for next session. Detail: [STATUS.md](STATUS.md), [PLAN.md](PLAN.md), [WHAT_WE_DID.md](WHAT_WE_DID.md), [BUGS.md](BUGS.md).

## Today's outcome (2026-05-06)

Cell 7 v54 + cell 8 v28 GREEN. Three architectural fixes shipped (BUG-956 + BUG-957 + BUG-958). Cells 5+6 (GitHub × GCP) still need a workflow trigger. PR #123 has all the code at `phase-118-faas-pods @ e97399c`.

## Resume sequence

### Step 1 — Trigger cells 5 + 6

The workflows have `workflow_dispatch:` AND `pull_request:` triggers, but the workflow files are NOT on `main` yet, so `gh workflow run` returns HTTP 422 ("Workflow does not have 'workflow_dispatch' trigger" — gh resolves dispatch off main only).

PR #124 is the throwaway PR designed to fire these workflows via `pull_request` event. Path filter is `paths: [.github/workflows/cell-N-*.yml]` so the trigger commit must touch the cell yml file itself.

```bash
# Find PR #124's branch
gh pr view 124 --repo e6qu/sockerless --json headRefName,state -q '.headRefName + " (" + .state + ")"'

# If still open: switch to its branch, touch the cell ymls, push.
git fetch origin pull/124/head:pr-124
git checkout pr-124
# Make a no-op edit to fire the path filter:
sed -i '' "1s/.*/# Cell 5 v$(date +%s) — re-trigger after BUG-958 fix/" .github/workflows/cell-5-cloudrun.yml
sed -i '' "1s/.*/# Cell 6 v$(date +%s) — re-trigger after BUG-957+958 fix/" .github/workflows/cell-6-gcf.yml
git add .github/workflows/cell-5-cloudrun.yml .github/workflows/cell-6-gcf.yml
git commit -m "trigger: cells 5+6 against phase-118-faas-pods bootstrap (BUG-957/958)"
git push origin pr-124
git checkout phase-118-faas-pods
```

### Step 2 — Watch dispatcher + runner-task

```bash
# Dispatcher should spawn one Cloud Run Job per workflow_job:
gcloud logging read 'resource.labels.service_name=github-runner-dispatcher-gcp AND timestamp>="'"$(date -u -v-5M +%Y-%m-%dT%H:%M:%SZ)"'"' \
  --project=sockerless-live-46x3zg4imo --limit=30 --order=desc --format='value(timestamp,textPayload,jsonPayload.message)'

# Runner-task executions:
gcloud run jobs executions list --project=sockerless-live-46x3zg4imo --region=us-central1

# Per-execution logs (replace EXEC_NAME):
gcloud logging read 'resource.type=cloud_run_job AND resource.labels.job_name=~"gh-" AND timestamp>="'"$(date -u -v-10M +%Y-%m-%dT%H:%M:%SZ)"'"' \
  --project=sockerless-live-46x3zg4imo --limit=50 --order=asc --format='value(timestamp,textPayload)'
```

### Step 3 — Closeout

After cells 5+6 GREEN:
1. Update STATUS.md cell scoreboard (mark 5+6 ✅).
2. Update WHAT_WE_DID.md with cells 5+6 outcome.
3. Update PR #123 description to reflect all 8 cells GREEN.
4. State save commit. NEVER MERGE — user handles merges.
5. Tear down `sockerless-live-46x3zg4imo` if no further live work needed (`./scripts/teardown-gcp-live.sh` or equivalent).

## Already-pushed assets (carried into resume)

- Backend digests (deployed on `gitlab-runner-{cloudrun,gcf}` Services):
  - `sockerless-backend-cloudrun@sha256:a221956c` (BUG-958 fix)
  - `sockerless-backend-gcf@sha256:d792e563` (BUG-956 + BUG-957 fix)
- Runner-task images (used by dispatcher to spawn cells 5+6):
  - `runner:cloudrun-amd64@sha256:2b4efebf`
  - `runner:gcf-amd64@sha256:b3b9a9de`
- gcf bootstrap binary baked into the runner-task images carries the BUG-957 persist module + BUG-947 tar-pack flow.

## Single-line summary

> Cells 7+8 GREEN. Cells 5+6 need PR #124 push to trigger `pull_request` workflows (workflow_dispatch is gated on main); runner images already pushed. Live project `sockerless-live-46x3zg4imo` is still up.
