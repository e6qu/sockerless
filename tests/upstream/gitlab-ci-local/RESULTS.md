# gitlab-ci-local Upstream Test Results

**Tool:** [gitlab-ci-local](https://github.com/firecow/gitlab-ci-local) v4.57.0
**Runner:** Bun 1.1.45 (package manager) + Node.js 22 (runtime)
**Backend:** Memory (WASM sandbox)
**Date:** 2026-02-20

## Summary

| Metric | Count |
|--------|-------|
| PASS   | 15    |
| FAIL   | 0     |
| TOTAL  | 15    |

## Test Cases

| # | Test | Description | Status |
|---|------|-------------|--------|
| 1 | simple-echo | Single echo command | PASS |
| 2 | multi-step | Three sequential echo commands | PASS |
| 3 | env-vars | Global `variables:` with `MY_VAR` | PASS |
| 4 | before-after-scripts | `before_script` / `script` / `after_script` | PASS |
| 5 | multiple-jobs | Two parallel jobs (job-a, job-b) | PASS |
| 6 | working-directory | `pwd` inside container | PASS |
| 7 | exit-code-success | `true` command succeeds | PASS |
| 8 | allow-failure | `false` with `allow_failure: true` | PASS |
| 9 | stages | Two stages (build → test) with ordering | PASS |
| 10 | variable-expansion | `${BASE}-world` expansion | PASS |
| 11 | rules-always | `rules: - when: always` | PASS |
| 12 | artifacts-script | File creation (mkdir + echo > file) | PASS |
| 13 | different-image | Using `debian:bookworm-slim` instead of alpine | PASS |
| 14 | job-level-variables | Job-level `variables:` block | PASS |
| 15 | needs-dependency | `needs:` for job ordering | PASS |

## How gitlab-ci-local Uses Docker

gitlab-ci-local shells out to the `docker` CLI via `execa`:
1. `docker create` — create container from image
2. `docker cp` — copy build scripts into container (pre-start)
3. `docker start` — start the container
4. `docker exec` — run CI scripts
5. `docker rm` — clean up

This exercises the Docker CLI → Docker API path (vs act which uses the Docker SDK).

## Key Fix: Pre-Start Archive Staging

All tests initially failed because gitlab-ci-local calls `docker cp` BEFORE `docker start`.
In the memory backend, the WASM sandbox filesystem only exists after container start.

**Fix:** Added staging directory support in `backends/core/handle_containers.go`:
- `PUT /containers/{id}/archive` extracts to a staging temp dir when no process exists
- `HEAD /containers/{id}/archive` stats the staging dir for pre-start path queries
- `GET /containers/{id}/archive` reads from the staging dir
- On `docker start`, staging dir contents are merged into the process root

## Docker API Endpoints Exercised

- `POST /images/create` (pull)
- `POST /containers/create`
- `PUT /containers/{id}/archive` (docker cp)
- `HEAD /containers/{id}/archive`
- `POST /containers/{id}/start`
- `POST /containers/{id}/exec`
- `POST /exec/{id}/start`
- `GET /exec/{id}/json`
- `DELETE /containers/{id}`
- `GET /_ping`
