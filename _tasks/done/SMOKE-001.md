# SMOKE-001: Act Smoke Test + Memory Backend

**Status:** DONE
**Phase:** 10 — Real Runner Smoke Tests

## Description

Run unmodified `act` (nektos/act for GitHub Actions) against Sockerless with the memory backend, proving it completes a CI workflow.

## Changes

- **`smoke-tests/act/workflows/test.yml`** — Minimal GitHub Actions workflow with alpine container and echo steps
- **`smoke-tests/act/Dockerfile`** — All-in-one Docker container: builds Sockerless (backend + frontend), installs act, runs the workflow
- **`smoke-tests/act/run.sh`** — Starts backend and frontend, waits for health, runs `act push` with `DOCKER_HOST`

## Result

- `make smoke-test-act` passes — act completes the workflow against memory backend
