# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-05 v24 — BUG-947 backend volume_emptydir landed; image rebuild pending)

### What just landed (commits on `phase-118-faas-pods`)

| Commit | What |
|---|---|
| `153f95c` | docs: BUG-947 filed (cell 7 v50 hung at git checkout under GCSFuse) |
| `bb420ca` | docs: corrected analysis — Cloud Run revision immutability rules out Path A (emptyDir + per-job Service) |
| `4d7e5d8` | docs: diagnostic — git clone on GCSFuse 211 s vs tmpfs 1 s (200× slower) |
| `1f06831` | feat: persist module added to `agent/cmd/sockerless-cloudrun-bootstrap/` and wired into `main.go` — tar-pack approach |
| `b381612` | docs: state save — Phase 122j BUG-947 tar-pack approach in flight; persist module committed, backend + redeploy pending |
| `f5e52f1` | **feat: backend Volume_EmptyDir + SOCKERLESS_PERSIST_VOLUMES env injection** — `backends/cloudrun/{jobspec,servicespec,volumes}.go` + `backends/cloudrun-functions/volumes.go`. Ad-hoc binds → tmpfs; SharedVolumes keep raw GCSFuse. |
| `29308e1` | **refactor: bootstrap status codes** — 5xx removed from cloudrun + gcf bootstraps; failures signal via 200 + `X-Sockerless-Exit-Code` header (or envelope `exitCode`). Per user directive 2026-05-05 "500 reserved for unexpected panics". |

### Next concrete steps (in order)

Backend code complete; bootstrap code complete. What remains is image rebuild + redeploy + cell trigger:

1. **Rebuild + push images:**
   ```sh
   cd /Users/zardoz/projects/sockerless
   # Bootstrap binary
   make -C agent build-cloudrun-bootstrap
   # Backend images (FROM distroless, embed the bootstrap binary)
   make -C backends/cloudrun docker-image                 # → sockerless-backend-cloudrun
   make -C backends/cloudrun-functions docker-image       # → sockerless-backend-gcf
   # Push to Artifact Registry (or `gcloud builds submit`).
   ```
   Capture the resulting `sha256:` digests — they'll be pinned in step 2.

2. **Update `terraform/cloud-run/gitlab-runner-cloudrun.yaml`:**
   - Bump sockerless sidecar container image digest to the new build (currently pins `sha256:c9716fa8...`).
   - No env additions in the runner sidecar — the backend injects `SOCKERLESS_PERSIST_VOLUMES` automatically on per-step service containers as of `f5e52f1`.
   - `gcloud run services replace terraform/cloud-run/gitlab-runner-cloudrun.yaml --project=sockerless-live-46x3zg4imo --region=us-central1`

3. **Trigger cell 7 v51:**
   - Branch `gitlab-cell-7-test` (push a trigger commit by editing line 1 of the branch's `.gitlab-ci.yml`).
   - Pipeline should reach `Job succeeded` with all 5 arithmetic results (11/14/21/13/6.5).

4. **Cell 8 v_? trigger** once cell 7 is green — same workflow on `gitlab-cell-8-test` branch.

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

> Tar-pack persistence for ad-hoc bind volumes — bootstrap (`1f06831`), backend (`f5e52f1`), and bootstrap status-code refactor (`29308e1`) all committed. Remaining: rebuild + push sockerless-backend images, bump digest in `terraform/cloud-run/gitlab-runner-cloudrun.yaml`, retrigger cell 7 v51.
