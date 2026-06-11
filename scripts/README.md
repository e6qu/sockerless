# Scripts

Repo-wide guard scripts, code-quality scans, and operator helpers. Most are wired into the pre-commit / commit-msg / pre-push hooks defined in [`.pre-commit-config.yaml`](../.pre-commit-config.yaml) and/or CI jobs in [`.github/workflows/ci.yml`](../.github/workflows/ci.yml); the rest are run by hand.

Every script must be shellcheck-clean and run under both bash and zsh on macOS and Linux — enforced at pre-push (`shellcheck -x -s bash scripts/*.sh`, `bash -n`, `zsh -n`) and again in the CI lint job.

## Architecture / fidelity guards

| Script | What it does | When it runs |
|---|---|---|
| `check-cloud-backend-isolation.sh` | Verifies cloud backends stay stateless: no `BaseServer` lifecycle/query/exec calls, no `Store.Containers` writes — cloud backends operate exclusively through cloud APIs. | pre-commit (Go changes) + CI `test (core)` |
| `check-simulator-tests.sh` | Simulator testing contract: every new `Register(...)` / `RegisterVersioned(...)` operation under `simulators/<cloud>/` must be referenced by an SDK / CLI / terraform test in the same commit (opt-outs via `simulators/<cloud>/tests-exempt.txt`). | pre-commit (always) |
| `check-simulator-coverage-matrix.sh` | Keeps [`specs/SIM_TEST_COVERAGE_MATRIX.md`](../specs/SIM_TEST_COVERAGE_MATRIX.md) in lockstep with [`specs/SIM_SURFACE_TABLES/`](../specs/SIM_SURFACE_TABLES/README.md). | pre-commit (always) + CI `build-gates` |
| `check-cli-shard-coverage.sh` | Asserts every AWS CLI test function matches exactly one of the CI shard `-run` regexes — a test matching no shard would silently never run. | pre-commit (always) |
| `check-no-public-cloud-services.sh` | Forbids Cloud Run / Cloud Functions resources granting invoke to `allUsers` / `allAuthenticatedUsers`; long-lived Services must default to `ingress=internal`. | pre-commit (always) |
| `check-port-defaults.sh` | Fails on references to the two obsolete default ports it greps for (canonical default is `:3375`). | pre-commit (always) |
| `scan-mux-overlap.sh` | Enumerates every sim route registration and flags wildcard patterns that shadow another service's literal path. Warn mode by default; `--gate` exits 1 on un-allowlisted overlap. | pre-commit (always, warn mode) / manual `--gate` |
| `mux-overlap-allowlist.txt` | Allowlist consumed by `scan-mux-overlap.sh` — one tab-separated `<pattern1> <pattern2> <justification>` per line (currently empty). | data file |

## Code-quality scans

All eight run in pre-commit when the matching files are touched, and unconditionally in the CI `bleephub-quality` / `simulators-quality` jobs.

| Script | What it does |
|---|---|
| `bleephub-deadcode.sh` | `deadcode` (unreachable Go functions) over `bleephub/`. |
| `bleephub-dupl.sh` | `dupl` Go copy-paste detection over bleephub (≥200 tokens). |
| `bleephub-knip.sh` | `knip` dead-TS-exports check over the bleephub UI package. |
| `bleephub-jscpd.sh` | `jscpd` TS copy-paste detection over the bleephub UI (≥200 tokens). |
| `simulators-deadcode.sh` | `deadcode` per simulator module (aws/gcp/azure, `noui` tag, `GOWORK=off`). |
| `simulators-dupl.sh` | `dupl` over the per-cloud simulator handler sources (≥200 tokens). |
| `simulators-knip.sh` | `knip` over the three simulator dashboard UI packages. |
| `simulators-jscpd.sh` | `jscpd` over the simulator dashboard UIs (≥200 tokens). |

## Lint / hygiene hooks

| Script | What it does | When it runs |
|---|---|---|
| `lint-changed.sh` | Runs golangci-lint per Go module containing changed files (module discovery walks up to the nearest `go.mod`). | pre-commit (Go changes) |
| `strip-ai-attribution.sh` | Strips AI attribution trailers + trailing whitespace from commit messages. | commit-msg hook |
| `check-rebased.sh` | Fails the push if the branch is not rebased on `origin/main`; mirror-remote pushes are exempt. | pre-push |
| `update-readme-badges.sh` | Recomputes the badge values in the top-level `README.md` from codebase stats. | pre-push |
| `check-latest-deps.sh` | Fails if any direct Go module require, Terraform provider constraint, or pinned GitHub Action is behind its latest published version (fix with `make upgrade-deps`). | pre-push + CI `lint` (bash and zsh passes) |

## Spec / table maintenance

| Script | What it does | When it runs |
|---|---|---|
| `seed-surface-tables.sh` | Regenerates [`specs/SIM_SURFACE_TABLES/`](../specs/SIM_SURFACE_TABLES/README.md) stubs from registered sim `HandleFunc` patterns; hand-written sections inside `<!-- HAND-WRITTEN BEGIN/END -->` are preserved. | manual, after adding sim routes |
| `update-github-openapi.sh` | Refreshes the vendored GitHub OpenAPI description (`bleephub/testdata/github-openapi.json.gz`) used by bleephub's hermetic API-definition fidelity test. | manual |

## Setup / CI infrastructure

| Script | What it does | When it runs |
|---|---|---|
| `install-firecracker.sh` | Downloads + installs the pinned Firecracker release binary (x86_64 / aarch64). | CI real-execution jobs |

## Operator / test helpers

| Script | What it does | When it runs |
|---|---|---|
| `manual-test-real-workloads.sh` | Exercises a sockerless backend with real container workloads via `docker run` against `DOCKER_HOST`, batching probes to keep cloud cold-start latency manageable. | manual |
| `dispatch-gcp-cells.sh` | Operator runbook automation for the GCP runner cells: triggers each cell via `gh` / GitLab REST, polls to a terminal state, prints run URLs + status. | manual (needs `GITHUB_TOKEN` / `GITLAB_TOKEN`) |
