# Do Next

Resume pointer for next session. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-06 — cell 8 GREEN; pause for GH Actions incident)

User goal: **all 4 GCP cells (5, 6, 7, 8) GREEN with full workflow + evidence + executing where they're supposed to**.

**Today's outcome**: cell 8 v28 GREEN (job 14234857458, 147s, all 5 arithmetic checks pass). 14 architectural fixes shipped — see STATUS.md table. BUG-956 + BUG-957 closed in commit `b223ecb`.

**Pause reason**: GitHub Actions ongoing incident — cells 5+6 dispatches deferred. User said "pause for 1 hour" 2026-05-06 ~08:48 UTC.

### When resuming

#### Step 1 — Confirm GH Actions is healthy

```bash
gh api /repos/e6qu/sockerless/actions/workflows --jq '.workflows[] | "\(.name): \(.state)"' | head
# If healthy proceed to step 2/3.
```

#### Step 2 — Cell 7 re-validation (in-flight at pause time)

Cell 7 v53 was running but bytesize stuck at 1533 since ~08:30 UTC (job 14235026887). Likely hung; cancel and re-trigger fresh.

```bash
# Check if v53 finished while paused
gcloud logging read 'resource.type=cloud_run_revision AND resource.labels.service_name=gitlab-runner-cloudrun AND timestamp>="2026-05-06T08:30:00Z" AND textPayload=~"Job succeeded|Job failed|aborted: terminated"' \
  --project=sockerless-live-46x3zg4imo --limit=5 --order=desc --format='value(timestamp,textPayload)'

# If still stuck or failed, fresh trigger:
git fetch origin-gitlab gitlab-cell-7-test
git worktree add -B gitlab-cell-7-test /tmp/cell7 origin-gitlab/gitlab-cell-7-test
cd /tmp/cell7/ui && bun install && cd /Users/zardoz/projects/sockerless
git -C /tmp/cell7 checkout -- ui/bun.lock
sed -i '' "1s/.*/# Cell 7 v54 - re-validate cloudrun GREEN after BUG-956+957/" /tmp/cell7/.gitlab-ci.yml
git -C /tmp/cell7 add .gitlab-ci.yml && git -C /tmp/cell7 commit -m "trigger: cell 7 v54"
git -C /tmp/cell7 push origin-gitlab gitlab-cell-7-test
git -C /Users/zardoz/projects/sockerless worktree remove /tmp/cell7 --force
git -C /Users/zardoz/projects/sockerless branch -D gitlab-cell-7-test
```

#### Step 3 — Cells 5 + 6 (GH × cloudrun, GH × gcf)

Runner images already rebuilt + pushed today with the BUG-957 persist module:
- `runner:gcf-amd64@sha256:b3b9a9de57b9c964697220dd4a4e51d7bd9c2ab3e79e0ab8abc0568d170772e2`
- `runner:cloudrun-amd64@sha256:2b4efebf155ed97986ae7b7bb1e0ea3e126d7623385ff63f211d999223baef17`

Dispatcher uses tag references (no digest pin) so the next spawn will pull these new digests automatically. Just trigger workflows:

```bash
gh workflow run cell-5-cloudrun.yml --repo e6qu/sockerless
gh workflow run cell-6-gcf.yml --repo e6qu/sockerless

# Watch dispatcher logs for spawn
gcloud logging read 'resource.labels.service_name=github-runner-dispatcher-gcp AND timestamp>="'"$(date -u -v-5M +%Y-%m-%dT%H:%M:%SZ)"'"' \
  --project=sockerless-live-46x3zg4imo --limit=30 --order=desc --format='value(timestamp,textPayload,jsonPayload.message)'

# Watch the spawned Cloud Run Jobs
gcloud run jobs executions list --project=sockerless-live-46x3zg4imo --region=us-central1
```

Each cell spawns one Cloud Run Job execution per workflow_job. The job's container starts sockerless-backend-{cloudrun,gcf} on `:3375|:3376` in background, then registers the runner with `RUNNER_REG_TOKEN` and runs the workflow. Vanilla actions/runner uses `DOCKER_HOST=tcp://localhost:3375|3376` to dispatch its `container:` directives via sockerless.

#### Step 4 — Closeout (after all 4 cells GREEN)

1. Update STATUS.md cell scoreboard
2. State save: STATUS.md / WHAT_WE_DID.md / DO_NEXT.md / PLAN.md / MEMORY
3. Update PR #123 title + description
4. PR is ready for user review (NEVER MERGE — user handles all merges)

## Live infra state (preserved across pause)

`sockerless-live-46x3zg4imo` (us-central1):
- `gitlab-runner-gcf` rev `00060-72h` @ `sha256:d792e563` ← v28 GREEN
- `gitlab-runner-cloudrun` rev `latest` @ `sha256:db43b1ec`
- `github-runner-dispatcher-gcp` rev `00021-fb2`
- VPC connector `sockerless-connector` @ Cloud NAT static IP `34.31.88.230`
- Runner images:
  - `runner:gcf-amd64@sha256:b3b9a9de`
  - `runner:cloudrun-amd64@sha256:2b4efebf`

## Architectural fixes shipped today (commit `b223ecb`)

- **BUG-956**: `pendingMembersOfNetwork` now filters already-materialized OpenStdin=true mains. Sidecars (postgres) stay so each stage's pod-Service revision gets its own postgres copy. Old main stays in its own pod-Service for cleanup_file_variables docker exec.
- **BUG-957**: gcf bootstrap now has `persist.go` (ported from cloudrun bootstrap). `handleInvoke` is wrapped with bufferedResponse + saveAll so /builds tar-pack flushes between stages. New `BootstrapBinaryHash` field on `OverlayImageSpec` so updating the bootstrap binary at the same path invalidates AR overlay caches automatically.

## Single-line summary

> Cell 8 v28 GREEN at `sha256:d792e563`; BUG-956 + BUG-957 closed; pause 1h for GH Actions incident; resume with cell 7 re-trigger then `gh workflow run cell-{5,6}-*.yml`.
