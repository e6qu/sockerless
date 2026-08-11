# Scripts

Repo-wide guard scripts, code-quality scans, and operator helpers. Most are wired into the pre-commit / commit-msg / pre-push hooks defined in [`.pre-commit-config.yaml`](../.pre-commit-config.yaml) and/or CI jobs in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml); the rest are run by hand.

Every script must be shellcheck-clean and run under both bash and zsh on macOS and Linux — enforced at pre-push (`shellcheck -x -s bash scripts/*.sh`, `bash -n`, `zsh -n`) and again in the CI lint job.

The simulator gate scripts (`check-simulator-tests.sh`, `scan-mux-overlap.sh`, `seed-surface-tables.sh`, the `check-spec-*.sh` / `fetch-*-spec.sh` spec-maintenance scripts, the `simulators-*.sh` code-quality scans, the shard/coverage/surface-table gates, and `install-firecracker.sh`) moved with the simulators to the [sockerless-cloud repository](https://github.com/e6qu/sockerless-cloud) and run in its CI.

## Architecture / fidelity guards

| Script | What it does | When it runs |
|---|---|---|
| `check-cloud-backend-isolation.sh` | Verifies cloud backends stay stateless: no `BaseServer` lifecycle/query/exec calls, no `Store.Containers` writes — cloud backends operate exclusively through cloud APIs. | pre-commit (Go changes) + CI `test (core)` |
| `check-no-public-cloud-services.sh` | Forbids Cloud Run / Cloud Functions resources granting invoke to `allUsers` / `allAuthenticatedUsers`; long-lived Services must default to `ingress=internal`. | pre-commit (always) |
| `check-port-defaults.sh` | Fails on references to the two obsolete default ports it greps for (canonical default is `:3375`). | pre-commit (always) |

## Lint / hygiene hooks

| Script | What it does | When it runs |
|---|---|---|
| `lint-changed.sh` | Runs golangci-lint per Go module containing changed files (module discovery walks up to the nearest `go.mod`). | pre-commit (Go changes) |
| `strip-ai-attribution.sh` | Strips AI attribution trailers + trailing whitespace from commit messages. | commit-msg hook |
| `check-rebased-on-main.sh` | Fails if the branch is not rebased on `origin/main` (origin/main an ancestor of HEAD), history isn't linear, you're on `main`, or local `main` is out of sync; mirror-remote pushes are exempt. Best-effort offline, authoritative in CI. | pre-push + CI (`rebased-on-main` job) |
| `check-single-open-pr.sh` | Fails if more than one PR is open in the project — all work goes in the single open PR. Best-effort offline, authoritative in CI. | pre-commit + CI (`single-open-pr` job) |
| `check-no-tool-absent-skips.sh` | Fails if a diff adds a test skip for a missing tool/dependency; required tools must be installed by the harness or fail loud. | pre-commit |
| `check-embedded-ui-build-order.sh` | Requires the root production build to create every web bundle before compiling Go binaries that embed those bundles. | pre-commit |
| `check-container-publication.sh` | Locks the main-only immutable short-SHA GHCR publication shape: revision-labelled native ARM64/AMD64 tags, a two-platform manifest, no mutable tags, and release-aware retention that treats coalesced package versions as atomic components. | pre-commit + CI `check-deps` |
| `check-workflow-timeouts.sh` | Requires every ordinary GitHub Actions job to declare a verifiable timeout of at most 15 minutes and validates every matrix timeout value. | pre-commit + `make check-workflow-timeouts` |
| `test-workflow-timeouts.sh` | Exercises the workflow-timeout parser against passing, over-limit, missing, reusable-workflow, and matrix fixtures. | pre-commit + `make check-workflow-timeouts` |
| `update-readme-badges.sh` | Recomputes the badge values in the top-level `README.md` from codebase stats. | pre-push |
| `check-latest-deps.sh` | Fails if any direct Go module require, Terraform provider constraint, or pinned GitHub Action is behind its latest published version (fix with `make upgrade-deps`). | pre-push + CI `lint` (bash and zsh passes) |

## Operator / test helpers

| Script | What it does | When it runs |
|---|---|---|
| `manual-test-real-workloads.sh` | Exercises a sockerless backend with real container workloads via `docker run` against `DOCKER_HOST`, batching probes to keep cloud cold-start latency manageable. | manual |
| `dispatch-gcp-cells.sh` | Operator runbook automation for the GCP runner cells: triggers each cell via `gh` / GitLab REST, polls to a terminal state, prints run URLs + status. | manual (needs `GITHUB_TOKEN` / `GITLAB_TOKEN`) |
| `prune-ghcr-images.sh` | Deletes unrecognized and obsolete GHCR versions while retaining up to the newest 20 complete immutable releases (`tag`, `tag-amd64`, `tag-arm64`) without splitting an indivisible coalesced package-version component. | main-only publication workflow |
| `select-obsolete-container-versions.jq` | Release-aware GHCR retention selector that keeps complete connected release components atomically and stops below the cap when the next component would exceed it. | helper |
