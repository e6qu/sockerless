# Sockerless — Status

**Date: 2026-05-05 v31 — Cell 7 GREEN; Cell 8 architectural fix in flight (8 iterations); Cells 5+6 dispatcher refactor not yet started.**

## Cell scoreboard (4 GCP cells = the user's "consider it done" gate)

| Cell | Path | State | Latest |
|------|------|-------|--------|
| **5** GH × cloudrun | sockerless-cloudrun | ❌ NOT STARTED | github-runner-dispatcher-gcp still on OLD-arch (custom runner image baking sockerless). Refactor required: vanilla `actions/runner` + sockerless sidecar in a multi-container Cloud Run Job. |
| **6** GH × gcf | sockerless-gcf | ❌ NOT STARTED | Same dispatcher refactor as cell 5; will inherit cell 8's gcf fixes. |
| **7** GL × cloudrun | sockerless-cloudrun | ✅ **GREEN heavy workload** 2026-05-05 | https://gitlab.com/e6qu/sockerless/-/pipelines/2500209956 (job 14213994152, 383 s, all 5 arithmetic results 11/14/21/13/6.5). |
| **8** GL × gcf | sockerless-gcf | 🟡 **8 iterations today; v8 in flight** | v7 reached `get_sources` (past prepare_executor + prepare_script) but failed `Container not found` on docker exec lookup. v8 fix: `cloud_state.queryPodServiceContainers` GetService follow-up + diagnostic logging. |

The cells 1-4 (AWS) are GREEN-trivial-workload only; the 4 GCP cells are the milestone the user has scoped as "must work fully before this is done".

## Architecture (cells 5-8 NEW vanilla-runner pattern)

Per user directives 2026-05-04:

1. github + gitlab runners stay UNMODIFIED (vanilla upstream images).
2. Only acceptable thing for GitHub is a dispatcher; for GitLab no dispatcher (gitlab-runner's docker executor IS the dispatcher).
3. Runners talk to sockerless via `DOCKER_HOST=tcp://localhost:3375` (cloudrun) / `:3376` (gcf); no sockerless code baked into runner images.
4. `GIT_STRATEGY` must work for `clone`/`fetch`/`none`.
5. HTTP 5xx is reserved for unexpected panics; failures signal via `X-Sockerless-Exit-Code` header / envelope `exitCode`.

**Cell 7 + 8 (GitLab):** pre-deployed multi-container Cloud Run Service per cell with three containers — `init` (registers fresh runner via gitlab API + writes `/shared/config.toml`), `gitlab-runner` (vanilla `gitlab/gitlab-runner:v17.5.0`, depends on init), and `sockerless` (standalone backend image, ingress on :3375 or :3376).

**Cells 5 + 6 (GitHub):** pre-deployed Cloud Run Job per cell label with multi-container TaskTemplate (vanilla `actions/runner --ephemeral` + sockerless sidecar). Dispatcher's only call is `Executions.RunJob(<predefined-job>)` with per-execution env override (`RUNNER_REG_TOKEN`, `RUNNER_NAME`, `RUNNER_LABELS`, `RUNNER_REPO`). **Not yet implemented** — current dispatcher creates a Job per spawn with a custom runner image; sockerless is baked in.

## Bug stack closed today (2026-05-05)

| Bug | Title | Status |
|---|---|---|
| 947 | GCSFuse `/builds` ~200× slower than tmpfs for git ops | ✅ closed (Volume_EmptyDir tmpfs + bootstrap tar-pack persist to GCS) |
| 950 | gcf `OverlayContentTag` fragmentation across container types | ✅ closed (drop entrypoint/cmd/workdir from contentTag; pass at runtime via env) |
| 951 | gcf claim-side env-update via UpdateService hits regional CPU quota | ✅ closed (drop env-update; pass user entrypoint via invoke exec envelope) |
| 952 | gcf `resolveGCFFromCloud` returns empty Function URL | ✅ closed (GetFunction follow-up + Service URL fallback) |

Open bugs: BUG-948 (pool-warming works for single-container claims; pod-mode covered by BUG-953), BUG-949 (pre-existing simulator macOS arithmetic test mismatch — low priority), BUG-953 (pod-materialize still being verified; v8 in flight). Plus older open: BUG-923, 925, 929, 942, 944, 945, 946.

## Live infra in `sockerless-live-46x3zg4imo` (us-central1)

- `gitlab-runner-cloudrun` rev `00003-csp` (sockerless-backend-cloudrun@sha256:f786c300...) — cell 7 serving GREEN.
- `gitlab-runner-gcf` rev `00039-rbj` (sockerless-backend-gcf@sha256:c69b3711...) — full BUG-947/948/950/951/952/953 fix stack incl. pod-mode-as-CR-Service.
- `github-runner-dispatcher-gcp` rev `00021-fb2` — OLD architecture; cells 5+6 refactor pending.
- VPC connector `sockerless-connector` (e2-micro × 4 min instances), Cloud NAT static IP `34.31.88.230`.
- Filestore not provisioned (held in reserve as Path B alternative for git ops if tar-pack ever proves insufficient).

## Active deployment images (Artifact Registry)

| Image | Digest | Notes |
|---|---|---|
| sockerless-backend-cloudrun | `sha256:f786c300...` | Cell 7 GREEN against this |
| sockerless-backend-gcf | `sha256:c69b3711...` | v8 in flight; full pod-mode-as-CR-Service stack |
