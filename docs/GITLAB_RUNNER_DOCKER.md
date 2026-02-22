# GitLab Runner (Docker Executor) E2E Tests

End-to-end tests for Sockerless using GitLab Runner's docker executor. A self-contained GitLab CE instance runs in Docker Compose alongside Sockerless. The orchestrator creates a project, pushes a CI pipeline, registers a runner, and monitors execution.

## Architecture

```
GitLab CE (Docker Compose)
  │
  ├── Project with .gitlab-ci.yml
  │
  ▼
GitLab Runner (docker executor)
  │
  ├── host = tcp://sockerless-backend:2375
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

All services run as Docker Compose containers. The backend container runs the simulator + backend + frontend in a single process using `start-backend.sh`.

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

FaaS backends (lambda, gcf, azf) skip tests that require service containers or volume-based artifacts.

## Quick Start

Run all 12 pipelines against the memory backend:

```bash
make e2e-gitlab-memory
```

Run against a specific cloud backend:

```bash
make e2e-gitlab-ecs
make e2e-gitlab-cloudrun
make e2e-gitlab-aca
```

Run a single pipeline:

```bash
cd tests/e2e-live-tests/gitlab-runner-docker
./run.sh --backend ecs --pipeline basic
```

Run against all 7 backends:

```bash
make e2e-gitlab-all
```

## Pipeline Tests

| # | Pipeline | Description | FaaS |
|---|----------|-------------|:----:|
| 1 | basic | Single echo job | Yes |
| 2 | multi-step | Multiple script lines, one job | Yes |
| 3 | env-vars | Custom variables, CI_* variables | Yes |
| 4 | exit-codes | Job with `exit 1`, `allow_failure: true` | Yes |
| 5 | before-after | `before_script` + `script` + `after_script` | Yes |
| 6 | multi-stage | build → test → deploy stages | Yes |
| 7 | artifacts | `artifacts:paths` passing between jobs | SKIP |
| 8 | services | Job with service container | SKIP |
| 9 | large-output | 1000-line stdout, tests log streaming | Yes |
| 10 | parallel-jobs | 3 jobs in same stage running concurrently | Yes |
| 11 | custom-image | Uses `python:3-alpine` | Yes |
| 12 | timeout | `timeout: 30s`, quick job | Yes |

## How It Works

1. `run.sh` builds the Docker images (once) and iterates over pipelines
2. For each pipeline, `docker compose up` starts:
   - **gitlab**: GitLab CE instance
   - **sockerless-backend**: All-in-one container (simulator + backend + frontend)
   - **orchestrator**: Waits for GitLab, creates project, pushes pipeline, registers runner, monitors
3. The orchestrator writes results to `/results/result.json`
4. `run.sh` captures logs and aggregates PASS/FAIL/SKIP

## Live Mode

To run against real cloud infrastructure:

```bash
cd tests/e2e-live-tests/gitlab-runner-docker
./run.sh --backend ecs --mode live
```

In live mode, set cloud credentials and Sockerless env vars per each backend's `README.md` (see the Configuration and Terraform outputs sections).

## Log Files

All output is captured to `tests/e2e-live-tests/logs/`:

- `gitlab-<backend>-<pipeline>-<timestamp>.log` — per-pipeline output
- `summary-gitlab-<backend>-<timestamp>.txt` — aggregated PASS/FAIL/SKIP

## Troubleshooting

**GitLab takes too long to start**: GitLab CE startup can take 2-5 minutes. The orchestrator waits up to 600s (configurable via `GITLAB_TIMEOUT`). On low-resource machines, increase this.

**Pipeline stuck in "pending"**: The runner may not have connected. Check that `SOCKERLESS_HOST` points to the correct frontend address (default: `tcp://sockerless-backend:2375`).

**Runner registration fails**: Both the new runner API (GitLab 16+) and legacy registration are attempted. Ensure the GitLab instance is fully initialized before the orchestrator connects.

**FaaS backend timeout**: FaaS backends use callback-based execution. Ensure the backend container's callback URL is reachable.
