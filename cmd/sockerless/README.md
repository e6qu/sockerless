# sockerless

CLI tool for managing Sockerless contexts and server lifecycle. Zero external dependencies.

## Overview

The CLI manages named contexts (backend configurations), starts and stops frontend/backend server processes, and provides commands for inspecting running services. Configuration is stored in `~/.sockerless/` (override with `SOCKERLESS_HOME`).

## Building

```sh
cd cmd/sockerless
go build -o sockerless .
```

## Commands

### context — manage backend contexts

```sh
sockerless context create myctx --backend memory
sockerless context create aws-dev --backend ecs --set AWS_REGION=us-east-1
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
| `--backend` | Backend type (required): memory, ecs, lambda, cloudrun, gcf, aca, azf, docker |
| `--frontend-addr` | Frontend management API address |
| `--backend-addr` | Backend API address |
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
| `--frontend-bin` | `sockerless-frontend-docker` | Path to frontend binary |
| `--backend-addr` | `:9100` | Backend listen address |
| `--frontend-addr` | `:2375` | Frontend Docker API listen address |
| `--mgmt-addr` | `:9080` | Frontend management API listen address |

### status — show server health

```sh
sockerless status
```

Checks frontend and backend health endpoints, reports uptime, backend type, and container count.

### ps — list containers

```sh
sockerless ps
```

Displays a table with ID, NAME, IMAGE, STATE, and POD columns.

### metrics — show server metrics

```sh
sockerless metrics
```

Fetches and pretty-prints metrics from both frontend and backend.

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

### version

```sh
sockerless version
```

## Environment variables

| Variable | Description |
|----------|-------------|
| `SOCKERLESS_HOME` | Override config directory (default `~/.sockerless`) |
| `SOCKERLESS_CONTEXT` | Override active context name |

## Configuration layout

```
~/.sockerless/
├── active                         Active context name
├── contexts/
│   └── {name}/
│       └── config.json            Backend type, addresses, env vars
└── run/
    └── {name}/
        ├── backend.pid            Backend process ID
        └── frontend.pid           Frontend process ID
```

## Project structure

```
cmd/sockerless/
├── main.go        Command dispatcher
├── context.go     Context CRUD commands
├── server.go      Server start/stop/restart
├── status.go      Health status display
├── ps.go          Container listing
├── metrics.go     Metrics display
├── resources.go   Cloud resource management
├── check.go       Health check runner
├── client.go      HTTP management client helpers
└── paths.go       Config directory and path resolution
```
