# frontend-docker

Docker REST API frontend that proxies Docker client requests to a Sockerless backend. Supports TCP and Unix socket listeners, TLS, OpenTelemetry tracing, and Libpod pod endpoints.

## Overview

The frontend translates Docker API v1.44 requests into Sockerless internal API calls. It is a thin proxy — nearly all logic resides in the backend. A separate management API runs on its own port for health checks, metrics, and status.

```
Docker Client          Frontend (:2375)          Backend (:9100)
    │                      │                          │
    ├─ docker run ────────►├─ POST /internal/v1/ ────►│
    │◄─ JSON ──────────────┤◄─ JSON ──────────────────┤
```

## Building

```sh
cd frontends/docker
go build -o sockerless-frontend-docker ./cmd
```

## Usage

```sh
# TCP listener (default)
sockerless-frontend-docker -addr :2375 -backend http://localhost:9100

# Unix socket
sockerless-frontend-docker -addr /var/run/docker.sock -backend http://localhost:9100

# With TLS
sockerless-frontend-docker -tls-cert cert.pem -tls-key key.pem
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:2375` | Docker API listen address (host:port or /path/to/socket) |
| `-backend` | `http://localhost:9100` | Backend service address |
| `-mgmt-addr` | `:9080` | Management API listen address |
| `-log-level` | `info` | Log level: debug, info, warn, error |
| `-tls-cert` | — | TLS certificate file path |
| `-tls-key` | — | TLS private key file path |

## Environment variables

| Variable | Description |
|----------|-------------|
| `OTEL_EXPORTER_OTLP_ENDPOINT` | OTLP HTTP endpoint for tracing (no-op if unset) |

## Docker API endpoints

### Containers (25)
Create, list, inspect, remove, start, stop, restart, kill, pause, unpause, rename, prune, logs, wait, top, stats, attach, resize, update, changes, export, archive get/head/put.

### Exec (4)
Create, inspect, start, resize.

### Images (6)
Pull (create), list, load, remove, prune, search (501).

### Networks (7)
Create, list, inspect, remove, connect, disconnect, prune.

### Volumes (5)
Create, list, inspect, remove, prune.

### System (6)
Ping (GET/HEAD), version, info, events, disk usage.

### Auth (1)
Login.

### Libpod pods (8)
Create, list, inspect, exists, start, stop, kill, remove.

## Management API

Served on `-mgmt-addr` (default `:9080`):

| Endpoint | Description |
|----------|-------------|
| `GET /healthz` | Health check with uptime |
| `GET /status` | Docker addr, backend addr, uptime |
| `GET /metrics` | Goroutines, heap, request count |
| `POST /reload` | Reload configuration |

## Project structure

```
frontends/docker/
├── cmd/main.go           Entry point, flag parsing, TLS setup
├── server.go             HTTP server, route registration, middleware
├── backend_client.go     HTTP client for backend internal API
├── containers.go         Container endpoint handlers
├── containers_stream.go  Attach/exec streaming (101 Upgrade)
├── exec.go               Exec create/start/inspect
├── images.go             Image pull/list/load/tag/remove
├── networks.go           Network CRUD + connect/disconnect
├── volumes.go            Volume CRUD
├── system.go             Ping, version, info
├── pods.go               Libpod pod endpoints
├── mgmt.go               Management API server
├── otel.go               OpenTelemetry tracer init
├── helpers.go             JSON/error/streaming utilities
└── mux/                  Stream multiplexer (reader/writer)
```

## Testing

```sh
cd frontends/docker
go test -v ./...
```

Tests cover the stream multiplexer and TLS configuration.
