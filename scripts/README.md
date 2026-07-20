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

The four simulator scans run in pre-commit when matching files are touched and unconditionally in the CI simulator-quality job.

| Script | What it does |
|---|---|
| `simulators-deadcode.sh` | `deadcode` per simulator module (aws/gcp/azure, `noui` tag, `GOWORK=off`). |
| `simulators-dupl.sh` | `dupl` over the per-cloud simulator handler sources (≥200 tokens). |
| `simulators-knip.sh` | `knip` over the three simulator dashboard UI packages. |
| `simulators-jscpd.sh` | `jscpd` over the simulator dashboard UIs (≥200 tokens). |

## Lint / hygiene hooks

| Script | What it does | When it runs |
|---|---|---|
| `lint-changed.sh` | Runs golangci-lint per Go module containing changed files (module discovery walks up to the nearest `go.mod`). | pre-commit (Go changes) |
| `strip-ai-attribution.sh` | Strips AI attribution trailers + trailing whitespace from commit messages. | commit-msg hook |
| `check-rebased-on-main.sh` | Fails if the branch is not rebased on `origin/main` (origin/main an ancestor of HEAD), history isn't linear, you're on `main`, or local `main` is out of sync; mirror-remote pushes are exempt. Best-effort offline, authoritative in CI. | pre-push + CI (`rebased-on-main` job) |
| `check-single-open-pr.sh` | Fails if more than one PR is open in the project — all work goes in the single open PR. Best-effort offline, authoritative in CI. | pre-commit + CI (`single-open-pr` job) |
| `check-no-tool-absent-skips.sh` | Fails if a diff adds a test skip for a missing tool/dependency; required tools must be installed by the harness or fail loud. | pre-commit |
| `check-container-publication.sh` | Locks the main-only immutable short-SHA GHCR publication shape: native ARM64/AMD64 tags, a two-platform manifest, no mutable tags, and 20-release retention for every operator image. | pre-commit + CI `check-deps` |
| `update-readme-badges.sh` | Recomputes the badge values in the top-level `README.md` from codebase stats. | pre-push |
| `check-latest-deps.sh` | Fails if any direct Go module require, Terraform provider constraint, or pinned GitHub Action is behind its latest published version (fix with `make upgrade-deps`). | pre-push + CI `lint` (bash and zsh passes) |

## Spec / table maintenance

| Script | What it does | When it runs |
|---|---|---|
| `seed-surface-tables.sh` | Regenerates [`specs/SIM_SURFACE_TABLES/`](../specs/SIM_SURFACE_TABLES/README.md) stubs from registered sim `HandleFunc` patterns; hand-written sections inside `<!-- HAND-WRITTEN BEGIN/END -->` are preserved. | manual, after adding sim routes |
| `fetch-aws-spec.sh` | Vendors/refreshes one AWS Smithy model into [`specs/cloud-api/aws/`](../specs/cloud-api/README.md), pinned to an `aws/aws-sdk-go-v2` commit; rewrites the `SOURCES.md` row. | manual, when adding a service / refreshing pins |
| `fetch-gcp-discovery.sh` | Vendors/refreshes one Google API Discovery document into `specs/cloud-api/gcp/`, pinned by the document's `revision`. | manual |
| `fetch-azure-spec.sh` | Vendors/refreshes one Azure Swagger 2.0 spec into `specs/cloud-api/azure/`, pinned to an `Azure/azure-rest-api-specs` commit. | manual |
| `spec-sources-row.sh` | Shared helper: idempotently upserts one provenance row in a `specs/cloud-api/<cloud>/SOURCES.md` table. Invoked by the three fetch scripts, not directly. | helper |
| `check-spec-violations.sh` | Ratchet gate for runtime spec-shape validation: dedupes a `SOCKERLESS_SPEC_VALIDATE` report and fails on violations missing from `simulators/<cloud>/spec-violation-allowlist.txt` (the list only shrinks; burn-down entries carry a BUG ID — currently only GCP carries an allowlist, with two permanent documented exemptions). | CI sim jobs, after sdk/cli suites |
| `check-spec-freshness.sh` | Reports (never gates) drift between the vendored cloud API spec pins and their upstreams. | manual |

## Setup / CI infrastructure

| Script | What it does | When it runs |
|---|---|---|
| `install-firecracker.sh` | Downloads + installs the pinned Firecracker release binary (x86_64 / aarch64). | CI real-execution jobs |

## Operator / test helpers

| Script | What it does | When it runs |
|---|---|---|
| `manual-test-real-workloads.sh` | Exercises a sockerless backend with real container workloads via `docker run` against `DOCKER_HOST`, batching probes to keep cloud cold-start latency manageable. | manual |
| `dispatch-gcp-cells.sh` | Operator runbook automation for the GCP runner cells: triggers each cell via `gh` / GitLab REST, polls to a terminal state, prints run URLs + status. | manual (needs `GITHUB_TOKEN` / `GITLAB_TOKEN`) |
| `prune-ghcr-images.sh` | Deletes unrecognized and obsolete GHCR versions while retaining the newest 20 complete immutable releases (`tag`, `tag-amd64`, `tag-arm64`) for one container package. | main-only publication workflow |
| `select-obsolete-container-versions.jq` | Release-aware GHCR retention selector used by `prune-ghcr-images.sh`. | helper |
