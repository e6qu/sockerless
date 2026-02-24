# gitlabhub

gitlabhub is a GitLab CI Runner coordinator that implements the GitLab Runner API. It accepts `.gitlab-ci.yml` pipelines, manages job dispatch with DAG and stage-based ordering, and coordinates with `gitlab-runner` executors over the standard Runner protocol.

## What it implements

| API | Path | Purpose |
|-----|------|---------|
| Runner registration | `POST /api/v4/runners` | Register/verify/delete runners |
| Job dispatch | `POST /api/v4/jobs/request` | 30s long-poll for pending jobs |
| Job status | `PUT /api/v4/jobs/{id}` | Update job state (running/success/failed) |
| Trace upload | `PATCH /api/v4/jobs/{id}/trace` | Incremental log append |
| Artifacts | `POST/GET /api/v4/jobs/{id}/artifacts` | Upload/download zip artifacts |
| Cache | `GET/PUT /api/v4/projects/{id}/cache` | CI/CD cache by key |
| Git | `/{project}.git/...` | Smart HTTP protocol (clone via go-git) |
| Pipeline submit | `POST /api/v3/gitlabhub/pipeline` | Management API for job input |
| Pipeline status | `GET /api/v3/gitlabhub/pipelines/{id}` | Pipeline + job statuses |

## Pipeline features

- **Stages**: ordered execution (default: build, test, deploy)
- **DAG**: `needs:` overrides stage ordering with direct dependencies
- **Matrix**: `parallel: {matrix: [...]}` expands to cartesian product jobs
- **Rules**: `if:` expressions with `$VAR`, `==`, `!=`, `=~`, `&&`, `||`
- **Extends**: `.template` jobs with deep-merge inheritance
- **Include**: `include: local:` reads files from in-memory git repo
- **Retry**: failed jobs reset to "created" and re-dispatched
- **Resource groups**: exclusive execution lock across jobs
- **Allow failure**: failed jobs don't block dependents
- **Artifacts + dependencies**: zip upload, dotenv reports, inter-job artifact passing
- **DinD**: auto-injects `DOCKER_HOST=tcp://docker:2375` for `docker:dind` services

## Usage

```sh
gitlabhub -addr :8080 -log-level info
```

### Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:8080` | Listen address |
| `-log-level` | `info` | Log level: debug, info, warn, error |

### Environment variables

| Variable | Description |
|----------|-------------|
| `GITLABHUB_MAX_PIPELINES` | Max concurrent pipelines (default 10, returns 429 when exceeded) |
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP endpoint for tracing (no-op if unset) |
| `GLH_TLS_CERT` | TLS certificate file path |
| `GLH_TLS_KEY` | TLS private key file path |

### Submitting a pipeline

```sh
curl -X POST http://localhost:8080/api/v3/gitlabhub/pipeline \
  -H "Content-Type: application/json" \
  -d '{
    "pipeline": "build:\n  script: [\"echo hello\"]\ntest:\n  script: [\"echo test\"]\n  needs: [\"build\"]",
    "image": "golang:1.24"
  }'
```

### Checking pipeline status

```sh
curl http://localhost:8080/api/v3/gitlabhub/pipelines/1
```

## How it works

1. A pipeline is submitted via the management API with raw `.gitlab-ci.yml` YAML
2. gitlabhub creates an in-memory git repo and parses the pipeline
3. Includes are resolved, extends are merged, matrix jobs are expanded
4. The engine dispatches jobs stage-by-stage (or by DAG `needs:`)
5. Runners long-poll `POST /api/v4/jobs/request` and receive job definitions
6. Runners report progress via trace uploads and status updates
7. On job completion, the engine dispatches newly unblocked jobs
8. The pipeline completes when all jobs reach a terminal state

## Project structure

```
gitlabhub/
├── cmd/main.go         Entry point, flag parsing
├── server.go           HTTP server, route registration
├── store.go            In-memory state (runners, projects, pipelines, jobs, git)
├── types.go            Data structures
├── pipeline.go         .gitlab-ci.yml parser and normalizer
├── engine.go           Job dispatch and completion logic
├── runners.go          Runner registration API
├── jobs_request.go     Long-poll job dispatch + response builder
├── jobs_update.go      Job status and trace updates
├── artifacts.go        Artifact upload/download
├── cache.go            Cache endpoints
├── pipeline_api.go     Management API (submit/status/cancel)
├── variables.go        CI variable builder (30+ built-in vars)
├── git.go              In-memory git repo + smart HTTP protocol
├── include.go          include: directive resolution
├── extends.go          extends: template support
├── parallel.go         Matrix job expansion
├── expressions.go      GitLab CI expression evaluator
├── dotenv.go           .env parsing from artifact reports
├── timeout.go          Job timeout enforcement
├── services.go         Service container parsing
├── secrets.go          Project variables API
├── metrics.go          Operational metrics
└── otel.go             OpenTelemetry tracer init
```

## Testing

```sh
cd gitlabhub
go test -v ./...
```

129 unit tests + 17 integration tests covering pipeline parsing, DAG dispatch, expression evaluation, matrix expansion, retry, resource groups, and the full runner coordinator protocol.
