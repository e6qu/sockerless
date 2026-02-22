# SMOKE-004: GitLab Runner Smoke Test Against Cloud Simulators

**Status:** DONE
**Phase:** 10 — Real Runner Smoke Tests

## Description

Run unmodified `gitlab-runner` (docker executor) against Sockerless with ECS, Cloud Run, and ACA backends via their simulators.

## Changes

- **`smoke-tests/gitlab/docker-compose.ecs.yml`** — ECS backend override
- **`smoke-tests/gitlab/docker-compose.cloudrun.yml`** — Cloud Run backend override
- **`smoke-tests/gitlab/docker-compose.aca.yml`** — ACA backend override
- **`smoke-tests/gitlab/Dockerfile.backend-ecs`** — Builds AWS simulator + ECS backend
- **`smoke-tests/gitlab/Dockerfile.backend-cloudrun`** — Builds GCP simulator + Cloud Run backend
- **`smoke-tests/gitlab/Dockerfile.backend-aca`** — Builds Azure simulator + ACA backend
- **`smoke-tests/gitlab/start-with-sim.sh`** — Starts simulator + backend in one container

## Fixes Required

1. **Image name normalization for cloud backends** — All 6 cloud backends had their own `ImagePull` handlers that lacked the `docker.io/library/` alias fix. Extracted `StoreImageWithAliases()` into `backends/core/handle_images.go` and replaced per-backend storage logic.
2. **ContainerWait auto-stop scope** — Restricted auto-stop to `c.Driver == "memory"` to prevent cloud backends from being affected (their lifecycle is managed by `pollExecutionExit`).
3. **Cloud Run Job re-creation** — Simulator auto-completes executions after 3s, `pollExecutionExit` stops the container, then runner restarts it for cleanup. `handleContainerStart` now deletes existing Cloud Run Job before creating a new one.

## Result

- `make smoke-test-gitlab-ecs` — PASS (~45s)
- `make smoke-test-gitlab-cloudrun` — PASS (~20s)
- `make smoke-test-gitlab-aca` — PASS (~20s)
