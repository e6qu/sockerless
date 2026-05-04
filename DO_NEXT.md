# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-04 v18 — end of Phase 122i session)

**Goal**: cells 5/6/7/8 GREEN with REAL workload (compile + use eval-arithmetic + probe environment) before merging PR #123. Cell 7 was GREEN once. Cells 5/6/8 never have been.

### What's done since v17

Phase 122i shipped 6 dispatcher/backend fixes (commits on `phase-118-faas-pods`):

- `0f94a53` runner-task 4Gi/2cpu (was 512Mi/1cpu OOM'ing the Go compile)
- `06561dd` dispatcher honors rate-limit window across cleanup ticker
- `c6e7dee` poller back-off + per-run seen-set + 60s cadence (eliminates 1+N call burn)
- `71288bf` revert gcf CPU=0.5 — gen2 requires ≥1
- `df75d4d` gcf pool claim retry ~5s — peers release before new deploy

Cell 6 SOLO got past CPU+OOM and deployed services successfully; new failure is exit 126 in the first script step (BUG-944 = GCS-Fuse mount latency).

### Next concrete steps

1. **Verify pool back-off (BUG-942)** — rebuild gcf backend image, deploy gitlab-runner-gcf service with `df75d4d`, trigger cell 8 SOLO. Expect cache-permission-container quartet to reuse one function instead of deploying 4. Trace count of deploys in Cloud Logging to confirm.

2. **Fix BUG-944 GCS-Fuse mount latency for docker exec** — cell 6 SOLO just hit this. Three viable fix candidates:
   - **Option A**: Tune GCS-Fuse mount opts on multi-container Cloud Run Service: `--implicit-dirs --stat-cache-ttl=0 --type-cache-ttl=0`. Touches `backends/cloudrun-functions/volumes.go` (gcf) + `backends/cloudrun/volumes.go` (cloudrun).
   - **Option B**: tmpfs volume for `/__w/_temp` (script delivery path), GCS only for the larger workspace `/__w`. The runner-task mounts both volumes; the multi-container revision shares the tmpfs natively.
   - **Option C**: bootstrap.sh on the runner-task fsync's the script + retries `docker exec` with a short loop until exit != 126.

3. **Fix BUG-929 cell 5 hang** — `cloudrun startSingleContainerService missing post-deploy invoke`. Cell 5 hangs on Initialize containers for 43+ min instead of failing fast like cell 6. Need to wake the cold-start service after deploy via an explicit POST.

4. **Re-trigger cell 7 via .gitlab-ci.yml swap** to confirm regression-free (cell 7 was GREEN at the BUG-925 12-step fix; nothing should have broken it but verify after CPU/pool changes).

5. **Cell 8 SOLO** with refreshed gitlab-runner-gcf image + pool back-off. Same .gitlab-ci.yml swap pattern.

### Test serially, NOT in parallel

Per BUG-942: cells in parallel exceed `cpu_allocation` per-minute rate. Run ONE cell at a time during verification rounds; only after the pool back-off and BUG-944 fixes verify GREEN solo, can we test parallel.

### Strict rules carried forward (from session learnings)

- DO NOT request Cloud Run quota increases — user rule "no fallbacks, fix the actual problem".
- DO NOT lower gcf CPU below 1 — gen2 rejects.
- DO NOT trust GitHub Actions step reporting "completed/success" without verifying workload markers in Cloud Logging (BUG-927 lesson).
- DO NOT add runner-specific labels in backend code (per user 2026-05-03 directive); coupling is docker/podman libpod HTTP API on entry, cloud primitives on backend, period.
- DO NOT push images to public registries (Docker Hub / GitLab Registry); only pull via AR remote-proxy.
- DO NOT cancel cells the agent didn't create — sandbox blocks; ask user.

## Tactical files for resume

- `backends/cloudrun-functions/pool.go` — `claimFreeFunction` is the recently-added back-off entry point.
- `backends/cloudrun-functions/volumes.go` — GCS-Fuse mount config (BUG-944 fix candidate).
- `backends/cloudrun/start_service.go` — `startSingleContainerService` (BUG-929 fix candidate).
- `github-runner-dispatcher-aws/pkg/poller/poller.go` — proactive rate-limit + runSeen.
- `github-runner-dispatcher-gcp/cmd/.../main.go` — `rateLimitedUntil` guard.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

See STATUS.md § "Live infra" for the canonical list. Key points: NAT pinned to `34.31.88.230` (don't auto-rotate), dispatcher rev `00021-fb2`, gitlab-runner-gcf rev `00027-jkg`. Quota preference withdrawn — don't re-create.

## Branch state

- `phase-118-faas-pods` (PR #123) at `df75d4d` — push fresh fixes here.
- `cell-workflows-on-main` (PR #124, throwaway) — close after GREEN; do NOT merge.
