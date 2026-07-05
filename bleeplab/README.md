# bleeplab

bleeplab is a self-contained Go reimplementation of the slice of GitLab's server-side surface that a real `gitlab-runner` (docker executor) and a CI orchestrator exercise — enough for the official runner binary to register, poll for jobs, stream build logs, and upload/download artifacts against a local process exactly as it would against `gitlab.com` or a self-managed GitLab instance.

It is the **GitLab analog of [`bleephub`](../bleephub/README.md)**, the GitHub control-plane simulator. The two are structural siblings: same object-store-backed git, same embedded SPA pattern, same "fidelity, not fakery" contract. Where bleephub speaks GHES `/_apis/` + `/api/v3/`, bleeplab speaks the GitLab runner API + `/api/v4/`.

**Fidelity, not fakery.** The runner authenticates and polls exactly as it does against real GitLab; bleeplab differs only in **coordinates** — the base URL and tokens. It implements the real wire shapes the runner consumes and never special-cases sockerless. This is the same rule the whole project lives by; see the repo root [`AGENTS.md`](../AGENTS.md).

## Reference adaptor

bleeplab is paired with the external GitLab-compatible tool that drives it. Anything that tool does against `gitlab.com` must work against bleeplab.

| Adaptor | Version | What it proves |
|---|---|---|
| [`gitlab-runner`](https://docs.gitlab.com/runner/) (official binary, docker executor) | v18.11.3 (pinned in the [`Dockerfile`](Dockerfile)) | The runner API end-to-end — register/verify, `POST /api/v4/jobs/request` long-poll, `PATCH .../trace` log streaming, `PUT .../jobs/:id` completion, artifact upload/download. |
| [Smart-HTTP git](https://git-scm.com/docs/http-protocol) (`go-git`) | git 2.40+ | `git clone` / `git fetch` of a project repo over `http://<host>/{namespace}/{project}.git` — what the runner's git step does before a job. |
| [GitLab runner API](https://docs.gitlab.com/ee/api/runners.html) / [CI/CD YAML](https://docs.gitlab.com/ee/ci/yaml/) | current | The authoritative reference for the runner wire shapes and the `.gitlab-ci.yml` subset bleeplab models (stages, `image`, `script`/`before_script`/`after_script`, `services`, `variables`, `artifacts`, `dependencies`). |

## How it works

bleeplab is the **control plane**; the sockerless backend + cloud simulator are the **data plane**. A real `gitlab-runner` sits between them:

1. The runner registers against bleeplab (`/api/v4/runners`) and long-polls `POST /api/v4/jobs/request`.
2. A pipeline is created for a project (from its committed `.gitlab-ci.yml`); bleeplab parses the CI config ([`ciyaml.go`](ciyaml.go)) into ordered stages and per-stage jobs, and enqueues the first stage.
3. bleeplab hands a queued job to the polling runner as a GitLab-shaped `jobResponse` (image, script steps, services, variables, git info, artifact/dependency specs).
4. The runner's **docker executor** dispatches the job + helper containers through a `--docker-host` that points at a **sockerless backend** — so the containers actually run on the cloud (ECS / Cloud Run / ACA / …) via the simulator, exactly as they would with a cloud `DOCKER_HOST` against real GitLab. This is the *runner-as-cloud-task* data plane the live GitLab cells use; the per-cloud mapping is in [`specs/CLOUD_RESOURCE_MAPPING.md`](../specs/CLOUD_RESOURCE_MAPPING.md).
5. The job clones its repo over smart-HTTP from bleeplab's git storage, runs, streams its trace back via `PATCH .../trace`, uploads artifacts, and reports completion via `PUT .../jobs/:id`. bleeplab advances the pipeline to the next stage.

The `externalURL` coordinate (`BLEEPLAB_EXTERNAL_URL`) matters here: the `git_info.repo_url` handed to a job must be reachable **from the job/helper container**, which is a different network vantage point than the runner process — so it is a distinct coordinate from the control-plane API URL. Runner hurdles like this (and their fixes) are catalogued in [`docs/RUNNERS.md`](../docs/RUNNERS.md).

## What it implements

All under one binary on one port (`:8929` by default).

**Runner-facing API** (`gitlab-runner` polls these):
- `POST /api/v4/runners/verify`, `POST /api/v4/runners`, `DELETE /api/v4/runners` — register / verify / unregister.
- `POST /api/v4/jobs/request` — long-poll job claim (201 with a job, or 204 when the queue is empty).
- `PUT /api/v4/jobs/{id}` — job completion (success/failed).
- `PATCH /api/v4/jobs/{id}/trace` — incremental build-log streaming.
- `POST /api/v4/jobs/{id}/artifacts`, `GET /api/v4/jobs/{id}/artifacts` — artifact upload / dependency download.

**Control-plane API** (the orchestrator / test harness drives these):
- `POST /api/v4/user/runners` — mint a runner registration token.
- `POST /api/v4/projects`, `POST /api/v4/projects/{id}/repository/commits` — create a project and commit its `.gitlab-ci.yml` + files.
- `POST /api/v4/projects/{id}/pipeline`, `GET .../pipelines`, `GET .../pipelines/{pid}`, `GET .../pipelines/{pid}/jobs`, `GET .../jobs/{jid}/trace` — trigger a pipeline and read pipeline/job/trace status.

**Git smart-HTTP** — `clone` / `fetch` / `push` on dynamic `/{namespace}/{project}.git/...` paths ([`git.go`](git.go)).

**Internal read-only surface** (`/internal/*`) — projections the embedded SPA consumes for its dashboard (status, projects, pipelines, jobs, runners, storage). Resource detail still comes from the public `/api/v4` surface; the internal routes exist only where there is no clean public-API equivalent.

**Health** — `GET /health`.

## Storage backend

bleeplab stores git repositories and CI artifacts in an **object store first**, exactly like bleephub — the backend is selected by environment, with the same precedence (S3-compatible object store → filesystem directory → in-memory). Git repos are `go-git` `Storer`s ([`git_storage.go`](git_storage.go)); artifacts go through an `artifactStore` ([`artifacts.go`](artifacts.go)); both share the S3 client ([`s3fs.go`](s3fs.go)). The object-store model and its rationale are described once, for both simulators, in bleephub's [Persistence](../bleephub/README.md#persistence) section — bleeplab follows it rather than restating it.

## Environment variables

| Variable | Purpose | Default |
|---|---|---|
| `BLEEPLAB_EXTERNAL_URL` | Base URL for `git_info.repo_url` handed to jobs — must be reachable from the job container. | request `Host` |
| `BLEEPLAB_S3_ENDPOINT` / `BLEEPLAB_S3_BUCKET` / `BLEEPLAB_S3_PREFIX` | S3-compatible object store for git + artifacts. `BUCKET` set ⇒ object-store mode. | — (in-memory) |
| `BLEEPLAB_GIT_DIR` | Filesystem directory for git repos when not using S3. | — (in-memory) |
| `BLEEPLAB_ARTIFACTS_DIR` | Filesystem directory for artifacts when not using S3. | — (in-memory) |
| `BLEEPLAB_BACKEND` | Selects the backend + cloud sim in the integration harness (`ecs` → AWS sim, `cloudrun` → GCP sim, …). | — |

## Quick start

bleeplab uses the [standard component Makefile](../docs/MAKEFILE_STANDARD.md) (`make/go-app.mk`), so it builds, runs, and tests with the same targets as every other component.

```bash
# Build + run on :8929 (embeds the UI when ui/packages/bleeplab/dist/ exists,
# else falls back to a -tags noui binary).
cd bleeplab
make run                     # build + run in the foreground on :8929

# Or build and run explicitly:
make build                   # → ./bleeplab
./bleeplab -addr :8929 -log-level debug
```

Nothing is required in the environment for the in-memory default — a runner can register and run jobs immediately. Point `BLEEPLAB_S3_*` or `BLEEPLAB_GIT_DIR` at durable storage to keep repos and artifacts across restarts.

### bleeplab UI

The dashboard is a Vite SPA embedded via Go `embed` (`ui_embed.go`; `-tags noui` drops it, `ui_noembed.go`). For live UI development, run the Go server headless and the Vite dev server against it:

```bash
make dev                     # Go server (no UI) + UI dev server in parallel
```

## Integration tests

```bash
# Go unit tests (in-process, in-memory storage — no Docker needed).
make test

# Full docker-executor harness: a REAL gitlab-runner registers against
# bleeplab and dispatches jobs through a sockerless backend + cloud sim.
# Self-contained (builds the sims, backends, agent, bleeplab, and pulls the
# pinned gitlab-runner into one image); pick the backend via BLEEPLAB_BACKEND.
docker build -f bleeplab/Dockerfile -t bleeplab-runner-int:local .
docker run --rm -v /var/run/docker.sock:/var/run/docker.sock \
  -e BLEEPLAB_BACKEND=ecs bleeplab-runner-int:local
```

The harness ([`test/run-integration.sh`](test/run-integration.sh)) exercises the whole runner-as-cloud-task data plane end to end: clone + compile, artifacts across stages, and `services:` sidecars over the network pod. Set `BLEEPLAB_HOLD=1` to hold the stack (bleeplab `:8929`, backend `:3375`) for inspection on failure. Building the runner image reliably requires `docker build --load` — see the harness note in [`docs/RUNNERS.md`](../docs/RUNNERS.md).

## Source layout

| File | Responsibility |
|---|---|
| [`server.go`](server.go) | HTTP mux, route table, middleware, lifecycle. |
| [`store.go`](store.go) | In-memory control-plane state (runners, projects, pipelines, jobs) + the runner-API wire shapes. |
| [`runner_api.go`](runner_api.go) | The runner-facing handlers (`/api/v4/runners`, `/api/v4/jobs/*`). |
| [`projects_api.go`](projects_api.go) | Control-plane handlers (projects, commits, pipeline trigger + status). |
| [`ciyaml.go`](ciyaml.go) | `.gitlab-ci.yml` parsing → stages + per-job execution spec. |
| [`pipeline.go`](pipeline.go) | Pipeline/stage advancement + job enqueueing. |
| [`artifacts.go`](artifacts.go) | CI artifact upload/download over the object-store-backed `artifactStore`. |
| [`git.go`](git.go), [`git_storage.go`](git_storage.go), [`s3fs.go`](s3fs.go) | Smart-HTTP git + the object-store / filesystem / in-memory storage backend. |
| [`internal_api.go`](internal_api.go) | Read-only `/internal/*` projections for the embedded UI. |
| [`cmd/main.go`](cmd/main.go) | Binary entrypoint (`-addr`, `-log-level`). |

## See also

- [`bleephub/README.md`](../bleephub/README.md) — the GitHub control-plane sibling and the model this README follows.
- [`docs/RUNNERS.md`](../docs/RUNNERS.md) — the GitHub-runner and GitLab-runner hurdle catalogue (closed bugs + predicted next hurdles).
- [`specs/CLOUD_RESOURCE_MAPPING.md`](../specs/CLOUD_RESOURCE_MAPPING.md) — how sockerless maps Docker/CI primitives onto each cloud (the runner-as-cloud-task data plane).
- [`ARCHITECTURE.md`](../ARCHITECTURE.md) — the whole-project architecture bleeplab plugs into.
- [`backends/README.md`](../backends/README.md) — the sockerless backends the runner's docker executor targets.
- [`docs/MAKEFILE_STANDARD.md`](../docs/MAKEFILE_STANDARD.md) — the shared component Makefile bleeplab uses.
