# Runner Capability Matrix

Tracks what each backend can actually do when driving real CI runners through the Docker API. No `-wasm` / `-faas` shortcut variants.

## Phase 86 status

**Simulator-API parity**: complete. See `docs/SIMULATOR_PARITY_{AWS,GCP,AZURE}.md` — zero ✖ rows on the runner path; the 707 bugs logged during Phase 86 are all fixed. Runner-command unit coverage via sim SDK + CLI tests is green for every cloud.

**Pipeline-level matrix population** (below): TBD cells are filled in by running the `make e2e-*` targets against each backend. The cells stay TBD until a dedicated Docker-in-Docker CI job cycles through the combinations — scripts for that job land via `scripts/phase86/*.sh` + `.github/workflows/phase86-aws-live.yml` (live-AWS) and the existing `smoke-test-act-*` / `smoke-test-gitlab-*` make targets (sim mode).

**Live-AWS columns**: pending-live. `phase86-aws-live.yml` is dispatched manually when AWS credentials are available.

## How to populate this matrix

1. For each row `<backend>`:
   ```bash
   make e2e-github-<backend>   # GitHub runner (via act) against simulator-mode sockerless
   make e2e-gitlab-<backend>   # GitLab runner against simulator-mode sockerless
   ```
2. Copy the PASS/FAIL lines from `tests/e2e-live-tests/logs/summary-<runner>-<backend>-<ts>.txt` into the matrix below.
3. Cells reflect simulator-mode results unless marked `(live)`.

Backends: `memory`, `docker`, `ecs`, `lambda`, `cloudrun`, `gcf`, `aca`, `azf`.
Pipelines: the sets in `tests/e2e-live-tests/github-runner/run.sh` `ALL_WORKFLOWS` and `tests/e2e-live-tests/gitlab-runner-docker/run.sh` `ALL_PIPELINES`.

## GitLab Runner (simulator mode)

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

## GitHub Actions (simulator mode, via `act`)

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
- Live-mode cells (AWS / GCP / Azure real accounts) are out of scope for this matrix; track them in a `-live` variant of this doc under Phase 86 AWS track.
