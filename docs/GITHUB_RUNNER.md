# GitHub Actions Runner E2E Tests

End-to-end tests for Sockerless using [act](https://github.com/nektos/act) as a local GitHub Actions runner. Jobs run through the Sockerless Docker frontend, which delegates container execution to the selected backend (memory, ECS, Lambda, Cloud Run, GCF, ACA, or Azure Functions).

## Architecture

```
act (GitHub Actions runner)
  │
  ├── DOCKER_HOST=tcp://frontend:2375
  │
  ▼
Sockerless Frontend (Docker API)
  │
  ▼
Sockerless Backend (memory / ecs / lambda / cloudrun / gcf / aca / azf)
  │
  ▼
Cloud Simulator (aws / gcp / azure)  ← simulator mode only
```

## Supported Backends

| Backend | Cloud | Simulator | Services | Artifacts |
|---------|-------|-----------|:--------:|:---------:|
| memory | — | — | — | — |
| ecs | AWS | simulator-aws | Yes | Yes |
| lambda | AWS | simulator-aws | SKIP | SKIP |
| cloudrun | GCP | simulator-gcp | Yes | Yes |
| gcf | GCP | simulator-gcp | SKIP | SKIP |
| aca | Azure | simulator-azure | Yes | Yes |
| azf | Azure | simulator-azure | SKIP | SKIP |

FaaS backends (lambda, gcf, azf) skip tests that require service containers.

## Quick Start

Run all 12 workflows against the memory backend:

```bash
make e2e-github-memory
```

Run against a specific cloud backend:

```bash
make e2e-github-ecs
make e2e-github-cloudrun
make e2e-github-aca
```

Run a single workflow:

```bash
# Inside Docker (after building)
docker run --rm -e BACKEND=ecs sockerless-e2e-github --backend ecs --workflow basic
```

Run against all 7 backends:

```bash
make e2e-github-all
```

## Workflow Tests

| # | Workflow | Description | FaaS |
|---|----------|-------------|:----:|
| 1 | basic | Single container job, echo | Yes |
| 2 | multi-step | Multiple `run:` steps | Yes |
| 3 | env-vars | Job-level and step-level `env:`, CI variables | Yes |
| 4 | exit-codes | Failing step with `continue-on-error: true` | Yes |
| 5 | multi-job | 2 sequential jobs with `needs:` | Yes |
| 6 | container-action | `uses: docker://alpine` with entrypoint | Yes |
| 7 | services | Job with `services:` (busybox) | SKIP |
| 8 | large-output | 1000-line stdout, tests log streaming | Yes |
| 9 | matrix | `strategy.matrix` with 2 values | Yes |
| 10 | custom-image | `container: python:3-alpine` | Yes |
| 11 | working-dir | Steps with `working-directory:` | Yes |
| 12 | outputs | Step outputs via `$GITHUB_OUTPUT` | Yes |

## Live Mode

To run against real cloud infrastructure instead of simulators:

```bash
# Set cloud credentials + Sockerless env vars per each backend's README.md
export AWS_ACCESS_KEY_ID=...
export SOCKERLESS_ECS_CLUSTER=...
# etc.

./run.sh --backend ecs --mode live
```

In live mode, no simulator is started. The backend connects to real cloud APIs using credentials from the environment.

## Log Files

All output is captured to `tests/e2e-live-tests/logs/`:

- `github-<backend>-<workflow>-<timestamp>.log` — per-workflow output
- `summary-github-<backend>-<timestamp>.txt` — aggregated PASS/FAIL/SKIP

## Troubleshooting

**act hangs on container pull**: Ensure the Docker frontend is reachable at the configured address. Check `DOCKER_HOST` is set correctly.

**Workflow fails with "image not found"**: The backend must be able to pull container images. In simulator mode, image pulls are simulated. In live mode, ensure the cloud environment has internet access.

**FaaS backend timeout**: FaaS backends have a callback-based execution model. Ensure `SOCKERLESS_CALLBACK_URL` is reachable from the backend process.
