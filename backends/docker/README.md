# backend-docker

Local Docker passthrough backend. Proxies all container operations directly to a Docker daemon via the Docker SDK.

## Overview

Unlike the cloud backends, this backend does not use `core.BaseServer` or the driver architecture. It implements all route handlers directly using the Docker Go SDK (`github.com/docker/docker/client`), acting as a thin translation layer between the Sockerless internal API and the Docker API.

No agent is involved — exec, attach, and logs go straight to Docker.

## Building

```sh
cd backends/docker
go build -o sockerless-backend-docker ./cmd/main.go
```

## Usage

```sh
# Use default Docker socket
sockerless-backend-docker

# Specify Docker host
sockerless-backend-docker -docker-host tcp://localhost:2375
```

## Flags

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:9100` | Listen address |
| `-docker-host` | _(auto-detect)_ | Docker daemon socket or TCP address |
| `-log-level` | `info` | Log level: debug, info, warn, error |

## Supported operations

The full Docker API surface is implemented:

- **Containers** — create, list, inspect, start, stop, kill, remove, restart, rename, pause, unpause, top, stats, wait, attach, logs, prune
- **Exec** — create, inspect, start
- **Images** — pull, inspect, load, tag, list, remove, history, prune
- **Networks** — create, list, inspect, connect, disconnect, remove, prune
- **Volumes** — create, list, inspect, remove, prune
- **System** — events, disk usage
- **Auth** — registry authentication

## Docker API mapping

For a detailed breakdown of how each Docker REST API endpoint and CLI command maps to Docker SDK calls — including what's implemented, what's not, and minor differences from vanilla Docker — see [docs/docker_api_mapping.md](docs/docker_api_mapping.md).

## Project structure

```
docker/
├── cmd/
│   └── main.go          CLI entrypoint
├── server.go            Server type, route registration
├── client.go            Docker SDK client initialization
├── containers.go        Container lifecycle + attach/wait/logs
├── exec.go              Exec create, inspect, start
├── images.go            Image pull, inspect, load, tag
├── networks.go          Network CRUD + connect/disconnect
├── volumes.go           Volume CRUD
└── extended.go          Restart, top, stats, rename, pause, events, df
```
