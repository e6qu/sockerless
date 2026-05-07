# Do Next

Resume pointer for next session. State: [STATUS.md](STATUS.md) · Bugs: [BUGS.md](BUGS.md) · Narrative: [WHAT_WE_DID.md](WHAT_WE_DID.md) · Roadmap: [PLAN.md](PLAN.md) · Architecture: [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Today's outcome (2026-05-07)

**Phase 123 fully shipped (all 8 steps), then drilled through 4 follow-up bugs. Cells 5+6 v13 still failing at the Cloud-Run-multi-container startup probe — local repro of the exact overlay binds 8080 in <5s, so the issue is Cloud-Run-specific and needs runtime debugging the autonomous loop can't perform.**

Bugs surfaced + fixed today (in order):
- **BUG-964** (commit `48d5b37`): gcf `invokePodServiceMain` gained `skipIfNoStdin` param so OpenStdin=false runner-pattern containers don't default-invoke and hang. Mirror of cloudrun BUG-961.
- **BUG-966** (commit `2591964`): drop `WorkingDir` from JOB pod-Service container spec — Cloud Run validates the dir exists at startup, but under gcs-sync the workspace is empty until the bootstrap restores. Bootstrap chdir's per-exec via `envelope.Workdir` instead.
- **BUG-967** (commits `e286ba8` → `05c0ecd` → `3ca6614`): three iterations to land the right design. Final shape: `SOCKERLESS_SYNC_MOUNTS=name=mountpath` injected at materialize time on the JOB main container; `SOCKERLESS_SYNC_VOLUMES=name=gs://bucket/object` per-exec via envelope; bootstrap joins by name. Materializer looks up SharedVolume by name (binds are friendly-named at ContainerCreate), not by source path.
- **BUG-968** (commit `4dc8cdc`): cloudrun's `OverlayContentTag` never included `BootstrapBinaryHash` (gcf already did) — the AR cache hit forever, deploying yesterday's bootstrap inside fresh containers. Fix: hash the binary at server startup, stamp into every `OverlayImageSpec`.
- **BUG-969** (commit `d20cb38`): cloudrun default `mapCPUMemory` was `512Mi`/container — too small for multi-container revisions with a postgres sidecar (initdb OOMs). Bumped to `1Gi`/container to match gcf default.

**Still failing (BUG-970 — TO INVESTIGATE)**: cells 5+6 v13 fail at "container failed to start and listen on the port defined provided by the PORT=8080 environment variable within the allocated timeout" inside Cloud Run multi-container revisions. Diagnostic facts:
  - The exact overlay images (gcf-ae32ddee4c8fb839 main, gcf-e443776f095203b4 postgres) bind 8080 in <5s when run locally with the same env Cloud Run sets.
  - The previous v10 cell 6 service (sockerless-svc-abc6788d98b4) is still running and was created with NEAR-IDENTICAL spec — same backend (gcf), same memory (1Gi/container), same number of containers (2), same SA, same annotations. The ONLY difference is v10 didn't have `SOCKERLESS_SYNC_MOUNTS` env (because the materializer fix landed after v10). v10's bootstrap binary is the same as v13's.
  - No bootstrap stderr ever reaches Cloud Logging — only `Starting new instance` system events.
  - All my fixes are in BOTH the runner image (verified by `sha256sum` inside the pulled image) AND the deployed overlay (verified by `docker run` locally).
  - The failed pod-Service spec confirms `SOCKERLESS_SYNC_MOUNTS=runner-externals=/__e,runner-workspace=/__w` is set on the main container's env.

  **Hypotheses to test**:
  1. Cloud Run multi-container revision startup probe behavior changed (or has a quota / region issue we're hitting). Check Cloud Run release notes / status page for us-central1.
  2. Some non-deterministic resource contention — try with `minInstanceCount=0` (lazy start) instead of `1` to see if the cold-start path differs.
  3. Add a `console.log("BOOTSTRAP MAIN ENTRY", os.Args)` at the very top of bootstrap main() and rebuild — confirms whether the binary is even being invoked.
  4. Check if Cloud Run startup probe is hitting POSTGRES port 5432 instead of MAIN port 8080 by misconfiguration. The audit-log spec showed main has `ports: [{containerPort: 8080}]` and postgres `ports: []` — but verify with `gcloud run revisions describe`.

Linux/amd64 binaries are staged. Runner manifests for cloudrun + gcf are at the latest digests. Dispatcher service is on the latest revision (00023-wvr) with `runner_workspace_backing=gcs-sync` on both labels. All architecture is in place; the remaining work is debugging that one Cloud-Run-side startup behavior.

What landed today:
- **Step 1+2** (already in branch from prior session): `core.StorageBackingRegistry` with no-fallback `Resolve(StorageBacking)` + GCSFuse and GCSSync drivers in `gcp-common`.
- **Step 3+4** (commit `997de12`): cloudrun + gcf volume translators that route SharedVolume → BackingDriver → runpb.Volume; `preExecHintsForVolumes` + `postExecForVolumes` wired into both backends' execStartViaInvoke.
- **Step 5** (commit `19d09fe`): bootstrap-side `persist_sync.go` in cloudrun + gcf bootstraps. Reads `SOCKERLESS_SYNC_VOLUMES` from envelope.Env, restores tar.gz before subprocess, saves tar.gz after. Driver interface widened to `map[string][]string` so multi-volume hints don't clobber.
- **Step 7** (commit `48d5b37`): BUG-964 fix — gcf `invokePodServiceMain` gained `skipIfNoStdin` param, skips default-invoke for `OpenStdin=false` JOB containers (mirrors cloudrun BUG-961 fix).
- **Cross-compile verified locally**: linux/amd64 binaries staged at `tests/runners/github/dockerfile-{cloudrun,gcf}/sockerless-{backend,bootstrap}-amd64` — ready for `make build-amd64 push-amd64 manifest`.

What remains: **step 8 only** — push backend + runner + dispatcher images to AR, redeploy services, retrigger cells 5+6 v7.

> **6/8 cells GREEN**. Cells 5+6 (GH × GCP) drilled through 8 architectural blockers in prior sessions — last attempt v6 reached deep into the actual workflow before two final layered failures (BUG-964 gcf default-invoke hang, BUG-965 GCSFuse stale-handle on `event.json`). BUG-964 fixed in code today; BUG-965 superseded by gcs-sync data plane (Phase 123).

**User directives 2026-05-07** that re-shape the next session:
1. Storage MUST be pluggable — implement the storage backing driver abstraction (Phase 123) so we can swap options without re-refactoring.
2. Zero-scaling, no-cost-when-not-in-use is absolute. NFS / Filestore / Memorystore / persistent-mode PDs all rejected.
3. Exploration priority: object storage → in-memory → ephemeral managed FS (only if sockerless owns the lifecycle).
4. No FUSE-on-object-store for new SharedVolumes (GCSFuse retained only for cells 7+8's legacy tar-pack persist).

Branch `phase-118-faas-pods @ fbd3d2b` is pushed. Live infra in `sockerless-live-46x3zg4imo` is up.

## Resume sequence — Phase 123 (storage backing driver abstraction)

This is the biggest architectural lift this session — ~1300 LOC + tests, ~2 days of focused work. Sequencing chosen so each step is independently testable.

### Step 1 — API + core types (foundation, ~200 LOC)

```go
// api/storage_backing.go (NEW)
type StorageBacking string

const (
    BackingEmptyDir StorageBacking = "emptyDir"
    BackingGCSSync  StorageBacking = "gcs-sync"
    BackingGCSFuse  StorageBacking = "gcs-fuse"  // legacy; cells 7+8 only
)

// SharedVolume gains:
type SharedVolume struct {
    Name          string
    ContainerPath string
    Backing       StorageBacking
    GCSBucket     string  // for gcs-sync + gcs-fuse
}
```

```go
// backends/core/storage_backing.go (REPLACES storage_driver.go)
type StorageBackingDriver interface {
    Backing() api.StorageBacking
    CloudSpec(vol api.SharedVolume) (BackingSpec, error)
    PreExec(ctx context.Context, vol api.SharedVolume, execID, localPath string) (envHints map[string]string, err error)
    PostExec(ctx context.Context, vol api.SharedVolume, execID, localPath string) error
}

type BackingSpec struct {
    Kind     api.StorageBacking
    EmptyDir *EmptyDirSpec
    GCS      *GCSSpec  // bucket + mount opts; only used by gcs-fuse driver
}

type StorageBackingRegistry struct {
    drivers map[api.StorageBacking]StorageBackingDriver
}

func (r *Registry) Resolve(b api.StorageBacking) StorageBackingDriver {
    if d, ok := r.drivers[b]; ok { return d }
    return r.drivers[api.BackingEmptyDir]
}
```

Plus `EmptyDirDriver` (returns emptyDir spec, no-op hooks). Delete `core/storage_driver.go` and `api/drivers.go::VolumeDriver` while you're here — both vestigial, no real callers.

### Step 2 — GCP drivers (~400 LOC)

```go
// backends/gcp-common/storage_gcsfuse.go
// Spec = direct GCS bucket mount via Cloud Run native Volume{Gcs}.
// PreExec/PostExec = no-op (FUSE handles it live).
// Used by cells 7+8 with their separate persist module on top.
```

```go
// backends/gcp-common/storage_gcssync.go
// Spec = emptyDir tmpfs.
// PreExec = tar(localPath) → upload to gs://<bucket>/<execID>.tar.gz; return env hint
//          {SOCKERLESS_WORKSPACE_OBJECT: "gs://<bucket>/<execID>.tar.gz",
//           SOCKERLESS_WORKSPACE_PATH: localPath}
// PostExec = download gs://<bucket>/<execID>.tar.gz; untar → localPath; delete object
// Reuse persist.go's gcsGet/gcsPut/tarFrom/untarInto helpers.
```

### Step 3 — Per-backend volume translator (~160 LOC)

```go
// backends/cloudrun-functions/volume_translator.go
func (s *Server) cloudRunVolumeFromBacking(name string, vol api.SharedVolume) (*runpb.Volume, error) {
    spec, err := s.StorageBackings.Resolve(vol.Backing).CloudSpec(vol)
    if err != nil { return nil, err }
    switch spec.Kind {
    case api.BackingEmptyDir, api.BackingGCSSync:
        return &runpb.Volume{Name: name, VolumeType: &runpb.Volume_EmptyDir{...}}, nil
    case api.BackingGCSFuse:
        return &runpb.Volume{Name: name, VolumeType: &runpb.Volume_Gcs{Gcs: &runpb.GCSVolumeSource{Bucket: spec.GCS.Bucket, MountOptions: spec.GCS.MountOptions}}}, nil
    }
    return nil, fmt.Errorf("unsupported backing %q", spec.Kind)
}
```

Same shape in `backends/cloudrun/volume_translator.go`. Replace inline literals at:
- `backends/cloudrun/volumes.go:74` (`runpb.Volume_Gcs{}`)
- `backends/cloudrun-functions/pod_service.go:587` (`runpb.Volume_Gcs{}`)
- `backends/cloudrun-functions/pod_service.go::buildVolumeForBindGCF` (existing emptyDir + tar-pack persist path — keep behavior identical, just route through translator)

### Step 4 — Backend ExecStart wrapper (~200 LOC)

In both cloudrun and gcf, wrap `s.ExecStart` (or `execStartViaInvoke`) so it:
1. Resolves all `SharedVolume` entries on the container.
2. For each: calls `driver.PreExec(ctx, vol, execID, vol.ContainerPath)` → collects returned env hints.
3. Merges hints into the envelope's `Env` field.
4. Forwards the envelope POST to the bootstrap.
5. On response: calls `driver.PostExec(ctx, vol, execID, vol.ContainerPath)` for each volume to pull changes back.

For cells 5+6 specifically: the runner-task's sockerless-backend is the one that does PreExec/PostExec. The "localPath" on PreExec is `/tmp/runner-work` (the runner-task's local emptyDir mount of the SharedVolume). PostExec untars the response back to the same path so the runner sees changes.

### Step 5 — Bootstrap envelope handler (~160 LOC, both bootstraps)

```go
// agent/cmd/sockerless-{cloudrun,gcf}-bootstrap/main.go::runExecEnvelope
// At top:
if obj := envHint(env, "SOCKERLESS_WORKSPACE_OBJECT"); obj != "" {
    path := envHint(env, "SOCKERLESS_WORKSPACE_PATH")
    if err := persist.RestoreFromGCSObject(ctx, obj, path); err != nil {
        return failure(...)
    }
    defer func() {
        // After subprocess returns, save back to the same object.
        // The backend's PostExec downloads + deletes after picking up changes.
        if err := persist.SaveToGCSObject(ctx, obj, path); err != nil {
            log error but don't fail — subprocess already ran
        }
    }()
}
// ... existing subprocess run ...
```

Lift `gcsGet/gcsPut/tarFrom/untarInto` from `persist.go` into a small `persist.RestoreFromGCSObject(obj, path)` / `persist.SaveToGCSObject(obj, path)` API. The existing `restoreAll/saveAll` keep using it for the BUG-947 persist module (cells 7+8 path) — same primitives, different orchestration.

### Step 6 — Dispatcher TOML schema (~60 LOC)

```toml
[[label]]
name                       = "sockerless-cloudrun"
gcp_project                = "..."
gcp_region                 = "..."
image                      = "...:cloudrun-amd64"
service_account            = "...iam.gserviceaccount.com"
runner_workspace_backing   = "gcs-sync"   # or "emptyDir" / "gcs-fuse"
runner_workspace_bucket    = "..."        # for gcs-sync / gcs-fuse
```

`spawner.go` reads these and sets env on the spawned runner-task: `SOCKERLESS_GCP_SHARED_VOLUMES` becomes `name=path=bucket=backing` (4-tuple, was 3-tuple). The bootstrap.sh in the runner image already exports this env to sockerless-backend; backend parses and uses to populate `SharedVolume.Backing` per entry.

### Step 7 — Co-shipped fix BUG-964 (gcf invokePodServiceMain skip-default-invoke)

```go
// backends/cloudrun-functions/pod_service.go::invokePodServiceMain
// Add right after the captured-stdin path:
if len(capturedStdin) == 0 && !mainContainer.Config.OpenStdin {
    s.Logger.Info().Str("main", mainID).Msg("invokePodServiceMain: no stdin + OpenStdin=false (GH actions/runner) — skipping default-invoke")
    return
}
```

This unblocks cell 6 independent of Phase 123 — they can land in either order.

### Step 8 — Build, push, redeploy, retrigger (ONLY remaining step — live-cloud)

linux/amd64 binaries are already staged from this session's local build. To complete the step run from the repo root:

```bash
# 1. GH runner images: build + push the multi-arch manifest list.
make -C tests/runners/github/dockerfile-gcf manifest
make -C tests/runners/github/dockerfile-cloudrun manifest

# 2. Standalone backend images (sidecar in dispatcher / Cloud Run JOB).
#    Stages binary first, builds image, pushes.
make -C tests/runners/dockerfile-sockerless-backend cloudrun-push
make -C tests/runners/dockerfile-sockerless-backend gcf-push

# 3. Dispatcher (github-runner-dispatcher-gcp) — Cloud Build to AR + redeploy.
gcloud builds submit --project=sockerless-live-46x3zg4imo \
  --config=github-runner-dispatcher-gcp/cloudbuild.yaml .

# 4. Cleanup any stale runner-tasks, then push PR #124 to retrigger cells 5+6 v7.
gcloud run jobs list --project=sockerless-live-46x3zg4imo --region=us-central1 \
  --filter="metadata.labels.sockerless_owner=runner-task" \
  --format="value(metadata.name)" | xargs -I{} gcloud run jobs delete {} --quiet
git push origin phase-118-faas-pods
```

What to watch for in the cell logs:
- bootstrap startup line `parsed N sync volumes from SOCKERLESS_SYNC_VOLUMES` (proves env hint flow).
- bootstrap restore line `sync restore <name>: NN bytes -> /tmp/runner-work` per exec.
- cell 5 should now progress past `clone-and-compile` (BUG-965 GCSFuse stale handle is gone — pure GCS SDK no FUSE).
- cell 6 should now progress past the 10-min hang (BUG-964 fix lets the bootstrap stay listening for /exec POSTs).

### Step 9 — Closeout

After cells 5+6 GREEN:
1. Update STATUS.md cell scoreboard (mark 5+6 ✅).
2. Update WHAT_WE_DID.md.
3. Update PR #123 description with all 8 cells GREEN.
4. State save commit. NEVER MERGE — user handles merges.

## Reference: this session's commits

| Commit | Fix |
|--------|-----|
| `17a688b` | feat(phase-123): storage backing driver abstraction + GCS drivers (steps 1+2) |
| `997de12` | feat(phase-123): wire storage backing translators + ExecStart hooks (steps 3+4) |
| `19d09fe` | feat(phase-123): bootstrap-side gcs-sync envelope handler + multi-volume hints (step 5) |
| `48d5b37` | fix(BUG-964): gcf invokePodServiceMain skipIfNoStdin (mirror of BUG-961) (step 7) |

(Prior session commits b223ecb, e97399c, 2ba02f5, e8a85e6, 33e205a, c01067b, d187cc2, fbd3d2b in branch history.)

## Single-line summary

> Phase 123 steps 1-5 + 7 IN-CODE complete (lint clean, tests pass, linux/amd64 binaries cross-compile clean). One step left: push images + retrigger cells 5+6 v7. Expected to close cells 5+6 GREEN — closes the 8/8 milestone.
