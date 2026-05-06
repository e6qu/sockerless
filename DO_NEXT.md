# Do Next

Resume pointer for next session. State: [STATUS.md](STATUS.md) · Bugs: [BUGS.md](BUGS.md) · Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md) · Roadmap: [PLAN.md](PLAN.md).

## Today's outcome (2026-05-06)

**6/8 cells GREEN.** Cells 5+6 (GH × GCP) drilled through 8 architectural blockers today (BUG-956 → BUG-963). Both reached deep into the actual workflow on v6. **Two final blockers** remain — fix shapes pinned below.

Branch `phase-118-faas-pods @ c01067b` is pushed. Live infra in `sockerless-live-46x3zg4imo` is up.

## Resume sequence

### Step 1 — BUG-964: gcf invokePodServiceMain skip-default-invoke

Cell 6 v6 hit a 10-min HTTP timeout because gcf's `invokePodServiceMain` POSTs the JOB container's long-lived `tail -f /dev/null`-style CMD as a one-shot subprocess (holding `invokeMu` so subsequent `/exec` POSTs queue forever). Same shape as cloudrun BUG-961 (already fixed).

```go
// backends/cloudrun-functions/pod_service.go
// Add to invokePodServiceMain right after the captured-stdin path,
// BEFORE the existing "if len(capturedStdin) > 0" branch:

// BUG-964: GH actions/runner pattern — main is OpenStdin=false with a
// long-lived `tail -f /dev/null`-style entrypoint kept alive for
// `docker exec`. Skip default-invoke (would block forever on the
// long-lived CMD); the bootstrap stays listening on :8080 for /exec.
// Mirror of cloudrun BUG-961 fix in commit c01067b.
if len(capturedStdin) == 0 && !mainContainer.Config.OpenStdin {
    s.Logger.Info().Str("main", mainID).Msg("invokePodServiceMain: no captured stdin + OpenStdin=false (GH actions/runner) — skipping default-invoke")
    return
}
```

`invokePodServiceMain` already has an OpenStdin=true / no-stdin "skip" branch — this adds the OpenStdin=false / no-stdin "skip" branch. Then rebuild + push gcf-backend image + GH gcf runner image:

```bash
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -C backends/cloudrun-functions \
  -tags noui -o tests/runners/dockerfile-sockerless-backend/sockerless-backend-gcf \
  ./cmd/sockerless-backend-gcf
GOOS=linux GOARCH=amd64 CGO_ENABLED=0 go build -C agent/cmd/sockerless-gcf-bootstrap \
  -o tests/runners/dockerfile-sockerless-backend/sockerless-gcf-bootstrap .
podman build --platform=linux/amd64 \
  -t us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/sockerless-backend-gcf:latest \
  -f tests/runners/dockerfile-sockerless-backend/Dockerfile.gcf \
  tests/runners/dockerfile-sockerless-backend
gcloud auth print-access-token | podman login -u oauth2accesstoken --password-stdin us-central1-docker.pkg.dev
podman push us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/sockerless-backend-gcf:latest
make -C tests/runners/github/dockerfile-gcf push-amd64

DIGEST=$(gcloud artifacts docker images describe \
  us-central1-docker.pkg.dev/sockerless-live-46x3zg4imo/sockerless-live/sockerless-backend-gcf:latest \
  --format='value(image_summary.digest)')
sed -i '' "s|sockerless-backend-gcf@sha256:[a-f0-9]\{64\}|sockerless-backend-gcf@$DIGEST|" \
  terraform/cloud-run/gitlab-runner-gcf.yaml
gcloud run services replace terraform/cloud-run/gitlab-runner-gcf.yaml \
  --project=sockerless-live-46x3zg4imo --region=us-central1
```

### Step 2 — BUG-965: replace GCSFuse with Cloud Filestore (NFSv3)

**User directive 2026-05-06: no GCSFuse.** Cell 5 v6 hit `Stale file handle: event.json` because GCSFuse invalidates open handles on object rewrite. Replace with Cloud Run native `Volume{Nfs{Server,Path}}` backed by Cloud Filestore — full POSIX, no stale handles, no eventual consistency.

Implementation steps:

```bash
# 1. Provision Filestore (one-time terragrunt apply)
# terraform/environments/runner/live/main.tf — add:
#   resource "google_filestore_instance" "runner_workspace" {
#     name     = "sockerless-runner-workspace"
#     tier     = "BASIC_HDD"
#     location = "us-central1-a"
#     file_shares { name = "workspace"; capacity_gb = 1024 }
#     networks { network = "default"; modes = ["MODE_IPV4"] }
#   }
# Note: BASIC_HDD minimum 1 TiB ≈ $160/mo. Operator-accepted.
```

```go
// 2. github-runner-dispatcher-gcp/internal/config/config.go
// REPLACE RunnerWorkspaceBucket with:
RunnerWorkspaceNfsServer string `toml:"runner_workspace_nfs_server"`
RunnerWorkspaceNfsPath   string `toml:"runner_workspace_nfs_path"`

// 3. github-runner-dispatcher-gcp/internal/spawner/spawner.go
// In the BUG-963 block, REPLACE Volume_Gcs with:
VolumeType: &runpb.Volume_Nfs{
    Nfs: &runpb.NFSVolumeSource{
        Server:   req.RunnerWorkspaceNfsServer,
        Path:     req.RunnerWorkspaceNfsPath,
        ReadOnly: false,
    },
},
```

```go
// 4. backends/{cloudrun,cloudrun-functions}/{jobspec,pod_service}.go
// SharedVolume struct gains NfsServer + NfsPath fields. When set,
// buildVolumeFor* emits Volume{Nfs} instead of Volume{Gcs}. The
// runner-task and JOB pod-Service then mount the SAME Filestore
// share — script files propagate atomically.
```

Update dispatcher TOML:
```toml
[[label]]
name                       = "sockerless-cloudrun"
...
runner_workspace_nfs_server = "10.x.y.z"   # Filestore reserved IP from terraform
runner_workspace_nfs_path   = "/workspace"
```

Rebuild + push dispatcher (`gcloud builds submit --config=github-runner-dispatcher-gcp/cloudbuild.yaml .`); update Service. Rebuild + push runner-task images.

### Step 3 — Cleanup + re-trigger cells 5+6 v7

```bash
# Cleanup
for svc in $(gcloud run services list --project=sockerless-live-46x3zg4imo --region=us-central1 \
    --format='value(metadata.name)' | grep -E '^(sockerless-svc-|skls-)'); do
  gcloud run services delete "$svc" --project=sockerless-live-46x3zg4imo --region=us-central1 --quiet
done

# Trigger v7
git fetch origin cell-workflows-on-main
git worktree add -B pr124 /tmp/pr124 origin/cell-workflows-on-main
cd /tmp/pr124/ui && bun install && cd /Users/zardoz/projects/sockerless
git -C /tmp/pr124 checkout -- ui/bun.lock
sed -i '' "1s/.*/# Cell 5 v7 — BUG-964+965/" /tmp/pr124/.github/workflows/cell-5-cloudrun.yml
sed -i '' "1s/.*/# Cell 6 v7 — BUG-964+965/" /tmp/pr124/.github/workflows/cell-6-gcf.yml
git -C /tmp/pr124 add .github/workflows/cell-5-cloudrun.yml .github/workflows/cell-6-gcf.yml
git -C /tmp/pr124 commit -m "trigger: cells 5+6 v7"
git -C /tmp/pr124 push origin pr124:cell-workflows-on-main
git -C /Users/zardoz/projects/sockerless worktree remove /tmp/pr124 --force
git -C /Users/zardoz/projects/sockerless branch -D pr124
```

### Step 4 — Closeout

After cells 5+6 GREEN: update STATUS.md, WHAT_WE_DID.md, PR #123 description. NEVER MERGE — user handles merges.

## Reference: today's commits

| Commit | Fix |
|--------|-----|
| `b223ecb` | BUG-956 (multi-image-per-stage materialize) + BUG-957 (gcf bootstrap persist + content-hash overlay tag) |
| `e97399c` | BUG-958 (cloudrun multi-stage runner-pattern) |
| `2ba02f5` | BUG-959 (GH actions/runner materialize on second-arrival) |
| `e8a85e6` | BUG-960 (Typed.Exec routes through s.ExecStart) |
| `33e205a` | BUG-961 (cloudrun skip-default-invoke) + BUG-962 (stdcopy framing) |
| `c01067b` | BUG-963 (dispatcher GCS workspace mount) |

## Single-line summary

> 6/8 cells GREEN. Cells 5+6 v6: cell 5 deep into `clone-and-compile` (BUG-965 GCSFuse stale-handle), cell 6 hit 10-min default-invoke hang (BUG-964: mirror BUG-961 to gcf). Both fixes pinned in this file. Live project still up.
