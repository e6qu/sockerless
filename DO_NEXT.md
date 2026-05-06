# Do Next

Resume pointer for next session. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-06 — cell 8 4/5 stages GREEN; v26 BUG-956 fix is the path)

User goal: **all 4 GCP cells (5, 6, 7, 8) GREEN with full workflow + evidence + executing where they're supposed to**. Today's session shipped 12 architectural fixes (see STATUS.md table). Cell 8 reached step_script (4/5 stages GREEN). Final blocker is BUG-956.

### Step 1 — Cell 8 v26: BUG-956 architectural fix

**The problem**: gitlab-runner v17 docker executor spawns a NEW build container with `image: golang:1.22-alpine` for step_script (different from the helper image used for prep/get_sources). Both join the same `FF_NETWORK_PER_BUILD=true` network. Our `shouldDeferOrMaterializeNetworkPod` triggers a fresh `materializePodService` with 3 members (new build + postgres + OLD build still in PendingCreates) — chaos.

**The fix** (recommended path; minimal change):

```go
// backends/cloudrun-functions/network_pod.go::pendingMembersOfNetwork
// Exclude containers already materialized in an existing pod-Service.
// gitlab-runner v17 spawns NEW containers per stage with different
// images (helper for prep/get_sources, user image for step_script).
// Each one joining the same FF_NETWORK_PER_BUILD=true network must NOT
// re-trigger materialize while the OLD build container is still in
// PendingCreates.
func (s *Server) pendingMembersOfNetwork(netID, excludeID string) []api.Container {
    var out []api.Container
    for _, pc := range s.PendingCreates.List() {
        if pc.ID == excludeID {
            continue
        }
        mid, ok := s.userDefinedNetworkID(pc)
        if !ok || mid != netID {
            continue
        }
        // Skip if already running in an existing pod-Service.
        if state, ok := s.resolveGCFFromCloud(s.ctx(), pc.ID); ok && state.FunctionURL != "" {
            continue
        }
        out = append(out, pc)
    }
    return out
}
```

**Build + deploy + trigger**:

```bash
# build
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -C backends/cloudrun-functions \
  -tags noui -o tests/runners/dockerfile-sockerless-backend/sockerless-backend-gcf \
  ./cmd/sockerless-backend-gcf

podman build --platform=linux/amd64 \
  -t us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/sockerless-backend-gcf:latest \
  -f tests/runners/dockerfile-sockerless-backend/Dockerfile.gcf \
  tests/runners/dockerfile-sockerless-backend

gcloud auth print-access-token | podman login -u oauth2accesstoken --password-stdin us-central1-docker.pkg.dev
podman push us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/sockerless-backend-gcf:latest

DIGEST=$(gcloud artifacts docker images describe \
  us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/sockerless-backend-gcf:latest \
  --format='value(image_summary.digest)')
sed -i '' "s|sockerless-backend-gcf@sha256:[a-f0-9]\{64\}|sockerless-backend-gcf@$DIGEST|" \
  terraform/cloud-run/gitlab-runner-gcf.yaml

gcloud run services replace terraform/cloud-run/gitlab-runner-gcf.yaml \
  --project=sockerless-live-46x3zg4imo --region=us-central1

# Aggressive quota cleanup BEFORE the test (per user directive — be aggressive)
for svc in $(gcloud run services list --project=sockerless-live-46x3zg4imo --region=us-central1 \
    --format='value(metadata.name)' --filter="metadata.name~^(sockerless-svc-|skls-)"); do
  gcloud run services delete "$svc" --project=sockerless-live-46x3zg4imo --region=us-central1 --quiet
done
# Prune old revisions on long-lived Services
for svc in gitlab-runner-gcf gitlab-runner-cloudrun github-runner-dispatcher-gcp; do
  ACTIVE=$(gcloud run services describe "$svc" --project=sockerless-live-46x3zg4imo --region=us-central1 \
    --format='value(status.latestReadyRevisionName)')
  for rev in $(gcloud run revisions list --service="$svc" --project=sockerless-live-46x3zg4imo --region=us-central1 \
      --format='value(metadata.name)'); do
    if [ "$rev" != "$ACTIVE" ]; then
      gcloud run revisions delete "$rev" --project=sockerless-live-46x3zg4imo --region=us-central1 --quiet
    fi
  done
done

# trigger
git fetch origin-gitlab gitlab-cell-8-test
git worktree add -B gitlab-cell-8-test /tmp/cell8 origin-gitlab/gitlab-cell-8-test
cd /tmp/cell8/ui && bun install
git -C /tmp/cell8 checkout -- ui/bun.lock
sed -i '' "1s/.*/# Cell 8 v26 - BUG-956 filter materialized members from pendingMembersOfNetwork/" /tmp/cell8/.gitlab-ci.yml
git -C /tmp/cell8 add .gitlab-ci.yml
git -C /tmp/cell8 commit -m "trigger: cell 8 v26"
git -C /tmp/cell8 push origin-gitlab gitlab-cell-8-test
git -C /Users/zardoz/projects/sockerless worktree remove /tmp/cell8 --force
git -C /Users/zardoz/projects/sockerless branch -D gitlab-cell-8-test
```

**Watch for**: trace progressing past `step_script` to actually run the cell-8 workflow (probe + git checkout + apk add + go build eval-arithmetic + 5 arithmetic markers).

### Step 2 — Cell 7 re-validation (if needed)

Cell 7 was GREEN at digest `f786c300` this morning. Today's v52 retest hit a transient regional CPU quota error (8s-fail with `FailedPrecondition: Quota exceeded`). Today's deployed cloudrun digest is `sha256:db43b1ec` which adds HTTP middleware Info-level logging + AR HEAD precheck. Re-trigger cell 7 to confirm those didn't regress:

```bash
git fetch origin-gitlab gitlab-cell-7-test
git worktree add -B gitlab-cell-7-test /tmp/cell7 origin-gitlab/gitlab-cell-7-test
cd /tmp/cell7/ui && bun install
git -C /tmp/cell7 checkout -- ui/bun.lock
sed -i '' "1s/.*/# Cell 7 v53 - re-validate after today's gcp-common changes/" /tmp/cell7/.gitlab-ci.yml
git -C /tmp/cell7 add .gitlab-ci.yml
git -C /tmp/cell7 commit -m "trigger: cell 7 v53"
git -C /tmp/cell7 push origin-gitlab gitlab-cell-7-test
git -C /Users/zardoz/projects/sockerless worktree remove /tmp/cell7 --force
git -C /Users/zardoz/projects/sockerless branch -D gitlab-cell-7-test
```

### Step 3 — Cells 5+6

Per user directive 2026-05-05: **dispatcher stays generic; sockerless+runner pairing lives in the runner image**. The runner-task images at `tests/runners/github/dockerfile-{cloudrun,gcf}/` already bundle vanilla actions/runner + sockerless-backend-{cloudrun,gcf} with the bootstrap that launches both.

```bash
# 1. Rebuild runner-task images with today's sockerless-backend bits
make -C tests/runners/github/dockerfile-cloudrun push-amd64
make -C tests/runners/github/dockerfile-gcf push-amd64

# 2. Update dispatcher TOML config to point at the new digests
#    (operator-side; lives in github-runner-dispatcher-gcp's mounted config)
#    The TOML's `[[label]]` entries map runs-on label → runner-task image.
#    Format documented in github-runner-dispatcher-gcp/internal/config/config.go.

# 3. Trigger workflows
gh workflow run cell-5-cloudrun.yml
gh workflow run cell-6-gcf.yml

# 4. Watch dispatcher logs
gcloud logging read 'resource.labels.service_name=github-runner-dispatcher-gcp' \
  --project=sockerless-live-46x3zg4imo --limit=50 --order=desc

# 5. Watch the spawned Cloud Run Job execution
gcloud run jobs list --project=sockerless-live-46x3zg4imo --region=us-central1
gcloud run jobs executions list --project=sockerless-live-46x3zg4imo --region=us-central1
```

Cell 5 + 6 each spawns one Cloud Run Job execution per workflow_job. The job's container starts sockerless-backend-{cloudrun,gcf} on `:3375|:3376` in background, then registers the runner with `RUNNER_REG_TOKEN` and runs the workflow. Vanilla actions/runner uses `DOCKER_HOST=tcp://localhost:3375|3376` to dispatch its `container:` directives via sockerless.

### Step 4 — Closeout (after all 4 cells GREEN)

1. Mark BUG-953/954/955/956 closed in BUGS.md
2. State save: STATUS.md / WHAT_WE_DID.md / DO_NEXT.md / PLAN.md / MEMORY
3. Update PR #123 title + description (already drafted in `.git/notes/pr-123-description` if persisted)
4. PR is ready for user review (NEVER MERGE — user handles all merges)

## Single-line summary

> Cell 8 4/5 stages GREEN at digest `sha256:79621fbe`; v26 needs BUG-956 fix in `network_pod.go::pendingMembersOfNetwork` to filter out already-materialized members. Then cell 7 re-validate, then cells 5+6 via `make push-amd64` + dispatcher TOML + `gh workflow run`. All architectural fixes for the gcf network-pod path are in place; no more new mechanisms needed.

## Aggressive cleanup discipline (per user directive 2026-05-06)

When resources aren't needed: delete them. The build script template above includes:
- Delete every `sockerless-svc-*` Service after each test
- Delete every `skls-*` prewarm Function
- Prune old revisions on long-lived Services (keep only the active one)

This keeps regional CPU quota from cascading failures.
