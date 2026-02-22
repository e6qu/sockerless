# Backends

Backends implement the Sockerless internal API by translating Docker-compatible container operations into cloud-specific resources. Each backend is a separate Go module that listens on an HTTP port and speaks the same protocol understood by the [frontends](../frontends/).

## Architecture

All backends (except `docker`) are built on top of `core`, a shared library that provides:

- In-memory state management for containers, images, networks, volumes
- HTTP route registration for 50+ Docker API endpoints
- A driver chain (Agent -> WASM -> Synthetic) for pluggable execution
- Reverse agent registry for FaaS callback connections
- Dockerfile parsing and image build support
- Container health checking

Cloud backends override specific route handlers (create, start, stop, kill, remove, logs) to map container operations to cloud resources, while inheriting everything else from core.

## Backends

| Backend | Module | Cloud Resource | Agent Mode |
|---------|--------|----------------|------------|
| [core](core/) | `backend-core` | _(shared library)_ | _(n/a)_ |
| [ecs](ecs/) | `backend-ecs` | ECS Fargate Tasks | Forward |
| [lambda](lambda/) | `backend-lambda` | Lambda Functions | Reverse |
| [cloudrun](cloudrun/) | `backend-cloudrun` | Cloud Run Jobs | Forward |
| [cloudrun-functions](cloudrun-functions/) | `backend-gcf` | Cloud Run Functions | Reverse |
| [aca](aca/) | `backend-aca` | Container Apps Jobs | Forward |
| [azure-functions](azure-functions/) | `backend-azf` | Function Apps | Reverse |
| [docker](docker/) | `backend-docker` | Docker Containers | _(native)_ |
| [memory](memory/) | `backend-memory` | WASM Sandbox | _(in-process)_ |

**Agent modes:**
- **Forward** — Backend dials into the running container's agent after it starts
- **Reverse** — Container's agent dials back to the backend via a callback URL (used by FaaS platforms where inbound connections are not possible)

## Building

Each backend is a separate Go module with its own `go.mod`. Build from the backend directory:

```sh
cd backends/ecs
go build -o sockerless-backend-ecs ./cmd/sockerless-backend-ecs
```

Or use the top-level Makefile targets.

## Common flags

All backends accept:

| Flag | Default | Description |
|------|---------|-------------|
| `-addr` | `:9100` | Listen address |
| `-log-level` | `info` | Log level: debug, info, warn, error |

## Common environment variables

| Variable | Description |
|----------|-------------|
| `SOCKERLESS_CALLBACK_URL` | Backend URL for reverse agent connections |
| `SOCKERLESS_ENDPOINT_URL` | Custom cloud API endpoint (simulator mode) |
| `SOCKERLESS_FETCH_IMAGE_CONFIG` | Set to `true` to fetch real image configs from Docker registries |

## Testing

Backend integration tests run against the [simulators](../simulators/):

```sh
# Run all simulator-backend integration tests
make sim-test-all

# Run tests for a specific cloud
make sim-test-aws
make sim-test-gcp
make sim-test-azure
```

Full Terraform integration tests deploy real cloud resources:

```sh
make terraform-test-ecs
make terraform-test-cloudrun
make terraform-test-aca
```
