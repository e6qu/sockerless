# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-05 v25 — Cell 7 GREEN, cell 8 next)

### What just landed (commits on `phase-118-faas-pods`)

| Commit | What |
|---|---|
| `1f06831` | feat: persist module in cloudrun-bootstrap (tar-pack restoreAll/saveAll) |
| `f5e52f1` | feat: backend Volume_EmptyDir for ad-hoc binds + SOCKERLESS_PERSIST_VOLUMES env injection (cloudrun + gcf) |
| `29308e1` | refactor: bootstrap status codes — 5xx replaced with 200 + X-Sockerless-Exit-Code header (per "500 reserved for panics" user rule) |
| `687cdb8` | deploy: bump sockerless-backend-cloudrun digest → `sha256:f786c300...`; rev `00003-csp` Ready |

**Cell 7 v51 GREEN** — pipeline 2500209956, job 14213994152, 383 s. Heavy workload verified end-to-end: git fetch + git checkout + apk add file + go build eval-arithmetic + run-with-postgres-sidecar. All 5 arithmetic results correct (11/14/21/13/6.5). BUG-947 closed.

### Next concrete steps (in order)

#### Cell 8 (GL × gcf) — same fix pattern, but gitlab-runner-gcf still on OLD architecture

Cell 8's gcf backend already has the BUG-947 fix in code (`f5e52f1` patches both `backends/cloudrun-functions/volumes.go` and the gcf bootstrap), and the new `sockerless-backend-gcf@sha256:4c84a691...` image is already pushed to AR. What's still needed:

1. **Refactor `terraform/cloud-run/gitlab-runner-gcf.yaml`** to the same vanilla 3-container shape as `gitlab-runner-cloudrun.yaml`:
   - init: gitlab-runner-init image (register + write config.toml with DOCKER_HOST=tcp://localhost:3376)
   - gitlab-runner: vanilla `gitlab/gitlab-runner:v17.5.0`, dependsOn init
   - sockerless: `sockerless-backend-gcf@sha256:4c84a691021fed10f6382292df8e8c66cace73a337bc28bcd6b2a89dd6189050`, ports 3376
   - VPC connector + ALL_TRAFFIC egress (same as cloudrun)
2. **Deploy:** `gcloud run services replace terraform/cloud-run/gitlab-runner-gcf.yaml --project=sockerless-live-46x3zg4imo --region=us-central1`
3. **Trigger cell 8** — push a commit to `gitlab-cell-8-test` branch's `.gitlab-ci.yml` line 1; expect arithmetic results.

#### Cells 5+6 (GH × cloudrun, GH × gcf) — pivot to vanilla-runner architecture

Per user directives (and STATUS.md's recorded plan):
- **Cells 5+6 architecture:** pre-deployed Cloud Run **Job** per cell label with multi-container TaskTemplate (vanilla `actions/runner --ephemeral` + sockerless sidecar). Dispatcher's only call = `Executions.RunJob(<predefined-job>)` with per-execution env override (`RUNNER_REG_TOKEN`, `RUNNER_NAME`, `RUNNER_LABELS`, `RUNNER_REPO`).
- The current `github-runner-dispatcher-gcp` rev `00021-fb2` is OLD architecture (custom image baking sockerless) — needs replacement.
- Unmodified vanilla `actions/runner` image required (no sockerless code baked in).

### What was rejected (and why — don't re-propose)

| Path | Rejection reason |
|---|---|
| **Path A — emptyDir + single Cloud Run Service revision per gitlab-runner job** | Cloud Run revisions are immutable; modifying a Service spawns a NEW instance with a fresh `emptyDir`. gitlab-runner adds containers dynamically across stages, so we can't deploy them all in one revision upfront. Architecturally infeasible. |
| **Path B — Cloud Filestore (NFS)** | $160/mo BASIC_HDD floor (1 TiB minimum, even empty). User noted GCP has no pay-per-use NFS equivalent of AWS EFS. Held in reserve as the long-term fix for big-repo workloads where tar-pack roundtrip dominates. |
| **Path C — git config workarounds (`core.useHardlinks=false`, `core.fsync=off`)** | Forbidden per "no quick fixes" project rule — gitlab-runner must work for `GIT_STRATEGY=clone/fetch/none`. |
| **`GIT_STRATEGY=none` in cell 7 yml** | User explicitly rejected: "we still want the gitlab runner to support the GIT_STRATEGY feature for all values of it." |
| **`fuse-overlayfs` (tmpfs upper, gcsfuse lower)** | Cloud Run gen2 may not have the syscall caps; would still need per-file sync to GCS at exit (slow). |
| **LD_PRELOAD shim to fake `link()`/`flock()`/`rename()` syscalls** | Image-specific, fragile, breaks "vanilla runner" rule on observable behavior. |
| **Pre-warm Filestore pool** | Adds quota pressure + 5-15 min provisioning latency per job; hide-the-cold-start complexity not worth it before tar-pack proves insufficient. |

### Why tar-pack works (chosen approach)

GCSFuse's slowness is **per-file metadata round trips**, not raw bandwidth. A single tar object replaces N small-file writes with one upload. Sockerless-repo-sized data (~10 MB) packs/uploads/downloads in ~2-5 sec. For each gitlab-runner stage boundary, that's the entire overhead — total ~15-25 sec across a 5-stage CI job. Same `Volume_Gcs` bucket sockerless already provisions per volume; no new infra; no new auth. Bootstrap binary grows ~0 MB (raw HTTP + metadata-server token, no GCS SDK dep).

User's explicit answer-tier preferences (recorded today, do not re-ask):
- Scope: ad-hoc bind volumes only; SharedVolumes stay raw GCSFuse.
- Boundary: every exec (under `invokeMu`).
- Storage: existing per-volume bucket + single object key `sockerless-volume.tar`.
- Format: plain tar (no compression).
- Multi-container: only **main** container persists; sidecars (`SOCKERLESS_SIDECAR=1`) skip both restore + save.
- Auth: ADC via metadata server.
- Failure: hard-fail save → exec returns 500 → gitlab-runner stage fails cleanly.
- Always-on, no opt-out env. Apply to both cloudrun + gcf backends in one change.

### Architecture context (NEW vanilla-runner pivot, in flight since 2026-05-04 afternoon)

Per user directives:
1. github + gitlab runners stay UNMODIFIED (vanilla upstream images).
2. only acceptable thing for GitHub is the dispatcher; for GitLab no dispatcher (gitlab-runner's docker executor IS the dispatcher).
3. runners talk to sockerless via DOCKER_HOST = `tcp://localhost:3375`/3376; no sockerless code baked into runner images.

**Cells 7 + 8 (gitlab):** pre-deployed multi-container Cloud Run **Service** per cell. Containers: [init: registers fresh runner via gitlab API → writes /shared/config.toml], [gitlab-runner: vanilla `gitlab/gitlab-runner:v17.5.0`], [sockerless: standalone backend image, ingress on :3375]. Live at `gitlab-runner-cloudrun-00002-8l8`.

**Cells 5 + 6 (github):** still TODO. Architecture: pre-deployed Cloud Run Job per cell label with multi-container TaskTemplate (vanilla `actions/runner` + sockerless sidecar); dispatcher's only call = `Executions.RunJob(<predefined-job>)` with per-execution env override (`RUNNER_REG_TOKEN`, `RUNNER_NAME`, `RUNNER_LABELS`, `RUNNER_REPO`).

### Live infra state (`sockerless-live-46x3zg4imo`, us-central1)

| Resource | State | Notes |
|---|---|---|
| `github-runner-dispatcher-gcp` rev `00021-fb2` | OLD architecture (custom image baking sockerless) | will be replaced when cells 5+6 refactor |
| `gitlab-runner-cloudrun` rev `00002-8l8` | NEW architecture, healthy | needs sockerless image bump after persist patch |
| `gitlab-runner-gcf` rev `00027-jkg` | OLD architecture | needs full refactor for cell 8 |
| VPC connector `sockerless-connector` | min-instances 4, max 5, e2-micro | scaled up 2→4 today; fixed git-fetch egress timeout |
| Cloud NAT `sockerless-nat` | static IP `34.31.88.230` | works |
| Filestore | not provisioned | held in reserve per Path B above |
| Stale Cloud Run Job `sockerless-491f3e44a7eb` | leftover from cell 7 v5 | `gcloud run jobs delete` was permission-denied — leave for next session or operator |
| Stale gitlab project_type runners | 2/50 currently — purged 48 today | init script in gitlab-runner-cloudrun re-purges on each revision |

### What carries over from prior work (unchanged + still needed)

- BUG-923 cancellation channel for ContainerCreate→ContainerStart pod materialization (`2b16791`) — step-container scope, unaffected.
- BUG-944 GCS-Fuse MountOptions + idempotent attach (`a7e3b00`) — step-container scope, unaffected.
- BUG-946 integration test build tag (`e733d70`) — unrelated to runner architecture.
- Dispatcher rate-limit/poller fixes (`0f94a53` / `06561dd` / `c6e7dee`) — still correct.
- `SOCKERLESS_GCR_USE_SERVICE=1` + `SOCKERLESS_GCR_VPC_CONNECTOR=projects/.../sockerless-connector` env vars on the sockerless sidecar in `terraform/cloud-run/gitlab-runner-cloudrun.yaml` — required for step-Service path; do not remove.

### Single-line summary

> Cell 7 GREEN under vanilla-runner architecture (BUG-947 closed via tar-pack persist). Next: refactor `gitlab-runner-gcf.yaml` to same shape, redeploy with `sockerless-backend-gcf@sha256:4c84a691...`, trigger cell 8. Then GH dispatcher refactor for cells 5+6.
