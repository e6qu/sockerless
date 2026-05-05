# Sockerless — Status

**Date: 2026-05-05 v26 — Cell 7 GREEN; cell 8 hit BUG-948 (gcf Function-deploy CPU quota timeout); pool-warming fix queued**

## Cell scoreboard

| Cell | Path | Last result | Active blocker |
|------|------|-----------|----------------|
| 1 GH × ECS (AWS) | `hello-ecs` echo | ✅ GREEN 2026-04-28 (https://github.com/e6qu/sockerless/actions/runs/25075259911) — trivial workload only | — |
| 2 GH × Lambda (AWS) | `hello-lambda` echo | ✅ GREEN 2026-04-29 (https://github.com/e6qu/sockerless/actions/runs/25113565115) — trivial workload only | — |
| 3 GL × ECS (AWS) | `hello` echo | ✅ GREEN 2026-04-29 (https://gitlab.com/e6qu/sockerless/-/pipelines/2489293496) — trivial workload only | — |
| 4 GL × Lambda (AWS) | `hello` echo | ✅ GREEN 2026-04-30 (https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943) — trivial workload only | — |
| 5 GH × cloudrun | sockerless-cloudrun | ❌ no GREEN under NEW vanilla-runner architecture | github-side dispatcher refactor pending (after gitlab cells GREEN). |
| 6 GH × gcf | sockerless-gcf | ❌ same as cell 5 | same blocker. |
| 7 GL × cloudrun | sockerless-cloudrun | ✅ **GREEN 2026-05-05 v51** under vanilla-runner architecture (https://gitlab.com/e6qu/sockerless/-/pipelines/2500209956, job 14213994152, 383 s). Heavy workload verified: `git fetch + git checkout + apk add + go build + eval-arithmetic` all returned correct results (11/14/21/13/6.5). BUG-947 fix end-to-end confirmed. | — |
| 8 GL × gcf | sockerless-gcf | ❌ v1 FAILED 2026-05-05 (https://gitlab.com/e6qu/sockerless/-/pipelines/2500312481, job 14214568062, `system_failure`). NEW vanilla-runner architecture deployed cleanly (`gitlab-runner-gcf` rev `00028-7jg`); failure is BUG-948 (per-step Function deploy hit Cloud Run regional CPU quota; gitlab-runner timed out at 120s on `ContainerStart` with misleading "Cannot connect to Docker daemon" error). | Pool-warming fix needed: pre-deploy N functions tagged with the standard gitlab-runner cache-permission overlay hash at backend startup. See [BUGS.md](BUGS.md) BUG-948 for analysis + fix candidates. |

**The "GREEN 2026-04-30" claim for AWS cells covers only the `hello` workload (echo + env), NOT the heavy probe + git-clone + go-build + arithmetic suite that cells 5–8 run.** Cell 7 v51 (2026-05-05) is the **first end-to-end heavy-workload pass on GCP under the vanilla-runner architecture** — confirms BUG-947 fix and unblocks cells 5+6+8.

## Architectural pivot — Phase 122j (in flight 2026-05-04)

Per user directives:
1. github + gitlab runners **stay UNMODIFIED** (vanilla upstream images).
2. Only acceptable thing for GitHub is the dispatcher; for GitLab no dispatcher (gitlab-runner's docker executor IS the dispatcher).
3. Runners talk to sockerless via `DOCKER_HOST=tcp://localhost:3375`; no sockerless code baked into runner images.
4. `GIT_STRATEGY` must work for `clone` / `fetch` / `none` — workarounds that disable get_sources are forbidden.

**Resulting architecture (gitlab cells 7+8):** pre-deployed multi-container Cloud Run **Service** per cell with three containers:
1. **init** — `gitlab-runner-init` image (alpine + curl + jq + register.sh). Cleans stale offline project_type runners; `POST /api/v4/user/runners` to register a fresh runner; writes `/shared/config.toml` with auth token + `DOCKER_HOST=tcp://localhost:3375`. Binds `:8081` after registration so Cloud Run's container-dependencies probe passes.
2. **gitlab-runner** — vanilla `gitlab/gitlab-runner:v17.5.0`. `dependsOn: init`.
3. **sockerless** — standalone `sockerless-backend-cloudrun:latest` (FROM distroless, ~56 MB). Cloud Run **ingress** container, binds `:3375`.

GitHub side (cells 5+6) is the same shape but uses Cloud Run **Job** + `actions/runner --ephemeral` per execution. Implementation pending after gitlab cells GREEN.

## What's working

- ✅ Multi-container Cloud Run Service `gitlab-runner-cloudrun` deploys cleanly, init registers fresh runner, vanilla `gitlab/gitlab-runner:v17.5.0` polls + dispatches.
- ✅ sockerless-backend-cloudrun serves Docker API; gitlab-runner reaches it via DOCKER_HOST.
- ✅ Pre-job overlay build via Cloud Build succeeds; `useServicePath=true` routing confirmed (`SOCKERLESS_GCR_USE_SERVICE=1` env).
- ✅ Step containers deploy as Cloud Run Services with `ALL_TRAFFIC` egress through VPC connector + Cloud NAT.
- ✅ git fetch over gitlab.com (~2 MB pack) completes successfully (since connector min-instances 2→4 scale-up).

## BUG-947 closed — cell 7 v51 confirms tar-pack persist works

GCSFuse-backed `/builds` was ~200× slower than tmpfs for git operations (cell 7 v50 evidence). Fix landed in 3 commits:
- `1f06831` — bootstrap persist module (download tar at restore, upload tar after every exec)
- `f5e52f1` — backend emits `Volume_EmptyDir{MEMORY}` for ad-hoc binds + injects `SOCKERLESS_PERSIST_VOLUMES=name=path=bucket` env
- `29308e1` — bootstrap status code refactor (200 + `X-Sockerless-Exit-Code` header instead of 500) per user directive

Cell 7 v51 (pipeline 2500209956, 383 s) ran the full heavy workload — git fetch, git checkout, apk add file, go build eval-arithmetic, run with PostgreSQL sidecar — all 5 arithmetic results correct. Tar-pack roundtrip ~2-5 s per stage as predicted.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

- `github-runner-dispatcher-gcp` rev `00021-fb2` — OLD architecture (will replace when cells 5+6 refactor).
- `gitlab-runner-cloudrun` rev `00003-csp` — NEW vanilla architecture with BUG-947 tar-pack persist baked in (sockerless-backend-cloudrun@sha256:f786c300...). Healthy. Cell 7 v51 GREEN against this revision.
- `gitlab-runner-gcf` rev `00028-7jg` — NEW vanilla 3-container architecture (init + vanilla gitlab-runner + sockerless-backend-gcf@sha256:4c84a691...). Healthy at the Service level; cell 8 v1 hit BUG-948 (per-step gcf Function deploy quota).
- Orphan function `skls-gcf-19eef3119854a1bc-9e2e52` — left ACTIVE from cell 8 v1; deploy eventually succeeded after gitlab-runner gave up. Will be cleaned up by sockerless pool-prune or operator.
- `gitlab-runner-gcf` rev `00027-jkg` — OLD architecture (full refactor pending for cell 8).
- VPC connector `sockerless-connector` — e2-micro × 4 min instances (raised from 2 today).
- Cloud NAT `sockerless-nat` — static IP `34.31.88.230`.
- Filestore — NOT provisioned (Path B held in reserve).
- Stale Cloud Run Job `sockerless-491f3e44a7eb` — leftover from cell 7 v5; `gcloud run jobs delete` was permission-denied; left for next session/operator.

## What we tried this session that did NOT work

1. **Custom runner images that bake sockerless backend** — rejected by user directives.
2. **GCSFuse `metadata-cache:ttl-secs=0` MountOptions** — Cloud Run rejects unsupported flags. Reverted (`b7aeaf2`).
3. **Init container that exits on completion** — Cloud Run requires "ready" (probe passing), not "exited". Fixed by binding `:8081` after init.
4. **`gcloud run services replace` with `:latest` tag** — Cloud Run doesn't re-resolve `:latest` if spec is byte-identical. Pin digests when iterating.
5. **`--working-directory /tmp/runner-work`** override on vanilla gitlab-runner — directory missing in image. Removed; default `/home/gitlab-runner/builds` works.
6. **Cloud Run regional CPU quota raise request** — user rejected ("quota increase is the wrong path").
7. **Path A — emptyDir + single-Service-per-job** — Cloud Run revisions are immutable; modifying a Service spawns a new instance with fresh emptyDir. Architecturally infeasible.
8. **Filestore (Path B)** — viable but $160/mo BASIC_HDD floor. Held in reserve.
9. **Filestore on-demand per job** — provisioning latency 5–15 min would blow gitlab-runner's job timeout.
10. **`GIT_STRATEGY=none` workaround** — user explicitly rejected: "we still want the gitlab runner to support the GIT_STRATEGY feature for all values of it".
11. **`fuse-overlayfs` (tmpfs upper, gcsfuse lower)** — Cloud Run gen2 may not have syscall caps; per-file sync at exit would still be slow.
12. **LD_PRELOAD shim for `link()`/`flock()`/`rename()`** — image-specific, fragile, breaks "vanilla runner".

## Today's commits (chronological, on `phase-118-faas-pods`)

| Commit | Subject |
|--------|---------|
| `153f95c` | docs(BUG-947): file GCSFuse-vs-git-checkout incompatibility — cell 7 v50 evidence |
| `bb420ca` | docs(BUG-947): correct analysis — Path A infeasible due to Cloud Run revision immutability; Path B (Filestore NFS) is the only practical fix |
| `4d7e5d8` | docs(BUG-947): empirical confirmation — GCSFuse ~200× slower than tmpfs for git |
| `1f06831` | feat(BUG-947): wire persist module into bootstrap startup + invoke handler (tar-pack approach replaces Path B as chosen fix) |
| `b381612` | docs: state save — Phase 122j BUG-947 tar-pack approach in flight; persist module committed, backend + redeploy pending |
| `f5e52f1` | feat(BUG-947): backend volume_emptydir + SOCKERLESS_PERSIST_VOLUMES injection — both cloudrun + gcf emit Volume_EmptyDir{MEMORY} for ad-hoc binds + inject persist env on main container; SharedVolumes keep raw GCSFuse |
| `29308e1` | refactor(bootstrap): replace HTTP 500 with 200 + X-Sockerless-Exit-Code — 5xx reserved for unexpected panics; expected failures (subprocess crash, missing entrypoint, persist save) signal via the existing exit-code header/envelope |
| `dcb20c3` | docs: state save — BUG-947 fix code-complete (backend + bootstrap), image rebuild + retest pending |
| `687cdb8` | deploy(BUG-947): bump sockerless-backend-cloudrun digest to f786c300 — gitlab-runner-cloudrun rev 00003-csp deployed; cell 7 v51 GREEN against this revision |

See [BUGS.md](BUGS.md) for per-bug fix shape. See [DO_NEXT.md](DO_NEXT.md) for the next steps (cell 8 + cells 5-6 GH refactor).
