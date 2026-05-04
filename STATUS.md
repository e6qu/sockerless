# Sockerless — Status

**Date: 2026-05-04 v21 (vanilla-runner architecture pivot — Phase 122j)**

## Cell scoreboard

| Cell | Path | Last result | Active blocker |
|------|------|-----------|----------------|
| 1 GH × ECS (AWS) | n/a | ✅ GREEN 2026-04-30 | — |
| 2 GH × Lambda (AWS) | n/a | ✅ GREEN 2026-04-30 | — |
| 3 GL × ECS (AWS) | n/a | ✅ GREEN 2026-04-30 | — |
| 4 GL × Lambda (AWS) | n/a | ✅ GREEN 2026-04-30 | — |
| 5 GH × cloudrun | sockerless-cloudrun | ❌ — old custom-image architecture being torn down | New shape: vanilla actions/runner + sockerless sidecar in multi-container Cloud Run Job, dispatched by `github-runner-dispatcher-gcp`. Implementation pending after gitlab side is GREEN. |
| 6 GH × gcf | sockerless-gcf | ❌ same as cell 5 | Same blocker — gcf sidecar version pending. |
| 7 GL × cloudrun | sockerless-cloudrun | 🟡 in flight on new architecture; gitlab-runner connects to sockerless via DOCKER_HOST; **step-container deploy via Cloud Run Service hangs (BUG-929 + Cloud Run regional CPU quota)** | New shape proven viable at the runner level; downstream backend issues persist. |
| 8 GL × gcf | sockerless-gcf | ❌ pending — refactor to vanilla `gitlab/gitlab-runner` + gcf sidecar pending. | Same architecture as cell 7, swap sockerless-backend-cloudrun → sockerless-backend-gcf. |

## Architectural pivot — Phase 122j (in flight 2026-05-04)

Per user directives (5 messages 2026-05-04 afternoon):
1. "github runner and gitlab runner unmodified"
2. "only acceptable thing for github is this dispatcher"
3. "runners work via docker-like interface of sockerless ... runners need no changes themselves since they already talk docker"
4. "dispatcher just provisions runners based on the job demand and should not provide any features other than to start the github runner"
5. "gitlab runner doesn't need a dispatcher because gitlab runner 'docker executor' already behaves like a dispatcher"

**Resulting architecture:**

- **GitLab side**: pre-deployed multi-container Cloud Run **Service** per cell (deployed via `terraform/cloud-run/gitlab-runner-cloudrun.yaml` + soon `gitlab-runner-gcf.yaml`). Three containers per revision:
  1. **init** — `gitlab-runner-init` image (alpine + curl + jq + register.sh). Cleans stale offline project_type runners from gitlab project (was hitting GitLab's 50-runner cap from old phase 110 cells), then `POST /api/v4/user/runners` to register a fresh `project_type` runner. Writes `/shared/config.toml` with the auth token + `DOCKER_HOST=tcp://localhost:3375`. Cloud Run requires the dependency container to keep running, so init binds `:8081` after writing config + sleeps. Cloud Run `container-dependencies` annotation makes the runner depend on init's startup probe passing.
  2. **gitlab-runner** — vanilla `gitlab/gitlab-runner:v17.5.0`. Args: `run --config /shared/config.toml`. `dependsOn: init`. `DOCKER_HOST=tcp://localhost:3375` connects to sibling.
  3. **sockerless** — `sockerless-backend-cloudrun:latest` (FROM distroless, ~56MB). Cloud Run **ingress** container — Docker daemon HTTP API serves on `:3375`.

- **GitHub side** (still TODO): pre-deployed Cloud Run **Job** per cell label with multi-container TaskTemplate (vanilla `github-runner-vanilla:2.334.0` + sockerless sidecar). Dispatcher's only call: `Executions.RunJob(predefined-job)` with per-execution env override (`RUNNER_REG_TOKEN`, `RUNNER_NAME`, `RUNNER_LABELS`, `RUNNER_REPO`).

**Vanilla runner image source** chosen (option b2 per user): `FROM mcr.microsoft.com/dotnet/runtime-deps:8.0` + `COPY` extracted upstream actions/runner tarball. Microsoft's image is GitHub's documented base for .NET runtime deps actions/runner needs. Image content matches upstream prescription.

## Today's commits (Phase 122j)

| Commit | Subject |
|--------|---------|
| `558511d` | standalone sockerless-backend sidecar images (distroless) |
| `4b2789c` | gitlab-runner-init image + vanilla github-runner image |
| `b7aeaf2` | revert MountOptions metadata-cache flags — Cloud Run rejects them |

## What's working

- ✅ Multi-container Cloud Run Service `gitlab-runner-cloudrun` deploys cleanly with [init, vanilla gitlab-runner, sockerless sidecar].
- ✅ init script runs at every revision spin-up: cleans stale offline runners (deleted 48 stale phase-110 runners, brought project from 50/50 to 2/50), registers fresh runner via gitlab API HTTP 201, writes config.toml.
- ✅ Vanilla `gitlab/gitlab-runner:v17.5.0` reads config.toml, connects to GitLab as a project runner, polls.
- ✅ sockerless-backend-cloudrun serves Docker API on `:3375`, gitlab-runner reaches it via `DOCKER_HOST=tcp://localhost:3375`.
- ✅ Pre-job overlay build via Cloud Build succeeds.

## What's blocking cell 7 v2

- ❌ Sockerless backend's `startSingleContainerService` for the cache permission helper container hangs at `CreateService.Wait` — no Cloud Run service appears, no error logged, gitlab-runner times out after 13+ min waiting for "running" state.
- Likely **BUG-929** (`startSingleContainerService missing post-deploy invoke` — known pending) compounded with **Cloud Run regional CPU quota** on the test project (we previously hit "Quota exceeded for total allowable CPU per project per region" repeatedly).
- The hang isn't logged because Cloud Run's CreateService API call returns immediately (LRO) and `.Wait()` polls without logging. Need either (a) timeout on Wait + explicit error or (b) verbose progress logging.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

- `github-runner-dispatcher-gcp` rev `00021-fb2` — STILL the OLD architecture (will replace when github side is refactored).
- `gitlab-runner-cloudrun` rev `00001-x4q` — NEW vanilla architecture, healthy.
- `gitlab-runner-gcf` rev `00027-jkg` — STILL the OLD architecture (custom image).
- VPC + connector + Cloud NAT + secrets unchanged.
- Runner SA granted `roles/dns.admin` (was getting 403 on Cloud DNS zone create — now fixed for cross-container DNS).

## What we tried that did NOT work this session

1. **Custom runner images that bake sockerless backend** — entire architecture rejected by user directives. Old `runner:gcf-amd64`, `runner:cloudrun-amd64`, `gitlab-runner:gcf-amd64`, `gitlab-runner:cloudrun-amd64` images are now obsolete (pending step-4 deletion).
2. **GCS-Fuse `metadata-cache:ttl-secs=0` / `negative-ttl-secs=0` MountOptions** — Cloud Run rejects with `Unsupported or unrecognized flag for Cloud Storage volume`. Cloud Run wraps gcsfuse and only allows `[implicit-dirs, o=, file-mode, dir-mode, uid, gid]`. Reverted in `b7aeaf2`.
3. **Init container that exits on completion** — Cloud Run requires the dependency container to be "ready" (startup probe passing), not "exited". Init now binds `:8081` after writing config + sleeps.
4. **`gcloud run services replace` with `:latest` tag** — Cloud Run doesn't re-resolve `:latest` if the spec is byte-identical. Pin digests when iterating.
5. **`--working-directory /tmp/runner-work`** override on vanilla gitlab-runner — directory doesn't exist in vanilla image. Removed; let gitlab-runner use its default `/home/gitlab-runner/builds`.

See [BUGS.md](BUGS.md) for per-bug fix shape. See [DO_NEXT.md](DO_NEXT.md) for resume runbook + remaining BUG-929 fix candidates.
