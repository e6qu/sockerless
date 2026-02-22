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

## What it does not implement

- Git hosting, pull requests, webhooks, or any GitHub UI
- Public REST/GraphQL API (`/repos/`, `/users/`, etc.)
- `uses:` actions — only `run:` (script) steps work
- Multi-job workflows, matrix strategies, `needs:` dependencies
- Artifacts, caching (`actions/upload-artifact`, `actions/cache`)
- Secrets or encrypted variables
- Runner auto-update (`AgentRefreshMessage`)
- V2 broker flow (uses legacy V1 pipelines paths)
- Multi-runner support (single runner at a time)

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

The integration test runs everything in Docker: bleephub + Sockerless memory backend + Docker frontend + official runner binary (v2.321.0).

```bash
# From the repository root:
make bleephub-test
```

This builds the Docker image (`bleephub/Dockerfile`), starts all services, configures the runner, submits a test job, and verifies it completes successfully.

## Source files

| File | Lines | Purpose |
|---|---|---|
| `cmd/main.go` | ~30 | Entry point, flag parsing |
| `server.go` | ~160 | HTTP server, route registration, middleware |
| `store.go` | ~100 | In-memory state (agents, sessions, jobs) |
| `auth.go` | ~170 | JWT generation, OAuth exchange, connection data |
| `agents.go` | ~180 | Runner registration, agent pools |
| `broker.go` | ~210 | Sessions, 30s message long-poll, job delivery |
| `jobs.go` | ~300 | Job submission, message builder (TemplateToken format) |
| `run_service.go` | ~120 | Job acquire/renew/complete lifecycle |
| `timeline.go` | ~130 | Timeline CRUD, log upload stubs |

## Prior art

[ChristopherHX/runner.server](https://github.com/ChristopherHX/runner.server) (C#, 25 controllers) proves this approach works. bleephub is a from-scratch Go implementation informed by studying the runner source and runner.server's protocol handling, but shares no code with either.
