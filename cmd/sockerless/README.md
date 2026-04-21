# sockerless

CLI tool for managing Sockerless contexts and server lifecycle. Zero external dependencies.

## Overview

The CLI manages named contexts (backend configurations), starts and stops backend servers, and provides commands for inspecting running services. Configuration is stored in `~/.sockerless/` (override with `SOCKERLESS_HOME`).

## Building

```sh
cd cmd/sockerless
go build -o sockerless .
```

## Commands

### context — manage backend contexts

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
|------|-------------|
| `--backend` | Backend type (required): ecs, lambda, cloudrun, gcf, aca, azf, docker |
| `--addr` | Server address (e.g. http://localhost:3375) |
| `--simulator` | Simulator name (from `config.yaml` simulators section) |
| `--set KEY=VALUE` | Set environment variable (repeatable) |

### server — start/stop/restart

```sh
sockerless server start
sockerless server stop
sockerless server restart
```

Flags for `server start`:

| Flag | Default | Description |
|------|---------|-------------|
| `--backend-bin` | `sockerless-backend-{type}` | Path to backend binary |
| `--addr` | `:3375` | Listen address (Docker API + management) |

### status — show server health

```sh
sockerless status
```

Checks the backend health endpoint, reports uptime, backend type, and container count.

### ps — list containers

```sh
sockerless ps
```

Displays a table with ID, NAME, IMAGE, STATE, and POD columns.

### metrics — show server metrics

```sh
sockerless metrics
```

Fetches and pretty-prints metrics from the backend.

### resources — manage cloud resources

```sh
sockerless resources list
sockerless resources orphaned
sockerless resources cleanup
```

### check — run health checks

```sh
sockerless check
```

Runs backend health checks, shows per-check pass/fail status.

### simulator — manage cloud simulators

```sh
sockerless simulator list
sockerless simulator add sim-aws --cloud aws --port 5111
sockerless simulator add sim-gcp --cloud gcp --port 5112 --grpc-port 5113
sockerless simulator remove sim-aws
```

Flags for `simulator add`:

| Flag | Description |
|------|-------------|
| `--cloud` | Cloud type (required): aws, gcp, azure |
| `--port` | Listen port (0 = auto) |
| `--grpc-port` | gRPC port (GCP only) |
| `--log-level` | Log level |

### config migrate — convert JSON contexts to config.yaml

```sh
sockerless config migrate          # preview to stdout
sockerless config migrate --write  # write to config.yaml
```

Reads existing `contexts/*/config.json` files and converts them to the unified `config.yaml` format. Detects simulator usage from `SOCKERLESS_ENDPOINT_URL` and creates simulator entries automatically.

### version

```sh
sockerless version
```

## Environment variables

| Variable | Description |
|----------|-------------|
| `SOCKERLESS_HOME` | Override config directory (default `~/.sockerless`) |
| `SOCKERLESS_CONTEXT` | Override active context name |
| `SOCKERLESS_CONFIG` | Override config file path (default `~/.sockerless/config.yaml`) |

## Configuration layout

```
~/.sockerless/
├── config.yaml                    Unified configuration (environments + simulators)
├── active                         Active context/environment name
├── contexts/                      Legacy JSON contexts (still supported)
│   └── {name}/
│       └── config.json            Backend type, address, env vars
└── run/
    └── {name}/
        └── backend.pid            Server process ID
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

**Priority order:** config.yaml environment values → context env vars (legacy) → process environment variables → defaults.

Use `sockerless config migrate` to convert existing JSON contexts to config.yaml format. Legacy `contexts/*/config.json` files continue to work — the CLI checks config.yaml first, then falls back to JSON contexts.

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
