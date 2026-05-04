# Do Next

Resume pointer. Roadmap detail in [PLAN.md](PLAN.md); narrative in [WHAT_WE_DID.md](WHAT_WE_DID.md); bug log in [BUGS.md](BUGS.md); architecture in [specs/CLOUD_RESOURCE_MAPPING.md](specs/CLOUD_RESOURCE_MAPPING.md).

## Resume pointer (2026-05-04 v19)

**Goal**: cells 5/6/7/8 GREEN with REAL workload (compile + use eval-arithmetic + probe environment) before merging PR #123.

### Where we are

Phase 122i shipped 13 commits over multi-iteration debug. Latest at `a7e3b00`. Cell 6 has been the live experiment because gcf is the most contested path (CPU quota + pool reuse + GCS-Fuse). Investigation peeled BUG-944 in 3 layers (see STATUS.md table). All three layers shipped; verification pending after image rebuild + retrigger.

### Immediate next actions (in dependency order)

1. **Fix runner image build flakiness** (newly demoted from "transient" to real bug, per user directive). The `installdependencies.sh` step in `tests/runners/github/dockerfile-{gcf,cloudrun}/Dockerfile` fetches Microsoft .NET runtime deps from `packages.microsoft.com`. Failures: intermittent network or apt-get hash-mismatch. Real fix candidates:
   - Pin to a known-good apt snapshot (use `snapshot.debian.org` style mirror).
   - Pre-bake the apt-cache layer and skip re-downloading.
   - Add `--retry-on-error` style to the apt step (last resort — workaround).

   Investigation in flight (background task `bakyy540j`). After root cause is named, ship a real fix; don't retry-loop.

2. **Verify BUG-944 layer 3 fix** (`a7e3b00`). Process:
   - Rebuild + push `runner:gcf-amd64` (after #1 lands).
   - Trigger cell 6 SOLO (`gh workflow run cell-6-gcf.yml ...`).
   - **BEFORE assuming success**: dump `gcloud run services describe <skls-gcf-*> --format=json | jq .spec.template.spec.volumes` for both deployed services and verify `mountOptions` field is present with the 3 entries.
   - If exit 126 still, dig into bootstrap.sh (host-side fsync? container-side stat-fresh?) — the chain is: github-runner writes script → host gcsfuse syncs to GCS → container's gcsfuse reads. Each link is a potential cache point.

3. **Fix BUG-929 cell 5 hang** (`backends/cloudrun/start_service.go::startSingleContainerService missing post-deploy invoke`). Cell 5 hangs Initialize containers 43+ min instead of failing fast. This is independent of BUG-944. Code-side fix candidates:
   - Cold-start service needs an explicit POST after deploy to bring up the first instance (otherwise `min-instances=0` keeps it cold and any access blocks).
   - OR set `min-instances=1` for runner-pattern services (similar to gcf path which already does this).

4. **Re-trigger cell 7** via `.gitlab-ci.yml` swap (cell-7-cloudrun.yml content). Cell 7 was GREEN once at the BUG-925 12-step fix; verify nothing this session broke it.

5. **Cell 8 SOLO**: rebuild gitlab-runner-gcf image with all of `a7e3b00`, trigger via `.gitlab-ci.yml` swap to cell-8-gcf.yml.

### Diagnostic discipline carried forward

- After every BUG-944-class fix, **inspect the deployed Cloud Run service spec** (`gcloud run services describe <name> --format=json | jq`) BEFORE re-triggering. Don't trust the in-code claim — verify in production state.
- Test cells **serially**, not in parallel. Per-minute Cloud Run `cpu_allocation` rate window blows otherwise.
- Tail backend logs in real time when re-triggering: github-runner bootstrap now tee's stderr to Cloud Logging at debug level (`a07d63b`). Filter by `resource.labels.job_name="gh-..."`.

### Strict rules carried forward (this session learnings)

- **Transients and flakiness are bugs** (user directive 2026-05-04). No retry-and-hope. Investigate, name root cause, ship real fix.
- **No fallbacks, no workarounds, no fakes.** Pool back-off is OK (architectural amortization). Quota increase is NOT (workaround to avoid solving the real over-allocation issue).
- **Latest stable, no deprecated APIs.** Cloud Run gen2 stays — its ≥1 vCPU floor is a constraint, not a bug.
- **Don't trust GitHub Actions step "completed/success"** without verifying real workload markers (BUG-927 lesson).
- **Don't add runner-specific labels in backend code** — coupling is docker/podman libpod HTTP API on entry, cloud primitives on backend. The translator already handles standard Docker shapes.
- **Don't push images to public registries** — pull via AR remote-proxy only.
- **Don't cancel cells the agent didn't create** — sandbox blocks; ask user.

## Tactical files for resume

- `backends/cloudrun-functions/volumes.go` — full-shape idempotent attach landed `a7e3b00`.
- `backends/cloudrun-functions/backend_impl.go::deployFunction` — pool-hit path landing `ee63dae`.
- `backends/gcp-common/gcsfuse_mount.go` — `RunnerWorkspaceMountOptions()` (used by all 3 GCSVolumeSource sites).
- `backends/cloudrun/start_service.go` — `startSingleContainerService` (BUG-929 fix candidate).
- `tests/runners/github/dockerfile-{gcf,cloudrun}/Dockerfile` — runner image build (flakiness investigation).
- `github-runner-dispatcher-aws/pkg/poller/poller.go` — proactive rate-limit + runSeen.
- `github-runner-dispatcher-gcp/cmd/.../main.go` — `rateLimitedUntil` guard.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

See STATUS.md for the canonical list. Key reminders: NAT pinned to `34.31.88.230` (don't auto-rotate), dispatcher rev `00021-fb2`, gitlab-runner-gcf rev `00027-jkg` (needs rebuild with `a7e3b00`), quota preference withdrawn (don't recreate), pool currently empty (force fresh deploys with MountOptions).

## Branch state

- `phase-118-faas-pods` (PR #123) at `a7e3b00` — push fresh fixes here.
- `cell-workflows-on-main` (PR #124, throwaway) — close after cells GREEN; do NOT merge.
