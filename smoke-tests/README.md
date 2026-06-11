# Smoke tests

Docker-based smoke harnesses that boot a cloud simulator + its sockerless backend inside a container and assert the Docker round-trip with the real `docker` CLI. Three surfaces live here:

| Surface | Entry point | What it proves |
|---|---|---|
| Per-backend Docker CLI smoke | `run.sh` + `Dockerfile.{ecs,cloudrun,aca}` | `docker pull / create / start / ps / logs / inspect / stop / rm` (plus `docker run --rm` attach on ECS) against a sim-backed backend |
| GitLab Runner smoke | `gitlab/` (docker compose) | A real GitLab CE instance registers a runner whose Docker executor points at a sim-backed backend and runs a pipeline to completion |
| Live-AWS smoke | [`aws-live/`](aws-live/README.md) | ECS + Lambda against a real AWS account with real github.com / gitlab.com runners — see its own README |

The wider E2E smoke landscape (FaaS Go smokes, GitHub/GitLab runner e2e targets, real-runner arithmetic checks) is catalogued in [`docs/E2E_SMOKE_TESTS.md`](../docs/E2E_SMOKE_TESTS.md).

## `run.sh`

The shared test script baked into every `Dockerfile.*` image as the entrypoint. Selected by the `BACKEND` env var (`ecs` | `cloudrun` | `aca`), it:

1. Starts the matching simulator (`simulator-aws` on `:4566`, `simulator-gcp` on `:4567` + gRPC `:4568`, `simulator-azure` on `:4568`) and waits for `/health`.
2. Exports the backend's sim-pointing env (`SOCKERLESS_ENDPOINT_URL`, per-backend cluster / subscription / project settings, `SOCKERLESS_CALLBACK_URL` for the reverse-agent backends).
3. Starts the backend on `:3375` and waits for `/_ping`.
4. Runs the Docker CLI test sequence with `DOCKER_HOST=tcp://127.0.0.1:3375`, counting pass/fail and dumping sim + backend logs on failure.

Two behaviours worth knowing:

- The simulators execute workloads on the **host Docker daemon** (the socket is mounted in), so the script first pulls `alpine:latest` on the host daemon as well as through the backend.
- The `docker run --rm` attach-path test only runs for ECS — Cloud Run and ACA have no container-level attach primitive.

## Dockerfile variants

`Dockerfile.ecs`, `Dockerfile.cloudrun`, and `Dockerfile.aca` are identical in shape: build the cloud's simulator (`GOWORK=off`, `noui` tag), build the backend binary from `api/` + `agent/` + `backends/core/` + the per-cloud common module + the backend dir, install a static `docker` CLI, and set `run.sh` as the entrypoint with the matching `BACKEND` env.

Run one locally (the host Docker socket mount is required):

```sh
docker build -t smoke-ecs -f smoke-tests/Dockerfile.ecs .
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock smoke-ecs
```

That is exactly what the `smoke` job in `.github/workflows/ci.yml` does for all three backends on every PR.

## Make targets (repo root)

```sh
make smoke-test-ecs               # Docker CLI round-trip smoke, ECS backend
make smoke-test-cloudrun
make smoke-test-aca
make smoke-test-all

make smoke-test-gitlab-ecs        # GitLab CE + runner compose, ECS backend
make smoke-test-gitlab-cloudrun
make smoke-test-gitlab-aca
make smoke-test-gitlab-all
```

Each `smoke-test-{ecs,cloudrun,aca}` target builds the matching `Dockerfile.*` and runs it with the host Docker socket mounted — the same shape as the `smoke` CI job.

Each `smoke-test-gitlab-*` target runs `docker compose up` in `gitlab/` with the chosen `BACKEND`; the `orchestrator` container (`gitlab/orchestrate.sh`) waits for GitLab readiness, creates a project, registers a runner, triggers a pipeline, and gates the compose exit code on pipeline success. `gitlab/start-with-sim.sh` boots the simulator and backend together inside the backend container.

The act-based upstream harness is separate from this directory — it lives under `tests/upstream/act/` (`make upstream-test-act-*`); see [`docs/E2E_SMOKE_TESTS.md`](../docs/E2E_SMOKE_TESTS.md).

## `make` interface of this directory

The local [`Makefile`](Makefile) follows [`docs/MAKEFILE_STANDARD.md`](../docs/MAKEFILE_STANDARD.md): `make test` (and `make build` / `make run`) execute `run.sh` directly on the host — useful when the simulator and backend binaries are already on `PATH`; the Dockerfile path is the self-contained variant CI uses.
