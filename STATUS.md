# Sockerless — Status

**Date: 2026-05-05 v24 — BUG-947 backend volume_emptydir landed; image rebuild + redeploy + retest pending**

## Cell scoreboard

| Cell | Path | Last result | Active blocker |
|------|------|-----------|----------------|
| 1 GH × ECS (AWS) | `hello-ecs` echo | ✅ GREEN 2026-04-28 (https://github.com/e6qu/sockerless/actions/runs/25075259911) — trivial workload only | — |
| 2 GH × Lambda (AWS) | `hello-lambda` echo | ✅ GREEN 2026-04-29 (https://github.com/e6qu/sockerless/actions/runs/25113565115) — trivial workload only | — |
| 3 GL × ECS (AWS) | `hello` echo | ✅ GREEN 2026-04-29 (https://gitlab.com/e6qu/sockerless/-/pipelines/2489293496) — trivial workload only | — |
| 4 GL × Lambda (AWS) | `hello` echo | ✅ GREEN 2026-04-30 (https://gitlab.com/e6qu/sockerless/-/pipelines/2490478943) — trivial workload only | — |
| 5 GH × cloudrun | sockerless-cloudrun | ❌ no GREEN under NEW vanilla-runner architecture | github-side dispatcher refactor pending (after gitlab cells GREEN). |
| 6 GH × gcf | sockerless-gcf | ❌ same as cell 5 | same blocker. |
| 7 GL × cloudrun | sockerless-cloudrun | 🟡 OLD-arch v49 GREEN 2026-05-03 (custom image, **rejected by user pivot**) — pipeline 2496721473. NEW-arch v50 (2498952453) git-fetched OK after connector min-instances 2→4 scale-up, then hung at `git checkout` (BUG-947 — GCSFuse 200× slower than tmpfs for git ops, verified 211 s vs 1 s diagnostic). | Tar-pack persist module committed (`1f06831`); backend Volume_EmptyDir + persist-env injection committed (`f5e52f1`); bootstrap 500-status replaced with 200+exit-code header per user directive (`29308e1`). **Image rebuild + redeploy + cell 7 v51 retest pending.** See [DO_NEXT.md](DO_NEXT.md). |
| 8 GL × gcf | sockerless-gcf | ❌ pending architectural refactor + same BUG-947 fix | Mirror of cell 7. |

**The "GREEN 2026-04-30" claim for AWS cells covers only the `hello` workload (echo + env), NOT the heavy probe + git-clone + go-build + arithmetic suite that cells 5–8 run.** The only end-to-end heavy-workload pass on GCP was cell 7 v49 (OLD architecture, since rejected).

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

## What's blocking cell 7 going GREEN under NEW architecture (BUG-947)

GCSFuse-backed `/builds` is **~200× slower than tmpfs for git operations** (diagnostic 2026-05-04 22:42 UTC: `git clone e6qu/sockerless` took 211 s on GCSFuse vs 1 s on tmpfs). git checkout exceeds sockerless backend's 10-min HTTP exec timeout → POST returns `Client.Timeout exceeded while awaiting headers` → gitlab-runner reports `Job failed: exit code 1`.

**Fix in flight: tar-pack persist** (BUG-947 chosen approach). Persist module committed to bootstrap (`1f06831`); backend Volume_EmptyDir + SOCKERLESS_PERSIST_VOLUMES env injection committed (`f5e52f1`); bootstrap 500-status replaced with 200+exit-code header per user directive 2026-05-05 (`29308e1`). Image rebuild + redeploy + retest pending. See [DO_NEXT.md](DO_NEXT.md) for the runbook.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

- `github-runner-dispatcher-gcp` rev `00021-fb2` — OLD architecture (will replace when cells 5+6 refactor).
- `gitlab-runner-cloudrun` rev `00002-8l8` — NEW vanilla architecture, healthy. **Will need image bump after persist patch lands.**
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

See [BUGS.md](BUGS.md) for per-bug fix shape. See [DO_NEXT.md](DO_NEXT.md) for the runbook to finish BUG-947.
