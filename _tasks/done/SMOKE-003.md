# SMOKE-003: GitLab Runner Smoke Test + Memory Backend

**Status:** DONE
**Phase:** 10 — Real Runner Smoke Tests

## Description

Run unmodified `gitlab-runner` (docker executor) against Sockerless with the memory backend, using GitLab CE for job dispatch.

## Changes

- **`smoke-tests/gitlab/docker-compose.yml`** — GitLab CE + Sockerless (backend + frontend) + orchestrator
- **`smoke-tests/gitlab/Dockerfile.backend`** — Builds memory backend binary
- **`smoke-tests/gitlab/Dockerfile.frontend`** — Builds Docker frontend binary
- **`smoke-tests/gitlab/Dockerfile.orchestrator`** — gitlab-runner image with curl/jq and orchestration script
- **`smoke-tests/gitlab/orchestrate.sh`** — Waits for GitLab, gets OAuth token, creates project, pushes CI config, registers runner, triggers pipeline, waits for completion
- **`smoke-tests/gitlab/test-project/.gitlab-ci.yml`** — Minimal CI job: `echo "Hello from Sockerless"`

## Fixes Required

1. **Image name normalization** — Docker SDK sends `fromImage=docker.io/library/alpine`, but runner inspects with `alpine:latest`. Added `docker.io/library/` and `docker.io/` alias storage to `handleImagePull`.
2. **ContainerWait auto-stop** — Runner's helper containers use `OpenStdin: true`, preventing the existing auto-stop. Added auto-stop in `handleContainerWait` for synthetic (memory) containers.

## Result

- `make smoke-test-gitlab` — PASS (pipeline succeeds in ~0s)
