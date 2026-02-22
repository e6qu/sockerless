# SMOKE-002: Act Smoke Test Against Cloud Simulators

**Status:** DONE
**Phase:** 10 — Real Runner Smoke Tests

## Description

Run unmodified `act` against Sockerless with ECS, Cloud Run, and ACA backends via their simulators.

## Changes

- **`smoke-tests/act/Dockerfile.ecs`** — Builds ECS backend + AWS simulator, starts with `start-with-sim.sh`
- **`smoke-tests/act/Dockerfile.cloudrun`** — Builds Cloud Run backend + GCP simulator
- **`smoke-tests/act/Dockerfile.aca`** — Builds ACA backend + Azure simulator
- **`smoke-tests/act/start-with-sim.sh`** — Starts simulator, waits for health, starts backend with `SOCKERLESS_ENDPOINT_URL`

## Result

- `make smoke-test-act-ecs` — PASS
- `make smoke-test-act-cloudrun` — PASS
- `make smoke-test-act-aca` — PASS
