# sockerless (CLI)

Zero-dependency CLI for managing Sockerless contexts and backend server lifecycle. Stores configuration in `~/.sockerless/` (override with `SOCKERLESS_HOME`). Talks to backends over their Docker REST API + management endpoints.

## Reference adaptors

The CLI is itself an adaptor against running Sockerless backends and against the user's shell. Its surface is small and validated by:

| Adaptor | What it proves |
|---|---|
| The user's terminal | `sockerless context`, `sockerless server`, `sockerless ps`, `sockerless status` — typed commands round-trip without surprises. |
| The backend management HTTP API | `/v1/health`, `/v1/info`, `/v1/containers`, `/v1/metrics` on each [backend](../../backends/) at the configured `addr`. The CLI is a thin HTTP client; backends are the truth. |
| The [Docker REST API v1.44](https://docs.docker.com/engine/api/v1.44/) | `sockerless ps` parses `/containers/json`; `sockerless metrics` reads Prometheus over HTTP. |
| `*_test.go` files in this package | Behaviour-level unit tests for context CRUD, config-migrate, simulator add/remove, status formatting. |

This means `sockerless` does **not** speak any cloud API directly. Configuration *describes* cloud backends; the cloud calls happen inside the backend processes.

## Validation

| Test path | What runs | Last green |
|---|---|---|
| `cmd/sockerless/*_test.go` | Go unit tests for context store, config migrate, simulator manager, paths. | 2026-05-16 |
| `make cmd/sockerless/test` | Leaf-Makefile suite per [`docs/MAKEFILE_STANDARD.md`](../../docs/MAKEFILE_STANDARD.md). | 2026-05-16 |
| Manual round-trip | `sockerless context create … && sockerless server start && sockerless status` against a built backend binary. Discipline: real binary, real terminal output — see [`manual-test`](../../.claude/skills/manual-test/SKILL.md). | continuous |

## Wiring

```sh
# Build
make cmd/sockerless/build

# Initialise a context backed by ECS + the AWS sim
sockerless context create ecs-dev --backend ecs --simulator sim-aws \
  --set SOCKERLESS_ECS_CLUSTER=sockerless \
  --set SOCKERLESS_ECS_SUBNETS=subnet-abc123 \
  --set SOCKERLESS_ECS_EXECUTION_ROLE_ARN=arn:aws:iam::000000000000:role/exec

# Start the backend
sockerless server start

# Drive workloads via the Docker frontend
export DOCKER_HOST=tcp://localhost:3375
docker run --rm alpine echo hello
```

### Environment variables

| Variable | Description |
|---|---|
| `SOCKERLESS_HOME` | Override config directory (default `~/.sockerless`). |
| `SOCKERLESS_CONTEXT` | Override active context name. |
| `SOCKERLESS_CONFIG` | Override config file path (default `~/.sockerless/config.yaml`). |

## Commands

### `context` — manage backend contexts

```sh
sockerless context create myctx --backend ecs
sockerless context create aws-dev --backend ecs --set AWS_REGION=us-east-1
sockerless context create sim-ecs --backend ecs --simulator sim-aws
sockerless context list
sockerless context show myctx
sockerless context use myctx
sockerless context current
sockerless context delete myctx
sockerless context reload
```

Flags for `context create`:

| Flag | Description |
|---|---|
| `--backend` | Backend type (required): `ecs`, `lambda`, `cloudrun`, `gcf`, `aca`, `azf`, `docker` |
| `--addr` | Server address (e.g. `http://localhost:3375`) |
| `--simulator` | Simulator name (from `config.yaml` simulators section) |
| `--set KEY=VALUE` | Set environment variable (repeatable) |

### `server` — lifecycle

```sh
sockerless server start
sockerless server stop
sockerless server restart
```

Flags for `server start`:

| Flag | Default | Description |
|---|---|---|
| `--backend-bin` | `sockerless-backend-{type}` | Path to backend binary |
| `--addr` | `:3375` | Listen address (Docker API + management) |

### Inspection

```sh
sockerless status      # Backend health + uptime + container count
sockerless ps          # Table: ID, NAME, IMAGE, STATE, POD
sockerless metrics     # Prometheus metrics from the backend
sockerless check       # Backend self-checks with per-check pass/fail
sockerless resources list      # Cloud resources owned by this backend
sockerless resources orphaned  # Resources without a matching sockerless owner-link
sockerless resources cleanup   # Reap orphans
```

### `simulator` — manage local cloud simulators

```sh
sockerless simulator list
sockerless simulator add sim-aws --cloud aws --port 5111
sockerless simulator add sim-gcp --cloud gcp --port 5112 --grpc-port 5113
sockerless simulator remove sim-aws
```

Flags for `simulator add`:

| Flag | Description |
|---|---|
| `--cloud` | Cloud type (required): `aws`, `gcp`, `azure` |
| `--port` | Listen port (0 = auto) |
| `--grpc-port` | gRPC port (GCP only) |
| `--log-level` | Log level |

### `config migrate` — convert JSON contexts to `config.yaml`

```sh
sockerless config migrate          # preview to stdout
sockerless config migrate --write  # write to config.yaml
```

Reads existing `contexts/*/config.json` files and converts them to the unified `config.yaml` format. Detects simulator usage from `SOCKERLESS_ENDPOINT_URL` and creates simulator entries automatically.

### `version`

```sh
sockerless version
```

## Configuration layout

```
~/.sockerless/
├── config.yaml          Unified configuration (environments + simulators)
├── active               Active context/environment name
├── contexts/            Legacy JSON contexts (still supported)
│   └── {name}/
│       └── config.json  Backend type, address, env vars
└── run/
    └── {name}/
        └── backend.pid  Server process ID
```

## Unified configuration

The preferred way to configure Sockerless is via `~/.sockerless/config.yaml`. This file defines named environments (backend configurations) and optional simulator definitions in a single place.

```yaml
simulators:
  sim-aws:
    cloud: aws
    port: 5111
  sim-gcp:
    cloud: gcp
    port: 5112
    grpc_port: 5113

environments:
  ecs-sim:
    backend: ecs
    simulator: sim-aws
    aws:
      region: us-east-1
      ecs:
        cluster: sockerless
        subnets: [subnet-abc123]
        execution_role_arn: arn:aws:iam::123456789012:role/ecsExec
  cloudrun-dev:
    backend: cloudrun
    simulator: sim-gcp
    gcp:
      project: my-project
      cloudrun:
        region: us-central1
  aca-prod:
    backend: aca
    azure:
      subscription_id: 00000000-0000-0000-0000-000000000000
      aca:
        resource_group: sockerless-rg
        environment: sockerless
        location: eastus
    common:
      agent_image: myregistry.azurecr.io/sockerless-agent:latest
```

Full schema: [`specs/CONFIG.md`](../../specs/CONFIG.md).

**Priority order:** `config.yaml` environment values → context env vars (legacy) → process environment variables → defaults.

Use `sockerless config migrate` to convert existing JSON contexts to `config.yaml` format. Legacy `contexts/*/config.json` files continue to work — the CLI checks `config.yaml` first, then falls back to JSON contexts.

## Known issues

None open. CLI evolution tracked alongside backend evolution in [`PLAN.md`](../../PLAN.md).

## What's out of scope

- Cloud-API operations. The CLI configures backends; it doesn't itself call AWS / GCP / Azure. Use `aws` / `gcloud` / `az` for cloud-side observation.
- Container exec / attach UX. Use the Docker frontend (`docker exec`, `docker attach`) — the backend serves the Docker REST API.
- Multi-machine orchestration. Use `cmd/sockerless-admin` instead — that surface is dedicated to topology.

## Project structure

```
cmd/sockerless/
├── main.go            Command dispatcher
├── configfile.go      Unified config.yaml types and I/O
├── config_migrate.go  JSON context → config.yaml migration
├── simulator.go       Simulator list/add/remove commands
├── context.go         Context CRUD commands
├── server.go          Server start/stop/restart
├── status.go          Health status display
├── ps.go              Container listing
├── metrics.go         Metrics display
├── resources.go       Cloud resource management
├── check.go           Health check runner
├── client.go          HTTP management client helpers
└── paths.go           Config directory and path resolution
```

See also: [`backends/*/README.md`](../../backends/) for what each backend type configures, [`cmd/sockerless-admin/README.md`](../sockerless-admin/README.md) for the topology / multi-backend orchestration surface, [`specs/CONFIG.md`](../../specs/CONFIG.md) for the full configuration schema.
