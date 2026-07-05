# GitHub Actions Runner E2E Tests

End-to-end tests for Sockerless using [act](https://github.com/nektos/act) as a local GitHub Actions runner. Jobs run through a Sockerless backend, which serves the Docker REST API in-process and executes containers on the selected backend (Amazon Elastic Container Service (ECS), AWS Lambda, Google Cloud Run, Google Cloud Functions (GCF), Azure Container Apps (ACA), Azure Functions, or Docker) — there is no separate frontend process.

> **Looking for real github.com + `actions/runner`?** See [`GITHUB_RUNNER_SAAS.md`](./GITHUB_RUNNER_SAAS.md) for the SaaS flow, or [`RUNNERS.md`](./RUNNERS.md) for the cross-platform canonical guide. This doc covers only the `act`-based simulator harness.

## Architecture

```
act (GitHub Actions runner)
  │
  ├── DOCKER_HOST=tcp://backend:3375
  │
  ▼
Sockerless Backend (ecs / lambda / cloudrun / gcf / aca / azf / docker)
  serves the Docker REST API in-process
  │
  ▼
Cloud simulator endpoint (aws / gcp / azure)
```

## Supported Backends

| Backend | Cloud | Simulator | Services | Artifacts |
|---------|-------|-----------|:--------:|:---------:|
| ecs | AWS | simulator-aws | Yes | Yes |
| lambda | AWS | simulator-aws | SKIP | SKIP |
| cloudrun | GCP | simulator-gcp | Yes | Yes |
| gcf | GCP | simulator-gcp | SKIP | SKIP |
| aca | Azure | simulator-azure | Yes | Yes |
| azf | Azure | simulator-azure | SKIP | SKIP |

FaaS backends (lambda, gcf, azf) skip tests that require service containers.

## Quick Start

Run all workflows against a specific cloud backend:

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

Run against all 6 cloud/FaaS backends (ecs, lambda, cloudrun, gcf, aca, azf):

```bash
make e2e-github-all
```

## Workflow Tests

The workflow suite is defined by `ALL_WORKFLOWS` in
`tests/e2e-live-tests/github-runner/run.sh` (the authoritative source), with one
`.yml` per name under `tests/e2e-live-tests/github-runner/workflows/`. It
currently covers 29 workflows:

`basic`, `multi-step`, `env-vars`, `exit-codes`, `multi-job`, `container-action`,
`large-output`, `matrix`, `working-dir`, `outputs`, `shell-features`,
`file-persistence`, `job-outputs`, `concurrent-jobs`, `env-inheritance`,
`github-env`, `step-outputs`, `defaults-shell`, `conditional-steps`,
`multi-job-data`, `services-http`, `container-options`, `container-env-create`,
`diamond-deps`, `matrix-multi`, `conditional-job`, `continue-on-error`,
`timeout-job`, `working-dir-nested`.

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

**act hangs on container pull**: Ensure the backend's Docker API is reachable at the configured address. Check `DOCKER_HOST` is set correctly.

**Workflow fails with "image not found"**: The backend must be able to resolve the same cloud-shaped image references it would use in production. With simulators, make sure the simulator registry/cloud slice is reachable through the configured endpoint and the required local images or registry mirrors exist. In live mode, ensure the cloud environment has registry access.

**FaaS backend timeout**: FaaS backends have a callback-based execution model. Ensure `SOCKERLESS_CALLBACK_URL` is reachable from the backend process.
