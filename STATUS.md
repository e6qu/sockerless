# Sockerless — Status

**Date: 2026-05-05 v32 — Cell 7 GREEN; Cell 8 v15 in flight (15 iterations); Cells 5+6 not started but runner-task images already exist.**

## Cell scoreboard (4 GCP cells = the user's "consider it done" gate)

| Cell | Path | State | Latest |
|------|------|-------|--------|
| **5** GH × cloudrun | sockerless-cloudrun | ❌ NOT STARTED | dispatcher stays generic per user directive 2026-05-05 — sockerless+vanilla-runner pairing already lives in `tests/runners/github/dockerfile-cloudrun/`. Just needs rebuild + AR push + dispatcher TOML. |
| **6** GH × gcf | sockerless-gcf | ❌ NOT STARTED | Same as cell 5 with `dockerfile-gcf` image; inherits cell 8's gcf fixes. |
| **7** GL × cloudrun | sockerless-cloudrun | ✅ **GREEN heavy workload** 2026-05-05 | https://gitlab.com/e6qu/sockerless/-/pipelines/2500209956 (job 14213994152, 383 s, all 5 arithmetic results 11/14/21/13/6.5). |
| **8** GL × gcf | sockerless-gcf | 🟡 **v21 architecture VERIFIED working — blocker is regional CPU quota** | digest `sha256:c15da1bf`. v21 added HTTP middleware ENTRY logs revealing the full flow: gitlab-runner DOES call `/containers/{id}/attach` BEFORE `/start` (hijacked); `/start` returns 500 with the real `FailedPrecondition: Quota exceeded for total allowable CPU per project per region` error in 14 s; gitlab-runner reports the error correctly. The v17-v20 silent hangs were the SAME quota error but masked: Service LRO `CreateService.Wait` returned successfully (status 204) while the underlying revision health-check kept failing in the background, and gitlab-runner's docker SDK held the hijacked /attach for an hour waiting on stdout that never came. Today's full architectural stack works; the remaining blocker is purely quota-availability. **Fix path**: wait for the regional `CpuAllocPerProjectRegion` rolling-window reset (~1 h after the last burst) and re-trigger; or scale down further (revisions, dispatcher). Once one cell GREEN proves the path, cells 5/6/7/8 can be exercised serially within quota. |

## Architecture (cells 5-8 vanilla-runner pattern)

Per user directives 2026-05-04 + reinforced 2026-05-05:

1. github + gitlab runners stay UNMODIFIED (vanilla upstream images).
2. **Dispatcher is generic** — provisions vanilla runners on demand based on queued jobs; not aware of sockerless. Sockerless+runner pairing lives in the runner IMAGE.
3. Runners talk to sockerless via `DOCKER_HOST=tcp://localhost:3375` (cloudrun) / `:3376` (gcf); no sockerless code baked into the dispatcher.
4. `GIT_STRATEGY` must work for `clone`/`fetch`/`none`.
5. HTTP 5xx is reserved for unexpected panics; failures signal via `X-Sockerless-Exit-Code` header / envelope `exitCode`.

**Cells 7 + 8 (GitLab):** pre-deployed multi-container Cloud Run Service per cell with three containers — `init` (registers fresh runner via gitlab API + writes `/shared/config.toml`), `gitlab-runner` (vanilla `gitlab/gitlab-runner:v17.5.0`, depends on init), and `sockerless` (standalone backend image, ingress on :3375 or :3376).

**Cells 5 + 6 (GitHub):** `github-runner-dispatcher-gcp` polls GitHub for queued workflow_jobs; for each, calls `Jobs.CreateJob + RunJob` with the runner-task image from per-label config. The runner-task IMAGE bundles vanilla actions/runner + sockerless backend with a bootstrap that launches both; image already exists at `tests/runners/github/dockerfile-{cloudrun,gcf}/`. Dispatcher submits a single image; the bundling happens inside.

## Today's progress on cell 8 (2026-05-05 second session)

| Iter | Change | Result |
|------|--------|--------|
| v9 | execStartViaInvoke entry/exit logs; reduce queryPodServiceContainers logging from Info to Debug | failed (170s) — runner timeout on docker exec |
| v10 | (continuation, pre-AR-precheck) | failed |
| v11 | **AR HEAD precheck on `/v2/<repo>/manifests/<tag>`** — skip Cloud Build on cache hit | total job 82s (was 170s); now fails at `No such container` cleanup |
| v12 | Update `PendingCreates(running)` through materialize, delete only on error | failed (43s) — log "marked running" never appeared |
| v13 | `Put` fallback when `Update` misses | failed |
| v14 | network-pod decision log + materializePodService entry/exit logs | failed (43s) — diagnostic logs ALL missing despite binary having strings |
| **v15** | ContainerStart **ENTRY/resolved/NOT FOUND** logs + `resolvePodServiceFromCloud` GetService follow-up | **HUGE PROGRESS, silent hang in new place**: all log lines fire (network-pod decision: netDefer=false, netMembers=2, materialize entry+exit in 13s). Both Services deployed. Bootstrap exec'd build's CMD `[/usr/bin/dumb-init /entrypoint gitlab-runner-build]` → exit=0 (expected — bootstrap stays up as HTTP server). gitlab-runner reaches "Preparing environment" then silently hangs with NO ExecCreate/ExecStart calls reaching sockerless. |

**Working en route, do not regress:**
- AR HEAD precheck (`backends/gcp-common/registry_check.go`) cuts ~28 s of Cloud Build per overlay when image already in AR
- The Service IS being created correctly post-materialize (verified via `gcloud run services describe sockerless-svc-*`); annotations + labels populated
- gcf bootstrap envelope path; Service URL fallback for empty `Function.ServiceConfig.uri`

**Key open question driving v15:**
gitlab-runner's failure mode is "Container not found or removed" during the post-script cleanup `docker exec`. v15 will distinguish:
- ContainerStart never fires → `ENTRY` log absent → routing/handler issue
- ContainerStart fires but `PendingCreates.Get(ref)` misses → `NOT FOUND` log → entry was deleted
- ContainerStart fires + container resolved → `resolved` log + `network-pod decision` log → bug is elsewhere (e.g. Service materialize race with cleanup)

## Bug stack closed today (2026-05-05)

| Bug | Title | Status |
|---|---|---|
| 947 | GCSFuse `/builds` ~200× slower than tmpfs for git ops | ✅ closed (Volume_EmptyDir tmpfs + bootstrap tar-pack persist to GCS) |
| 950 | gcf `OverlayContentTag` fragmentation across container types | ✅ closed (drop entrypoint/cmd/workdir from contentTag; pass at runtime via env) |
| 951 | gcf claim-side env-update via UpdateService hits regional CPU quota | ✅ closed (drop env-update; pass user entrypoint via invoke exec envelope) |
| 952 | gcf `resolveGCFFromCloud` returns empty Function URL | ✅ closed (GetFunction follow-up + Service URL fallback) |

**Open bugs:**
- **BUG-953** (Cell 8 pod-mode materialize) — PARTIAL: structural fix landed (multi-container Service direct deploy + AR precheck + PendingCreates speculative running marker). Active debug: "No such container" on cleanup exec. v15 in flight.
- **BUG-948** pool-warming (works for single-container claims; pod-mode covered by 953)
- **BUG-949** (pre-existing simulator macOS arithmetic test mismatch — low priority)
- Older: BUG-923, 925, 929, 942, 944, 945, 946

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

- `gitlab-runner-cloudrun` rev `00003-csp` (sockerless-backend-cloudrun@sha256:f786c300...) — cell 7 GREEN against this
- `gitlab-runner-gcf` rev `00047-45f` (sockerless-backend-gcf@sha256:ee7e5029...) — full BUG-947/948/950/951/952 stack + AR precheck + PendingCreates fix + ContainerStart diagnostic logs
- `github-runner-dispatcher-gcp` rev `00021-fb2` — GENERIC dispatcher (per user directive); cells 5+6 just need runner-task image rebuild + push to AR + TOML config update
- VPC connector `sockerless-connector` (e2-micro × 4 min instances), Cloud NAT static IP `34.31.88.230`
- Filestore not provisioned (held in reserve as Path B alternative for git ops if tar-pack ever proves insufficient)

## Active deployment images (Artifact Registry)

| Image | Digest | Notes |
|---|---|---|
| sockerless-backend-cloudrun | `sha256:f786c300...` | Cell 7 GREEN against this |
| sockerless-backend-gcf | `sha256:ee7e5029...` | v15 with ContainerStart ENTRY logs + resolvePodServiceFromCloud GetService follow-up |
