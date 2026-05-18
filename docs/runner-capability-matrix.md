# Runner Capability Matrix

Tracks what each backend can actually do when driving real CI runners through the Docker API.

## 8/8 cells GREEN (2026-05-07)

The runner-integration milestone closed: every cell-pair (GitHub × {ECS, Lambda, cloudrun, gcf} and GitLab × the same four) runs the full probe + git-clone + go-build + arithmetic suite end-to-end against real cloud infrastructure. Cell URLs in [STATUS.md](../STATUS.md). The capability summary table below remains the canonical answer for the architectural question; the per-pipeline matrices below it stay TBD until a Docker-in-Docker CI job cycles through the combinations.

## Capability summary (post-Phase 168 / PR #170)

`specs/CLOUD_RESOURCE_MAPPING.md` carries the **architectural** runner-compatibility matrix (long-lived containers vs invocation-scoped FaaS; `tail -f /dev/null` keep-alive; `docker exec` transport). That matrix answers "can this backend ever serve as the docker daemon for a runner?" — summarised here:

| Backend | Long-lived container? | `docker exec` transport | Suitable for GitLab/GitHub runner? |
|---|---|---|---|
| docker | ✅ | native | ✅ |
| ecs | ✅ Fargate task | SSM ExecuteCommand | ✅ verified live (cells 1+3 GREEN) |
| lambda | ✅ image-mode container w/ overlay-inject | reverse-agent WebSocket | ✅ verified live (cells 2+4 GREEN) |
| cloudrun | ✅ multi-container Service revision (pod-Service materialize) | reverse-agent WebSocket | ✅ verified live (cells 5+7 GREEN, gcs-sync workspace data plane) |
| gcf | ✅ multi-container Cloud Run Service via Cloud Functions Gen2 escape hatch | reverse-agent WebSocket | ✅ verified live (cells 6+8 GREEN) |
| aca (UseApp) | ✅ ACA App | reverse-agent WebSocket | ✅ architecturally; live cells not yet exercised |
| azf | ✅ image-mode container | reverse-agent | ✅ architecturally; live cells not yet exercised |
| cloudrun Jobs / aca Jobs | ❌ execution-scoped | — | ❌ use Services/Apps for runner workloads |

This file tracks the **empirical** results of running the `make e2e-*` targets (per-pipeline) against each backend. Cells stay `TBD` until a Docker-in-Docker CI job cycles through the combinations (scripts under `scripts/phase86/*.sh` + `.github/workflows/phase86-aws-live.yml` for live-AWS; `smoke-test-act-*` / `smoke-test-gitlab-*` make targets for sim mode).

## How to populate this matrix

1. For each row `<backend>`:
   ```bash
   make e2e-github-<backend>   # GitHub runner (via act) against a simulator endpoint
   make e2e-gitlab-<backend>   # GitLab runner against a simulator endpoint
   ```
2. Copy the PASS/FAIL lines from `tests/e2e-live-tests/logs/summary-<runner>-<backend>-<ts>.txt` into the matrix below.
3. Cells reflect simulator-endpoint results unless marked `(live)`.

Backends: `memory`, `docker`, `ecs`, `lambda`, `cloudrun`, `gcf`, `aca`, `azf`.
Pipelines: the sets in `tests/e2e-live-tests/github-runner/run.sh` `ALL_WORKFLOWS` and `tests/e2e-live-tests/gitlab-runner-docker/run.sh` `ALL_PIPELINES`.

## GitLab Runner (simulator endpoint)

| Pipeline \ Backend | memory | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|---|---|---|---|---|---|---|---|---|
| basic | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| multi-step | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| env-vars | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| exit-codes | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| before-after | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| multi-stage | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| artifacts | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| large-output | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| parallel-jobs | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| timeout | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| complex-scripts | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| variable-features | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| job-artifacts | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| large-script-output | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| concurrent-lifecycle | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| services-http | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| dag-dependencies | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| rules-conditional | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| multi-image-jobs | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| allow-failure-exit-code | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| container-action | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |

## GitHub Actions (simulator endpoint, via `act`)

| Workflow \ Backend | memory | docker | ecs | lambda | cloudrun | gcf | aca | azf |
|---|---|---|---|---|---|---|---|---|
| basic | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| multi-step | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| env-vars | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| exit-codes | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| multi-job | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| container-action | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| large-output | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| matrix | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| working-dir | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| outputs | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| shell-features | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| file-persistence | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| job-outputs | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| concurrent-jobs | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| env-inheritance | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| github-env | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| step-outputs | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| defaults-shell | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| conditional-steps | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| multi-job-data | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| services-http | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| container-options | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| container-env-create | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| diamond-deps | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| matrix-multi | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| conditional-job | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| continue-on-error | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| timeout-job | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |
| working-dir-nested | TBD | TBD | TBD | TBD | TBD | TBD | TBD | TBD |

## Interpretation

- **PASS**: the pipeline's job(s) completed with expected statuses on this backend.
- **FAIL**: the backend attempted the pipeline and it did not reach the expected end-state.
- **SKIP**: the backend cannot be reached in this environment (missing credentials for live mode, binary not built, etc.) — not a capability claim.

This matrix supersedes the prior silent `-wasm` / `-faas` remapping. Before this change, several cells reported green because the orchestrators pushed a variant file that no longer existed after commit `daeff00`, yet earlier logs from before that deletion were retained. The matrix records honest results.

## Known caveats

- GitHub workflow `container-action` and GitLab pipeline `container-action` were renamed from `container-action-faas` in this PR. The test body is unchanged.
- `services` and `custom-image` are removed from the `ALL_WORKFLOWS` / `ALL_PIPELINES` lists — the corresponding `.yml` files were removed in commit `daeff00` and no replacement was added. `services-http` remains and exercises a real HTTP service container.
- Live-mode cells (AWS / GCP / Azure real accounts) are out of scope for this matrix; the live-cloud sweeps live in [`manual-tests/`](../manual-tests/).
