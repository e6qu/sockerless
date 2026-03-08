# bleephub

bleephub is a minimal reimplementation of the internal service API that the official GitHub Actions runner (`actions/runner`) communicates with. It is not a GitHub server — it implements only the runner-to-server wire protocol, which is derived from Azure DevOps (Azure Pipelines).

The official runner does not use the public GitHub REST or GraphQL API. Instead, it talks to five internal services over HTTP, using GHES-style path prefixes. bleephub implements enough of these services for the runner to register, receive a container workflow, execute it through [Sockerless](../)'s Docker API, and report completion.

## What it implements

| Service | Path prefix | Purpose |
|---|---|---|
| Token service | `/_apis/v1/auth/` | JWT exchange (`alg: "none"`, unsigned) |
| Connection data | `/_apis/connectionData` | Service discovery via GUIDs |
| Agent service | `/_apis/v1/Agent/`, `/_apis/v1/AgentPools` | Runner registration, agent pools, credentials |
| Broker | `/_apis/v1/AgentSession/`, `/_apis/v1/Message/` | Session management, 30s message long-poll |
| Run service | `/_apis/v1/AgentRequest/`, `/_apis/v1/FinishJob/` | Job acquire/renew/complete |
| Timeline + logs | `/_apis/v1/Timeline/`, `/_apis/v1/Logfiles/` | Step status tracking, log upload |
| Job submission | `/api/v3/bleephub/submit` | Simplified JSON job input (not part of runner protocol) |

## What it implements beyond the core protocol

- `uses: docker://` actions (container actions with entrypoints)
- Multi-job workflows with `needs:` dependencies
- Matrix strategies (`strategy.matrix`) with fail-fast
- Persistent artifacts (`actions/upload-artifact`, `actions/download-artifact`)
- Secrets injection (job-level and organization-level)
- Expression evaluation (`${{ }}` syntax)
- Concurrency groups with cancel-in-progress
- Output passing between steps and jobs via `$GITHUB_OUTPUT`
- GitHub REST + GraphQL API (repos, orgs, teams, users, issues, PRs) for `gh` CLI
- Git smart HTTP protocol (`go-git`) for `actions/checkout`
- Webhooks (HMAC-SHA256, async delivery with 3 retries)
- GitHub Apps (RS256 JWT, installation tokens, `ghs_`-prefixed)
- OpenTelemetry tracing (optional, OTLP HTTP)
- Embedded React dashboard at `/ui/`

## What it does not implement

- Runner auto-update (`AgentRefreshMessage`)
- V2 broker flow (uses legacy V1 pipelines paths)
- Reusable workflows (`uses: ./.github/workflows/`)
- Composite actions

## How it works

```
┌──────────────────┐     internal API      ┌───────────┐     Docker API     ┌────────────┐
│  actions/runner   │ ◄──────────────────► │  bleephub  │                    │            │
│  (C# binary)     │                       │  (Go)      │                    │ Sockerless │
│                   │     docker exec       │            │                    │            │
│                   │ ─────────────────────►│            │───────────────────►│            │
└──────────────────┘                       └───────────┘                    └────────────┘
```

1. Runner calls `config.sh --url http://bleephub/owner/repo --token ...`
2. bleephub returns registration data, agent pool, credentials
3. Runner starts `run.sh`, creates a session, long-polls `/_apis/v1/Message/` for jobs
4. A job is submitted via `POST /api/v3/bleephub/submit` (simplified JSON)
5. bleephub converts it to the Azure DevOps PipelineAgentJobRequest format and delivers it
6. Runner acquires the job, creates a Docker container through `DOCKER_HOST` (pointing at Sockerless)
7. Runner execs each `run:` step inside the container via `docker exec`
8. Runner reports step status via timeline records and uploads logs
9. Runner calls `FinishJob` — bleephub marks the job as completed

## The job message format

The hardest part of bleephub is the job message builder (`jobs.go`). The runner expects Azure DevOps-format JSON with:

- **TemplateTokens**: A type system for values. Strings are `{"type": 0, "lit": "value"}`, mappings are `{"type": 2, "map": [{"Key": <token>, "Value": <token>}]}`. A JSON object without a `type` field is deserialized as an empty string, causing silent validation failures.

- **PipelineContextData**: Dictionary values use `{"t": 2, "d": [{"k": "key", "v": "value"}]}` format. String values are bare JSON strings.

- **Step format**: Script steps use type `"action"` with `reference: {"type": "script"}` and inputs containing the script content as a TemplateToken MappingToken.

## Usage

```bash
bleephub --addr :80 --log-level info
```

Flags:
- `--addr` — Listen address (default `:5555`). The runner strips non-standard ports from URLs, so use port 80 for integration testing.
- `--log-level` — Log level: `debug`, `info`, `warn`, `error` (default `info`).

### Submitting a job

```bash
curl -X POST http://localhost/api/v3/bleephub/submit \
  -H "Content-Type: application/json" \
  -d '{
    "image": "alpine:latest",
    "steps": [
      {"run": "echo Hello from bleephub"},
      {"run": "uname -a"}
    ]
  }'
```

### Checking job status

```bash
curl http://localhost/api/v3/bleephub/jobs/<jobId>
```

## Integration test

The integration test runs everything in Docker: bleephub + Sockerless backend + Docker frontend + official runner binary (v2.321.0).

```bash
# From the repository root:
make bleephub-test
```

This builds the Docker image (`bleephub/Dockerfile`), starts all services, configures the runner, submits a test job, and verifies it completes successfully.

## Source files

~53 Go source files organized by domain:

| Group | Files | Purpose |
|---|---|---|
| Core protocol | `server.go`, `auth.go`, `agents.go`, `broker.go`, `run_service.go`, `timeline.go` | Runner registration, job delivery, lifecycle |
| Jobs & workflows | `jobs.go`, `workflow.go`, `workflows.go`, `workflows_msg.go`, `matrix.go`, `outputs.go`, `secrets.go`, `expressions.go`, `actions.go`, `artifacts.go` | Multi-job, matrix, secrets, expressions, artifacts |
| GitHub API | `gh_rest.go`, `gh_graphql.go`, `gh_repos_*.go`, `gh_orgs_*.go`, `gh_issues_*.go`, `gh_pulls_*.go`, `gh_teams_rest.go`, `gh_labels_rest.go`, `gh_members_rest.go` | REST + GraphQL for `gh` CLI |
| GitHub Apps | `gh_apps_jwt.go`, `gh_apps_rest.go`, `gh_apps_store.go`, `gh_oauth.go` | JWT auth, installation tokens, OAuth device flow |
| Webhooks | `webhooks.go`, `webhooks_store.go`, `webhooks_payloads.go`, `gh_hooks_rest.go` | HMAC-SHA256 delivery with retry |
| Git | `git_http.go` | Smart HTTP protocol (go-git) |
| Infrastructure | `store.go`, `store_*.go`, `rbac.go`, `metrics.go`, `otel.go`, `handle_mgmt.go`, `ui_embed.go` | State, RBAC, metrics, OTel, dashboard |

## Prior art

[ChristopherHX/runner.server](https://github.com/ChristopherHX/runner.server) (C#, 25 controllers) proves this approach works. bleephub is a from-scratch Go implementation informed by studying the runner source and runner.server's protocol handling, but shares no code with either.
